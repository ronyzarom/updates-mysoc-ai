"use client";

import { useState, type ReactNode } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { api, TreeChildRow, TreeChildrenResponse } from "@/lib/api";
import {
  ChevronDown,
  ChevronRight,
  Network,
  Radio,
  Server,
  AlertTriangle,
  Archive,
} from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import Link from "next/link";
import { TierBadge } from "@/components/InstanceTree";

const PAGE_SIZE = 100;

// Freshness thresholds for the last-seen indicator (heartbeats run ~1m; rollup
// rows ride on their relay's heartbeat).
const FRESH_MS = 5 * 60 * 1000;
const STALE_MS = 60 * 60 * 1000;

function StatusDot({ status }: { status: string }) {
  const color =
    status === "online"
      ? "bg-emerald-400"
      : status === "degraded"
      ? "bg-amber-400"
      : status === "offline"
      ? "bg-red-400"
      : "bg-slate-500";
  return <span className={`inline-block w-2 h-2 rounded-full shrink-0 ${color}`} title={status} />;
}

function LastSeen({ ts, status, reportedVia }: { ts?: string; status?: string; reportedVia?: string }) {
  if (!ts) {
    return <span className="text-xs text-slate-600 whitespace-nowrap">never seen</span>;
  }
  // Relay-reported rows refresh only on change (delta reporting); the server
  // exempts them from the time-based offline rule, so ageing them out would
  // paint a healthy leaf red. Colour those by status instead.
  let color: string;
  if (reportedVia) {
    color =
      status === "online" ? "text-emerald-400/80" : status === "degraded" ? "text-amber-400/80" : "text-red-400/80";
  } else {
    const age = Date.now() - new Date(ts).getTime();
    color = age < FRESH_MS ? "text-emerald-400/80" : age < STALE_MS ? "text-amber-400/80" : "text-red-400/80";
  }
  return (
    <span
      className={`text-xs whitespace-nowrap ${color}`}
      title={reportedVia ? `Last reported via ${reportedVia}` : undefined}
    >
      {reportedVia ? "via " : ""}
      {formatDistanceToNow(new Date(ts), { addSuffix: true })}
    </span>
  );
}

// SubtreeRollup shows a collapsed relay's coverage (its whole cascade) so the
// tree stays informative without expanding. Counts come from SQL, not the
// client counting rows. When retired nodes are hidden, they're excluded from
// the "below" count so it matches what expanding actually shows.
function SubtreeRollup({ node, showRetired }: { node: TreeChildRow; showRetired: boolean }) {
  const s = node.subtree;
  // Subtree counts are descendants only (the server excludes the node itself).
  // When retired nodes are hidden, drop them from the "below" count so it
  // matches what expanding actually shows.
  const below = showRetired ? s.total : Math.max(0, s.total - s.decommissioned);
  if (below === 0) return null;
  return (
    <span className="flex items-center gap-2 text-[11px] whitespace-nowrap">
      <span className="text-slate-400">
        {below.toLocaleString()} below
      </span>
      {s.online > 0 && <span className="text-emerald-400/80">{s.online.toLocaleString()} online</span>}
      {s.offline > 0 && <span className="text-red-400/80">{s.offline.toLocaleString()} offline</span>}
      {s.failed > 0 && <span className="text-amber-400/80">{s.failed.toLocaleString()} failed</span>}
      {showRetired && s.decommissioned > 0 && (
        <span className="text-slate-500">{s.decommissioned.toLocaleString()} retired</span>
      )}
    </span>
  );
}

// useLevel lazily loads one cascade level: the direct children of `parent`, or
// the roots when parent is undefined. Paged so a relay with thousands of leaves
// never floods the client. The tree is NOT filtered by tier — roots are
// parentless nodes, so a tier filter would empty it and block drilling into a
// relay's differently-tiered children.
function useLevel(parent: string | undefined, showRetired: boolean, enabled: boolean) {
  return useInfiniteQuery({
    queryKey: ["fleet-tree", parent ?? "__roots__", showRetired],
    enabled,
    initialPageParam: 0,
    // An infinite list must not auto-refetch: the global 30s interval re-pulls
    // every loaded page and shifts rows under the user.
    refetchInterval: false,
    queryFn: ({ pageParam }) =>
      api.getTreeChildren({
        parent,
        includeDecommissioned: showRetired,
        limit: PAGE_SIZE,
        offset: pageParam as number,
      }),
    getNextPageParam: (last: TreeChildrenResponse) => {
      const loaded = last.offset + last.items.length;
      return loaded < last.total ? loaded : undefined;
    },
  });
}

