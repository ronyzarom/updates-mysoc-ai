"use client";

import { useQuery } from "@tanstack/react-query";
import { api, InstanceTreeNode, InstanceTreeCustomer } from "@/lib/api";
import {
  ChevronDown,
  ChevronRight,
  Server,
  RefreshCw,
  Building2,
  AlertTriangle,
  Network,
  Tag,
  Radio,
  KeyRound,
  Archive,
} from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import Link from "next/link";
import { useState } from "react";

const TIER_STYLES: Record<string, string> = {
  mysoc: "bg-violet-500/20 text-violet-300 border border-violet-500/30",
  siemcore: "bg-cyan-500/20 text-cyan-300 border border-cyan-500/30",
  swf: "bg-amber-500/20 text-amber-300 border border-amber-500/30",
};

const TIER_LABELS: Record<string, string> = {
  mysoc: "MySoc",
  siemcore: "SiemCore",
  swf: "SWF",
};

export function TierBadge({ tier }: { tier?: string }) {
  const key = (tier || "").toLowerCase();
  const style = TIER_STYLES[key] || "bg-slate-600/30 text-slate-300 border border-slate-600/40";
  const label = TIER_LABELS[key] || (tier ? tier : "untiered");
  return (
    <span className={`text-[10px] font-medium uppercase tracking-wide px-1.5 py-0.5 rounded ${style}`}>
      {label}
    </span>
  );
}

function StatusDot({ status }: { status: string }) {
  const color =
    status === "online"
      ? "bg-emerald-400"
      : status === "degraded"
      ? "bg-amber-400"
      : status === "offline"
      ? "bg-red-400"
      : "bg-slate-500";
  return <span className={`inline-block w-2 h-2 rounded-full ${color}`} title={status} />;
}

// Freshness thresholds for the last-seen indicator (heartbeats run ~1m,
// rollups ride on the parent's heartbeat).
const FRESH_MS = 5 * 60 * 1000;
const STALE_MS = 60 * 60 * 1000;

function LastSeen({ node }: { node: InstanceTreeNode }) {
  if (!node.last_heartbeat) {
    return <span className="ml-auto text-xs text-slate-600 whitespace-nowrap">never seen</span>;
  }
  const age = Date.now() - new Date(node.last_heartbeat).getTime();
  const color =
    age < FRESH_MS ? "text-emerald-400/80" : age < STALE_MS ? "text-amber-400/80" : "text-red-400/80";
  return (
    <span className={`ml-auto text-xs whitespace-nowrap ${color}`}>
      {formatDistanceToNow(new Date(node.last_heartbeat), { addSuffix: true })}
    </span>
  );
}

function matchesTier(node: InstanceTreeNode, tierFilter: string): boolean {
  if (tierFilter === "all") return true;
  if ((node.product_tier || "").toLowerCase() === tierFilter) return true;
  return node.children.some((c) => matchesTier(c, tierFilter));
}

