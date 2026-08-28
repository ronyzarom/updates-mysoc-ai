"use client";

import { useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { api, TreeChildRow, TreeChildrenResponse } from "@/lib/api";
import {
  ChevronDown,
  ChevronRight,
  Network,
  Radio,
  Server,
  AlertTriangle,
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

function LastSeen({ ts }: { ts?: string }) {
  if (!ts) {
    return <span className="text-xs text-slate-600 whitespace-nowrap">never seen</span>;
  }
  const age = Date.now() - new Date(ts).getTime();
  const color =
    age < FRESH_MS ? "text-emerald-400/80" : age < STALE_MS ? "text-amber-400/80" : "text-red-400/80";
  return (
    <span className={`text-xs whitespace-nowrap ${color}`}>
      {formatDistanceToNow(new Date(ts), { addSuffix: true })}
    </span>
  );
}

// SubtreeRollup shows a collapsed relay's coverage (its whole cascade) so the
// tree stays informative without expanding. Counts come from SQL, not the
// client counting rows.
function SubtreeRollup({ node }: { node: TreeChildRow }) {
  const s = node.subtree;
  // total includes the node itself; show how many sit beneath it.
  const below = Math.max(0, s.total - 1);
  if (below === 0) return null;
  return (
    <span className="flex items-center gap-2 text-[11px] whitespace-nowrap">
      <span className="text-slate-400">
        {below.toLocaleString()} below
      </span>
      {s.online > 0 && <span className="text-emerald-400/80">{s.online.toLocaleString()} online</span>}
      {s.offline > 0 && <span className="text-red-400/80">{s.offline.toLocaleString()} offline</span>}
      {s.failed > 0 && <span className="text-amber-400/80">{s.failed.toLocaleString()} failed</span>}
    </span>
  );
}

// useLevel lazily loads one cascade level: the direct children of `parent`, or
// the roots when parent is undefined. Paged so a relay with thousands of leaves
// never floods the client.
function useLevel(parent: string | undefined, tier: string, enabled: boolean) {
  return useInfiniteQuery({
    queryKey: ["fleet-tree", parent ?? "__roots__", tier],
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api.getTreeChildren({
        parent,
        tier: tier === "all" ? undefined : tier,
        limit: PAGE_SIZE,
        offset: pageParam as number,
      }),
    getNextPageParam: (last: TreeChildrenResponse) => {
      const loaded = last.offset + last.items.length;
      return loaded < last.total ? loaded : undefined;
    },
  });
}

function TreeNode({ node, depth, tier }: { node: TreeChildRow; depth: number; tier: string }) {
  const [open, setOpen] = useState(false);
  const isRelay = node.has_children;

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
        {node.subtree.offline > 0 && !open && (
          <AlertTriangle className="w-3.5 h-3.5 text-amber-400 shrink-0" aria-label="Offline nodes below" />
        )}
        <div className="ml-auto flex items-center gap-3 shrink-0">
          {!open && <SubtreeRollup node={node} />}
          <LastSeen ts={node.last_heartbeat} />
        </div>
      </div>
      {open && isRelay && <TreeLevel parent={node.instance_id} depth={depth + 1} tier={tier} />}
    </div>
  );
}

function TreeLevel({
  parent,
  depth,
  tier,
}: {
  parent: string | undefined;
  depth: number;
  tier: string;
}) {
  const { data, isLoading, isError, error, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useLevel(parent, tier, true);

  const nodes = data?.pages.flatMap((p) => p.items) ?? [];

  if (isLoading) {
    return (
      <div className="text-xs text-slate-500 py-1.5" style={{ marginLeft: depth * 18 + 8 }}>
        Loading…
      </div>
    );
  }
  if (isError) {
    return (
      <div className="text-xs text-red-400 py-1.5" style={{ marginLeft: depth * 18 + 8 }}>
        {error instanceof Error ? error.message : "Failed to load"}
      </div>
    );
  }
  if (nodes.length === 0) {
    return (
      <div className="text-xs text-slate-500 py-1.5" style={{ marginLeft: depth * 18 + 8 }}>
        No nodes reported.
      </div>
    );
  }

  return (
    <div>
      {nodes.map((n) => (
        <TreeNode key={n.id} node={n} depth={depth} tier={tier} />
      ))}
      {hasNextPage && (
        <button
          onClick={() => fetchNextPage()}
          disabled={isFetchingNextPage}
          className="text-xs text-cyan-400 hover:underline py-1"
          style={{ marginLeft: depth * 18 + 8 }}
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
export function FleetTree({ tier = "all", search = "" }: { tier?: string; search?: string }) {
  const needle = search.trim();
  const { data, isLoading, isError, error, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useInfiniteQuery({
      queryKey: ["fleet-tree-roots", tier, needle],
      initialPageParam: 0,
      queryFn: ({ pageParam }) =>
        api.getTreeChildren({
          tier: tier === "all" ? undefined : tier,
          search: needle || undefined,
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

  if (isLoading) return <div className="text-slate-400">Loading fleet cascade…</div>;
  if (isError) {
    return (
      <div className="card border-red-500/50 bg-red-900/20">
        <p className="text-red-400 font-medium">Failed to load fleet tree</p>
        <p className="text-sm text-slate-400 mt-1">
          {error instanceof Error ? error.message : "Unknown error"}
        </p>
      </div>
    );
  }

  if (roots.length === 0) {
    return (
      <div className="card text-center py-16">
        <Network className="w-12 h-12 text-slate-600 mx-auto mb-4" />
        <h3 className="text-lg font-medium text-white mb-2">
          {needle || tier !== "all" ? "No cascade roots match" : "No cascade roots yet"}
        </h3>
        <p className="text-slate-400">
          {needle || tier !== "all"
            ? "Search matches roots here; use the List view to find nodes deep in the cascade."
            : "Roots appear as operators' mysoc platform nodes heartbeat and roll up their cascade."}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2 text-sm text-slate-500">
        <Server className="w-4 h-4" />
        <span>
          {total.toLocaleString()} cascade root{total === 1 ? "" : "s"} · expand a relay to drill in
        </span>
      </div>
      <div className="card">
        <div className="space-y-0.5">
          {roots.map((n) => (
            <TreeNode key={n.id} node={n} depth={0} tier={tier} />
          ))}
        </div>
        {hasNextPage && (
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
    </div>
  );
}
