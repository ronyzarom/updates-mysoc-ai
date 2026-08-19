"use client";

import { useQuery } from "@tanstack/react-query";
import { api, InstanceTreeNode } from "@/lib/api";
import { ChevronDown, ChevronRight, Server, RefreshCw, Building2, AlertTriangle, Network, Tag } from "lucide-react";
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

function matchesTier(node: InstanceTreeNode, tierFilter: string): boolean {
  if (tierFilter === "all") return true;
  if ((node.product_tier || "").toLowerCase() === tierFilter) return true;
  return node.children.some((c) => matchesTier(c, tierFilter));
}

function TreeNodeRow({
  node,
  depth,
  tierFilter,
}: {
  node: InstanceTreeNode;
  depth: number;
  tierFilter: string;
}) {
  const [open, setOpen] = useState(true);
  const hasChildren = node.children.length > 0;
  const highlighted = tierFilter !== "all" && (node.product_tier || "").toLowerCase() === tierFilter;

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
        {node.orphan && (
          <span className="flex items-center gap-1 text-[10px] text-amber-400" title="Declared parent is not enrolled anywhere in the fleet">
            <AlertTriangle className="w-3 h-3" />
            orphan
          </span>
        )}
        {node.last_heartbeat && (
          <span className="ml-auto text-xs text-slate-500 whitespace-nowrap">
            {formatDistanceToNow(new Date(node.last_heartbeat), { addSuffix: true })}
          </span>
        )}
      </div>
      {hasChildren && open && (
        <div>
          {node.children
            .filter((c) => matchesTier(c, tierFilter))
            .map((child) => (
              <TreeNodeRow key={child.id} node={child} depth={depth + 1} tierFilter={tierFilter} />
            ))}
        </div>
      )}
    </div>
  );
}

export function InstanceTree({ tierFilter = "all" }: { tierFilter?: string }) {
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["instance-tree"],
    queryFn: () => api.getInstanceTree(),
  });

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

  const filtering = tierFilter !== "all";
  // When a tier filter is active, hide operators/customers with no match;
  // otherwise show everything, including customers with no instances yet.
  const operators = (data?.operators || []).filter(
    (op) =>
      !filtering ||
      op.platform_roots.some((r) => matchesTier(r, tierFilter)) ||
      op.customers.some((c) => c.roots.some((r) => matchesTier(r, tierFilter)))
  );

  if (operators.length === 0) {
    return (
      <div className="card text-center py-16">
        <Server className="w-12 h-12 text-slate-600 mx-auto mb-4" />
        <h3 className="text-lg font-medium text-white mb-2">No instances to show</h3>
        <p className="text-slate-400">
          Agents appear here once they send a heartbeat with a product tier.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {operators.map((op, opIdx) => (
        <div key={op.operator_id || `unassigned-${opIdx}`} className="card">
          <div className="flex items-center gap-3 mb-3 pb-3 border-b border-slate-700">
            <div className="p-2 rounded-lg bg-slate-800">
              <Network className="w-5 h-5 text-violet-400" />
            </div>
            <div className="min-w-0">
              <h3 className="font-semibold text-white truncate">{op.operator_name}</h3>
              <p className="text-xs text-slate-500">
                SOC operator{op.operator_id ? ` · ${op.operator_id}` : ""} ·{" "}
                {op.total_nodes} node{op.total_nodes === 1 ? "" : "s"}
              </p>
            </div>
          </div>

          {op.platform_roots.length > 0 && (
            <div className="space-y-0.5 mb-4">
              <p className="text-xs uppercase tracking-wide text-slate-500 px-2 mb-1">
                Platform (mysoc)
              </p>
              {op.platform_roots
                .filter((r) => matchesTier(r, tierFilter))
                .map((root) => (
                  <TreeNodeRow key={root.id} node={root} depth={0} tierFilter={tierFilter} />
                ))}
            </div>
          )}

          <div className="space-y-3">
            {op.customers
              .filter((c) => !filtering || c.roots.some((r) => matchesTier(r, tierFilter)))
              .map((customer, idx) => (
                <div
                  key={customer.license_id || `unlicensed-${idx}`}
                  className="rounded-lg border border-slate-700/70 bg-slate-900/40 p-3"
                >
                  <div className="flex items-center gap-2 mb-2">
                    <Building2 className="w-4 h-4 text-cyan-400 shrink-0" />
                    <span className="font-medium text-white text-sm truncate">
                      {customer.customer_name}
                    </span>
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
                      {customer.license_key ? ` · ${customer.license_key}` : " · no bound license"}
                    </span>
                  </div>
                  {customer.roots.length === 0 ? (
                    <p className="text-xs text-slate-500 px-2 py-1">
                      No instances enrolled yet.
                    </p>
                  ) : (
                    <div className="space-y-0.5">
                      {customer.roots
                        .filter((r) => matchesTier(r, tierFilter))
                        .map((root) => (
                          <TreeNodeRow key={root.id} node={root} depth={0} tierFilter={tierFilter} />
                        ))}
                    </div>
                  )}
                </div>
              ))}
          </div>
        </div>
      ))}
    </div>
  );
}
