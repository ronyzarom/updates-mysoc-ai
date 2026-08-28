"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, InstanceQuery } from "@/lib/api";
import { RefreshCw, LayoutGrid, Building2, Search } from "lucide-react";
import { CustomerDirectory } from "@/components/CustomerDirectory";
import { InstancesList } from "@/components/InstancesList";

const TIER_FILTERS = [
  { value: "all", label: "All tiers" },
  { value: "mysoc", label: "MySoc" },
  { value: "siemcore", label: "SiemCore" },
  { value: "swf", label: "SWF" },
];

const STATUS_FILTERS = [
  { value: "all", label: "All statuses" },
  { value: "online", label: "Online" },
  { value: "offline", label: "Offline" },
  { value: "degraded", label: "Degraded" },
  { value: "decommissioned", label: "Decommissioned" },
];

const SORT_OPTIONS = [
  { value: "created_at:desc", label: "Newest" },
  { value: "last_heartbeat:desc", label: "Recently seen" },
  { value: "hostname:asc", label: "Hostname A–Z" },
  { value: "instance_id:asc", label: "Instance ID A–Z" },
  { value: "status:asc", label: "Status" },
];

// useDebounced returns a value that only updates after it has been stable for
// `delay` ms — keeps typing in the search box from firing a query per keystroke
// against a 500k-row table.
function useDebounced<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(t);
  }, [value, delay]);
  return debounced;
}

export default function InstancesPage() {
  const [view, setView] = useState<"customers" | "list">("list");
  const [tierFilter, setTierFilter] = useState<string>("all");
  const [statusFilter, setStatusFilter] = useState<string>("all");
  const [sortKey, setSortKey] = useState<string>("created_at:desc");
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebounced(search);
  const [refreshTick, setRefreshTick] = useState(0);

  const [sort, dir] = sortKey.split(":") as [string, "asc" | "desc"];

  // Fleet-wide counts come from the SQL aggregate endpoint, not from counting
  // rows on the client — the same filters narrow it.
  const statsQuery: InstanceQuery = {
    tier: tierFilter === "all" ? undefined : tierFilter,
    status: statusFilter === "all" ? undefined : statusFilter,
    search: debouncedSearch.trim() || undefined,
  };
  const { data: stats } = useQuery({
    queryKey: ["fleet-stats", tierFilter, statusFilter, debouncedSearch, refreshTick],
    queryFn: () => api.getFleetStats(statsQuery),
  });

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between flex-wrap gap-3">
        <div>
          <h1 className="text-3xl font-bold text-white">Fleet</h1>
          <p className="text-slate-400 mt-1">
            The update cascade per operator: MySoc relays &rsaquo; SiemCore relays &rsaquo; SWF agents
          </p>
        </div>
        <div className="flex items-center gap-3 flex-wrap">
          <div className="relative">
            <Search className="w-4 h-4 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" />
            <input
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search instance, host, customer…"
              className="input py-2 pl-9 w-64"
              aria-label="Search fleet"
            />
          </div>
          <select
            value={tierFilter}
            onChange={(e) => setTierFilter(e.target.value)}
            className="input py-2"
            aria-label="Filter by product tier"
          >
            {TIER_FILTERS.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="input py-2"
            aria-label="Filter by status"
          >
            {STATUS_FILTERS.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
          <select
            value={sortKey}
            onChange={(e) => setSortKey(e.target.value)}
            className="input py-2"
            aria-label="Sort order"
          >
            {SORT_OPTIONS.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
          <div className="flex rounded-lg border border-slate-700 overflow-hidden">
            <button
              onClick={() => setView("customers")}
              className={`flex items-center gap-1.5 px-3 py-2 text-sm ${
                view === "customers" ? "bg-slate-700 text-white" : "bg-transparent text-slate-400 hover:text-white"
              }`}
              aria-pressed={view === "customers"}
            >
              <Building2 className="w-4 h-4" />
              Customers
            </button>
            <button
              onClick={() => setView("list")}
              className={`flex items-center gap-1.5 px-3 py-2 text-sm ${
                view === "list" ? "bg-slate-700 text-white" : "bg-transparent text-slate-400 hover:text-white"
              }`}
              aria-pressed={view === "list"}
            >
              <LayoutGrid className="w-4 h-4" />
              List
            </button>
          </div>
          <button onClick={() => setRefreshTick((t) => t + 1)} className="btn btn-secondary">
            <RefreshCw className="w-4 h-4" />
            Refresh
          </button>
        </div>
      </div>

      {/* SQL-aggregated headline counts (never counts rows client-side). */}
      {stats && (
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
          <StatCard label="Total" value={stats.total} />
          <StatCard label="Online" value={stats.online} tone="online" />
          <StatCard label="Offline" value={stats.offline} tone="offline" />
          <StatCard label="Failed updates" value={stats.failed_updates} tone="warn" />
          <StatCard label="Decommissioned" value={stats.decommissioned} />
        </div>
      )}

      {view === "customers" ? (
        <CustomerDirectory tier={tierFilter} search={debouncedSearch} />
      ) : (
        <InstancesList
          key={refreshTick}
          tier={tierFilter}
          status={statusFilter}
          search={debouncedSearch}
          sort={sort}
          dir={dir}
        />
      )}
    </div>
  );
}

function StatCard({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone?: "online" | "offline" | "warn";
}) {
  const toneClass =
    tone === "online"
      ? "text-emerald-400"
      : tone === "offline"
      ? "text-red-400"
      : tone === "warn"
      ? "text-amber-400"
      : "text-white";
  return (
    <div className="card py-3">
      <p className="text-xs text-slate-400">{label}</p>
      <p className={`text-2xl font-bold ${toneClass}`}>{value.toLocaleString()}</p>
    </div>
  );
}