function matchesSearch(node: InstanceTreeNode, needle: string): boolean {
  if (!needle) return true;
  const haystack = [
    node.instance_id,
    node.hostname,
    node.display_name,
    node.customer_id,
    node.version,
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
  if (haystack.includes(needle)) return true;
  return node.children.some((c) => matchesSearch(c, needle));
}

function nodeVisible(node: InstanceTreeNode, tierFilter: string, needle: string): boolean {
  return matchesTier(node, tierFilter) && matchesSearch(node, needle);
}

// Retired = decommissioned tombstone: kept for audit but excluded from alarms.
// The cascade tree hides them by default so stale test/replaced nodes don't read
// as live "offline" rows; the Show retired toggle brings them back.
function isRetired(node: InstanceTreeNode): boolean {
  return (node.status || "").toLowerCase() === "decommissioned";
}

// nodeRenderable folds the retired filter into the tier/search visibility so a
// single predicate drives every level of the tree.
function nodeRenderable(
  node: InstanceTreeNode,
  tierFilter: string,
  needle: string,
  showRetired: boolean
): boolean {
  return nodeVisible(node, tierFilter, needle) && (showRetired || !isRetired(node));
}

function TreeNodeRow({
  node,
  depth,
  tierFilter,
  search,
  showRetired,
}: {
  node: InstanceTreeNode;
  depth: number;
  tierFilter: string;
  search: string;
  showRetired: boolean;
}) {
  const [open, setOpen] = useState(true);
  const retired = isRetired(node);
  const visibleChildren = node.children.filter((c) =>
    nodeRenderable(c, tierFilter, search, showRetired)
  );
  const hasChildren = visibleChildren.length > 0;
  const highlighted = tierFilter !== "all" && (node.product_tier || "").toLowerCase() === tierFilter;
  // A relay serves updates to the nodes nested under it.
  const isRelay = hasChildren || (node.product_tier === "mysoc" || node.product_tier === "siemcore");

  return (
    <div>
      <div
        className={`flex items-center gap-2 py-1.5 px-2 rounded-md hover:bg-slate-800/60 ${
          highlighted ? "ring-1 ring-cyan-500/40 bg-slate-800/40" : ""
        }`}
        style={{ marginLeft: depth * 18 }}
      >
        {hasChildren ? (
          <button
            onClick={() => setOpen((v) => !v)}
            className="text-slate-400 hover:text-white"
            aria-label={open ? "Collapse" : "Expand"}
          >
            {open ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
          </button>
        ) : (
          <span className="w-4 h-4 inline-block" />
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
        {node.version && (
          <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700/70 text-slate-300 font-mono">
            v{node.version}
          </span>
        )}
        {isRelay && hasChildren && (
          <span
            className="flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-300 border border-emerald-500/30"
            title={`Relay: serves updates to ${visibleChildren.length} child node${visibleChildren.length === 1 ? "" : "s"}`}
          >
            <Radio className="w-3 h-3" />
            relay
          </span>
        )}
        {node.reported_via && (
          <span
            className="text-[10px] text-slate-500"
            title={`Reported through the cascade by ${node.reported_via}; this node never contacts the updates server directly`}
          >
            via {node.reported_via}
          </span>
        )}
        {node.orphan && (
          <span
            className="flex items-center gap-1 text-[10px] text-amber-400"
            title="Declared parent is not enrolled anywhere in the fleet"
          >
            <AlertTriangle className="w-3 h-3" />
            orphan
          </span>
        )}
        {retired && (
          <span
            className="flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-slate-600/30 text-slate-400 border border-slate-600/40"
            title="Retired: decommissioned, excluded from alarms; kept for audit"
          >
            <Archive className="w-3 h-3" />
            retired
          </span>
        )}
        <LastSeen node={node} />
      </div>
      {hasChildren && open && (
        <div>
          {visibleChildren.map((child) => (
            <TreeNodeRow
              key={child.id}
              node={child}
              depth={depth + 1}
              tierFilter={tierFilter}
              search={search}
              showRetired={showRetired}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function customerVisible(
  customer: InstanceTreeCustomer,
  filtering: boolean,
  tierFilter: string,
  search: string
): boolean {
  if (!filtering && !search) return true;
  if (search) {
    const label = `${customer.customer_name} ${customer.customer_id || ""}`.toLowerCase();
    if (label.includes(search) && customer.roots.some((r) => matchesTier(r, tierFilter))) {
      return true;
    }
  }
  return customer.roots.some((r) => nodeVisible(r, tierFilter, search));
}

export function InstanceTree({
  tierFilter = "all",
  search = "",
  operatorFilter,
  hideOperatorHeader = false,
}: {
  tierFilter?: string;
  search?: string;
  // Restrict the tree to one operator (used by the operator detail page).
  operatorFilter?: string;
  hideOperatorHeader?: boolean;
}) {
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["instance-tree"],
    queryFn: () => api.getInstanceTree(),
  });

  const [showRetired, setShowRetired] = useState(false);
  const needle = search.trim().toLowerCase();

  if (isLoading) {
    return <div className="text-slate-400">Loading fleet tree...</div>;
  }
  if (isError) {
    return (
      <div className="card border-red-500/50 bg-red-900/20">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-red-400 font-medium">Failed to load fleet tree</p>
            <p className="text-sm text-slate-400 mt-1">
              {error instanceof Error ? error.message : "Unknown error"}
            </p>
          </div>
          <button onClick={() => refetch()} className="btn btn-secondary">
            <RefreshCw className="w-4 h-4" />
            Retry
          </button>
        </div>
      </div>
    );
  }

  const filtering = tierFilter !== "all" || needle !== "";
  // When filtering, hide operators/customers with no match; otherwise show
  // everything, including customers with no instances yet.
  const operators = (data?.operators || [])
    .filter((op) => !operatorFilter || op.operator_id === operatorFilter)
    .filter(
      (op) =>
        !filtering ||
        op.platform_roots.some((r) => nodeVisible(r, tierFilter, needle)) ||
        op.customers.some((c) => customerVisible(c, filtering, tierFilter, needle)) ||
        (needle !== "" && op.operator_name.toLowerCase().includes(needle))
    );

  if (operators.length === 0) {
    return (
      <div className="card text-center py-16">
        <Server className="w-12 h-12 text-slate-600 mx-auto mb-4" />
        <h3 className="text-lg font-medium text-white mb-2">
          {filtering ? "Nothing matches the current filters" : "No instances to show"}
        </h3>
        <p className="text-slate-400">
          {filtering
            ? "Try clearing the search or tier filter."
            : "The fleet appears here as the operator's mysoc platform heartbeats and rolls up its cascade."}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex justify-end">
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
      {operators.map((op, opIdx) => (
        <div key={op.operator_id || `unassigned-${opIdx}`} className="card">
          {!hideOperatorHeader && (
            <div className="flex items-center gap-3 mb-3 pb-3 border-b border-slate-700">
              <div className="p-2 rounded-lg bg-slate-800">
                <Network className="w-5 h-5 text-violet-400" />
              </div>
              <div className="min-w-0">
                <h3 className="font-semibold text-white truncate">{op.operator_name}</h3>
                <p className="text-xs text-slate-500">
                  Operator{op.operator_id ? ` · ${op.operator_id}` : ""} ·{" "}
                  {op.total_nodes} node{op.total_nodes === 1 ? "" : "s"}
                </p>
              </div>
              {op.is_active === false && (
                <span className="ml-auto status-badge status-offline">Deactivated</span>
              )}
            </div>
          )}

          {op.platform_roots.length > 0 && (
            <div className="space-y-0.5 mb-4">
              <p className="text-xs uppercase tracking-wide text-slate-500 px-2 mb-1">
                Platform (mysoc)
              </p>
              {op.platform_roots
                .filter((r) => nodeRenderable(r, tierFilter, needle, showRetired))
                .map((root) => (
                  <TreeNodeRow
                    key={root.id}
                    node={root}
                    depth={0}
                    tierFilter={tierFilter}
                    search={needle}
                    showRetired={showRetired}
                  />
                ))}
            </div>
          )}

          <div className="space-y-3">
            {op.customers
              .filter((c) => customerVisible(c, filtering, tierFilter, needle))
              .map((customer, idx) => (
                <div
                  key={customer.customer_id || customer.license_id || `unlicensed-${idx}`}
                  className="rounded-lg border border-slate-700/70 bg-slate-900/40 p-3"
                >
                  <div className="flex items-center gap-2 mb-2">
                    <Building2 className="w-4 h-4 text-cyan-400 shrink-0" />
                    <span className="font-medium text-white text-sm truncate">
                      {customer.customer_name}
                    </span>
                    {customer.legacy && (
                      <span
                        className="flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-slate-600/30 text-slate-400 border border-slate-600/50"
                        title="Grouped by a pre-1.8.0 license; these nodes contact the updates server directly instead of the operator's cascade"
                      >
                        <KeyRound className="w-3 h-3" />
                        legacy key
                      </span>
                    )}
                    {customer.reseller_id && (
                      <span
                        className="flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-fuchsia-500/15 text-fuchsia-300 border border-fuchsia-500/30"
                        title={`Sold via reseller ${customer.reseller_name || customer.reseller_id}`}
                      >
                        <Tag className="w-3 h-3" />
                        {customer.reseller_name || customer.reseller_id}
                      </span>
                    )}
                    <span className="ml-auto text-xs text-slate-500 whitespace-nowrap">
                      {customer.total_nodes} node{customer.total_nodes === 1 ? "" : "s"}
                      {customer.legacy && customer.license_key ? ` · ${customer.license_key}` : ""}
                    </span>
                  </div>
                  {(() => {
                    const visibleRoots = customer.roots.filter((r) =>
                      nodeRenderable(r, tierFilter, needle, showRetired)
                    );
                    if (visibleRoots.length === 0) {
                      return (
                        <p className="text-xs text-slate-500 px-2 py-1">
                          {customer.roots.length === 0
                            ? "No instances reported yet."
                            : "Only retired nodes — use Show retired to view."}
                        </p>
                      );
                    }
                    return (
                      <div className="space-y-0.5">
                        {visibleRoots.map((root) => (
                          <TreeNodeRow
                            key={root.id}
                            node={root}
                            depth={0}
                            tierFilter={tierFilter}
                            search={needle}
                            showRetired={showRetired}
                          />
                        ))}
                      </div>
                    );
                  })()}
                </div>
              ))}
          </div>
        </div>
      ))}
    </div>
  );
}