function TreeNode({
  node,
  depth,
  showRetired,
}: {
  node: TreeChildRow;
  depth: number;
  showRetired: boolean;
}) {
  const [open, setOpen] = useState(false);
  const isRelay = node.has_children;
  const retired = node.status === "decommissioned";
  // A node that surfaces as a root but still declares a parent is an orphan:
  // its parent is missing or retired. Only meaningful at the root level.
  const orphan = depth === 0 && Boolean(node.parent_instance_id);

  return (
    <div>
      <div
        className="flex items-center gap-2 py-1.5 px-2 rounded-md hover:bg-slate-800/60"
        style={{ marginLeft: depth * 18 }}
      >
        {isRelay ? (
          <button
            onClick={() => setOpen((v) => !v)}
            className="text-slate-400 hover:text-white shrink-0"
            aria-label={open ? "Collapse" : "Expand"}
            aria-expanded={open}
          >
            {open ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
          </button>
        ) : (
          <span className="w-4 h-4 inline-block shrink-0" />
        )}
        <StatusDot status={node.status} />
        <TierBadge tier={node.product_tier} />
        <Link
          href={`/instances/${node.id}`}
          className="text-sm text-white hover:text-cyan-300 font-medium truncate"
        >
          {node.instance_id}
        </Link>
        {node.display_name && node.display_name !== node.instance_id && (
          <span className="text-xs text-slate-500 truncate">· {node.display_name}</span>
        )}
        {isRelay && (
          <span
            className="flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-300 border border-emerald-500/30 shrink-0"
            title="Relay: serves updates to the nodes nested under it"
          >
            <Radio className="w-3 h-3" />
            relay
          </span>
        )}
        {node.reported_via && (
          <span
            className="text-[10px] text-slate-500 shrink-0"
            title={`Reported through the cascade by ${node.reported_via}; this node never contacts the updates server directly`}
          >
            via {node.reported_via}
          </span>
        )}
        {retired && (
          <span
            className="text-[10px] px-1.5 py-0.5 rounded bg-slate-600/30 text-slate-400 border border-slate-600/40 shrink-0"
            title="Retired: decommissioned, excluded from alarms; kept for audit"
          >
            retired
          </span>
        )}
        {orphan && (
          <span
            className="text-[10px] px-1.5 py-0.5 rounded bg-amber-500/10 text-amber-400 border border-amber-500/30 shrink-0"
            title={`Orphan: declared parent ${node.parent_instance_id} is missing or retired`}
          >
            orphan
          </span>
        )}
        {node.self_update_capable === false && (
          <span
            className="text-[10px] px-1.5 py-0.5 rounded bg-red-500/10 text-red-400 border border-red-500/30 shrink-0"
            title="Cannot self-update — needs a kit reinstall"
          >
            no self-update
          </span>
        )}
        {node.relay_listener_ok === false && (
          <span
            className="text-[10px] px-1.5 py-0.5 rounded bg-red-500/10 text-red-400 border border-red-500/30 shrink-0"
            title="Relay listener down — dead cascade beneath this node"
          >
            listener down
          </span>
        )}
        {node.last_update_success === false && (
          <span
            className="text-[10px] px-1.5 py-0.5 rounded bg-amber-500/10 text-amber-400 border border-amber-500/30 shrink-0"
            title={node.last_update_error || "Last update failed"}
          >
            update failed
          </span>
        )}
        {node.subtree.offline > 0 && !open && (
          <AlertTriangle className="w-3.5 h-3.5 text-amber-400 shrink-0" aria-label="Offline nodes below" />
        )}
        <div className="ml-auto flex items-center gap-3 shrink-0">
          {!open && <SubtreeRollup node={node} showRetired={showRetired} />}
          <LastSeen
            ts={node.reported_via ? node.reported_at : node.last_heartbeat}
            status={node.status}
            reportedVia={node.reported_via}
          />
        </div>
      </div>
      {open && isRelay && (
        <TreeLevel parent={node.instance_id} depth={depth + 1} showRetired={showRetired} />
      )}
    </div>
  );
}

function TreeLevel({
  parent,
  depth,
  showRetired,
}: {
  parent: string | undefined;
  depth: number;
  showRetired: boolean;
}) {
  const { data, isLoading, isError, error, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useLevel(parent, showRetired, true);

  const nodes = data?.pages.flatMap((p) => p.items) ?? [];
  const indent = { marginLeft: depth * 18 + 8 };

  if (isLoading) {
    return (
      <div className="text-xs text-slate-500 py-1.5" style={indent}>
        Loading…
      </div>
    );
  }
  // Only blank the level when there is nothing loaded; a failed next page keeps
  // the rows already shown and offers an inline retry below.
  if (isError && nodes.length === 0) {
    return (
      <div className="text-xs text-red-400 py-1.5 flex items-center gap-2" style={indent}>
        <span>{error instanceof Error ? error.message : "Failed to load"}</span>
        <button onClick={() => refetch()} className="text-cyan-400 hover:underline">
          Retry
        </button>
      </div>
    );
  }
  if (nodes.length === 0) {
    return (
      <div className="text-xs text-slate-500 py-1.5" style={indent}>
        No nodes reported.
      </div>
    );
  }

  return (
    <div>
      {nodes.map((n) => (
        <TreeNode key={n.id} node={n} depth={depth} showRetired={showRetired} />
      ))}
      {isError && (
        <button onClick={() => refetch()} className="text-xs text-red-400 hover:underline py-1" style={indent}>
          Failed to load more — retry
        </button>
      )}
      {hasNextPage && !isError && (
        <button
          onClick={() => fetchNextPage()}
          disabled={isFetchingNextPage}
          className="text-xs text-cyan-400 hover:underline py-1"
          style={indent}
        >
          {isFetchingNextPage ? "Loading…" : "Load more nodes"}
        </button>
      )}
    </div>
  );
}

// FleetTree renders the update cascade (mysoc > siemcore > swf) aggregate-by-
// default: the roots load with a SQL rollup of everything beneath them, and each
// relay expands one bounded level at a time. Never an O(fleet) render.
// The tier prop is accepted for call-site compatibility but intentionally not
// used to filter: see useLevel. Tier narrowing belongs to the List view.
// `operator` scopes the roots to one operator's cascade (the operator detail
// page); child levels stay within that cascade because they load by parent.
export function FleetTree({ search = "", operator }: { tier?: string; search?: string; operator?: string }) {
  const needle = search.trim();
  const [showRetired, setShowRetired] = useState(false);
  const { data, isLoading, isError, error, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useInfiniteQuery({
      queryKey: ["fleet-tree-roots", operator ?? "", needle, showRetired],
      initialPageParam: 0,
      refetchInterval: false,
      queryFn: ({ pageParam }) =>
        api.getTreeChildren({
          operator: operator || undefined,
          search: needle || undefined,
          includeDecommissioned: showRetired,
          limit: PAGE_SIZE,
          offset: pageParam as number,
        }),
      getNextPageParam: (last: TreeChildrenResponse) => {
        const loaded = last.offset + last.items.length;
        return loaded < last.total ? loaded : undefined;
      },
    });

  const roots = data?.pages.flatMap((p) => p.items) ?? [];
  const total = data?.pages[0]?.total ?? 0;

  const header = (
    <div className="flex items-center justify-between gap-3">
      <div className="flex items-center gap-2 text-sm text-slate-500">
        <Server className="w-4 h-4" />
        <span>
          {isLoading
            ? "Loading fleet cascade…"
            : `${total.toLocaleString()} cascade root${total === 1 ? "" : "s"} · expand a relay to drill in`}
        </span>
      </div>
      <button
        onClick={() => setShowRetired((v) => !v)}
        className={`flex items-center gap-1.5 px-2.5 py-1.5 text-xs rounded-lg border ${
          showRetired
            ? "border-slate-500 bg-slate-700 text-white"
            : "border-slate-700 text-slate-400 hover:text-white"
        }`}
        aria-pressed={showRetired}
        title="Retired = decommissioned tombstones, hidden by default"
      >
        <Archive className="w-3.5 h-3.5" />
        {showRetired ? "Hide retired" : "Show retired"}
      </button>
    </div>
  );

  let body: ReactNode;
  if (isError && roots.length === 0) {
    body = (
      <div className="card border-red-500/50 bg-red-900/20">
        <p className="text-red-400 font-medium">Failed to load fleet tree</p>
        <p className="text-sm text-slate-400 mt-1">
          {error instanceof Error ? error.message : "Unknown error"}
        </p>
        <button onClick={() => refetch()} className="btn btn-secondary mt-3">
          Retry
        </button>
      </div>
    );
  } else if (isLoading) {
    body = <div className="card text-slate-400">Loading…</div>;
  } else if (roots.length === 0) {
    body = (
      <div className="card text-center py-16">
        <Network className="w-12 h-12 text-slate-600 mx-auto mb-4" />
        <h3 className="text-lg font-medium text-white mb-2">
          {needle ? "No cascade roots match" : "No cascade roots yet"}
        </h3>
        <p className="text-slate-400">
          {needle
            ? "Search matches roots here; use the List view to find nodes deep in the cascade."
            : "Roots appear as operators' mysoc platform nodes heartbeat and roll up their cascade."}
        </p>
      </div>
    );
  } else {
    body = (
      <div className="card">
        <div className="space-y-0.5">
          {roots.map((n) => (
            <TreeNode key={n.id} node={n} depth={0} showRetired={showRetired} />
          ))}
        </div>
        {isError && (
          <div className="pt-2">
            <button onClick={() => refetch()} className="btn btn-secondary">
              Failed to load more — retry
            </button>
          </div>
        )}
        {hasNextPage && !isError && (
          <div className="pt-2">
            <button
              onClick={() => fetchNextPage()}
              disabled={isFetchingNextPage}
              className="btn btn-secondary"
            >
              {isFetchingNextPage ? "Loading…" : "Load more roots"}
            </button>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {header}
      {body}
    </div>
  );
}
