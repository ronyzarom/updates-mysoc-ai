"use client";

import { useQuery } from "@tanstack/react-query";
import { api, InstanceTreeNode } from "@/lib/api";
import { ChevronDown, ChevronRight, Server, RefreshCw, Building2, AlertTriangle } from "lucide-react";
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
          <span className="flex items-center gap-1 text-[10px] text-amber-400" title="Declared parent not found in this customer">
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

  const customers = (data?.customers || []).filter((c) =>
    c.roots.some((r) => matchesTier(r, tierFilter))
  );

  if (customers.length === 0) {
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
      {customers.map((customer, idx) => (
        <div key={customer.license_id || `unlicensed-${idx}`} className="card">
          <div className="flex items-center gap-3 mb-3 pb-3 border-b border-slate-700">
            <div className="p-2 rounded-lg bg-slate-800">
              <Building2 className="w-5 h-5 text-cyan-400" />
            </div>
            <div className="min-w-0">
              <h3 className="font-semibold text-white truncate">{customer.customer_name}</h3>
              <p className="text-xs text-slate-500">
                {customer.total_nodes} node{customer.total_nodes === 1 ? "" : "s"}
                {customer.license_key ? ` · license ${customer.license_key}` : " · no bound license"}
              </p>
            </div>
          </div>
          <div className="space-y-0.5">
            {customer.roots
              .filter((r) => matchesTier(r, tierFilter))
              .map((root) => (
                <TreeNodeRow key={root.id} node={root} depth={0} tierFilter={tierFilter} />
              ))}
          </div>
        </div>
      ))}
    </div>
  );
}
