"use client";

import { useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api, CustomerDirectoryResponse, CustomerSummaryRow } from "@/lib/api";
import {
  Building2,
  ChevronDown,
  ChevronRight,
  AlertTriangle,
  Server,
} from "lucide-react";
import Link from "next/link";
import { TierBadge } from "@/components/InstanceTree";

const PAGE_SIZE = 50;
const CHILD_PAGE = 50;

const SORTS = [
  { value: "exceptions", label: "Problems first" },
  { value: "nodes", label: "Most nodes" },
  { value: "name", label: "Name A–Z" },
];

// CustomerDirectory renders the fleet as a paged, searchable, exceptions-first
// list of customers with SQL-aggregated health — never a 20k-box tree. Each
// row expands to that customer's nodes via a bounded, parent/customer-filtered
// paged query, so drilling in stays O(page) too.
export function CustomerDirectory({ tier, search }: { tier: string; search: string }) {
  const [sort, setSort] = useState("exceptions");

  const { data, isLoading, isError, error, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useInfiniteQuery({
      queryKey: ["customer-directory", search, sort],
      initialPageParam: 0,
      queryFn: ({ pageParam }) =>
        api.getCustomerDirectory({
          search: search.trim() || undefined,
          sort,
          limit: PAGE_SIZE,
          offset: pageParam as number,
        }),
      getNextPageParam: (last: CustomerDirectoryResponse) => {
        const loaded = last.offset + last.items.length;
        return loaded < last.total ? loaded : undefined;
      },
    });

  const customers = data?.pages.flatMap((p) => p.items) ?? [];
  const total = data?.pages[0]?.total ?? 0;

  if (isLoading) return <div className="text-slate-400">Loading customer directory…</div>;
  if (isError) {
    return (
      <div className="card border-red-500/50 bg-red-900/20">
        <p className="text-red-400 font-medium">Failed to load customer directory</p>
        <p className="text-sm text-slate-400 mt-1">
          {error instanceof Error ? error.message : "Unknown error"}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-sm text-slate-500">
          {total.toLocaleString()} customer{total === 1 ? "" : "s"}
        </p>
        <select
          value={sort}
          onChange={(e) => setSort(e.target.value)}
          className="input py-1.5 text-sm"
          aria-label="Sort customers"
        >
          {SORTS.map((s) => (
            <option key={s.value} value={s.value}>
              {s.label}
            </option>
          ))}
        </select>
      </div>

      {customers.length === 0 ? (
        <div className="card text-center py-16">
          <Building2 className="w-12 h-12 text-slate-600 mx-auto mb-4" />
          <h3 className="text-lg font-medium text-white mb-2">No customers match</h3>
          <p className="text-slate-400">Adjust the search, or wait for the cascade to report.</p>
        </div>
      ) : (
        <div className="space-y-2">
          {customers.map((c) => (
            <CustomerRow key={c.customer_id || "__platform__"} customer={c} tier={tier} />
          ))}
        </div>
      )}

      {hasNextPage && (
        <div className="text-center pt-2">
          <button
            onClick={() => fetchNextPage()}
            disabled={isFetchingNextPage}
            className="btn btn-secondary"
          >
            {isFetchingNextPage ? "Loading…" : "Load more customers"}
          </button>
        </div>
      )}
    </div>
  );
}

function CustomerRow({ customer, tier }: { customer: CustomerSummaryRow; tier: string }) {
  const [open, setOpen] = useState(false);
  const name = customer.customer_name || (customer.customer_id ? customer.customer_id : "Platform / unassigned");
  const hasProblems = customer.failed > 0 || customer.offline > 0;

  return (
    <div className="rounded-lg border border-slate-700/70 bg-slate-900/40">
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center gap-3 p-3 text-left hover:bg-slate-800/40 rounded-lg"
      >
        {open ? (
          <ChevronDown className="w-4 h-4 text-slate-400 shrink-0" />
        ) : (
          <ChevronRight className="w-4 h-4 text-slate-400 shrink-0" />
        )}
        <Building2 className="w-4 h-4 text-cyan-400 shrink-0" />
        <span className="font-medium text-white text-sm truncate">{name}</span>
        {hasProblems && (
          <AlertTriangle className="w-4 h-4 text-amber-400 shrink-0" aria-label="Needs attention" />
        )}
        <div className="ml-auto flex items-center gap-3 text-xs shrink-0">
          <Count label="nodes" value={customer.total} className="text-slate-300" />
          <Count label="online" value={customer.online} className="text-emerald-400" />
          {customer.offline > 0 && (
            <Count label="offline" value={customer.offline} className="text-red-400" />
          )}
          {customer.failed > 0 && (
            <Count label="failed" value={customer.failed} className="text-amber-400" />
          )}
        </div>
      </button>
      {open && <CustomerChildren customerId={customer.customer_id} tier={tier} />}
    </div>
  );
}

function Count({ label, value, className }: { label: string; value: number; className: string }) {
  return (
    <span className={className} title={label}>
      {value.toLocaleString()} {label}
    </span>
  );
}

function CustomerChildren({ customerId, tier }: { customerId: string; tier: string }) {
  const {
    data,
    isLoading,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useInfiniteQuery({
    queryKey: ["customer-children", customerId, tier],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api.getInstancesFiltered({
        customer: customerId,
        tier: tier === "all" ? undefined : tier,
        sort: "product_tier",
        dir: "asc",
        limit: CHILD_PAGE,
        offset: pageParam as number,
      }),
    getNextPageParam: (last) => {
      const loaded = last.offset + last.items.length;
      return loaded < last.total ? loaded : undefined;
    },
  });

  const nodes = data?.pages.flatMap((p) => p.items) ?? [];

  if (isLoading) {
    return <div className="px-4 pb-3 text-xs text-slate-500">Loading nodes…</div>;
  }
  if (nodes.length === 0) {
    return <div className="px-4 pb-3 text-xs text-slate-500">No nodes reported.</div>;
  }

  return (
    <div className="px-3 pb-3 space-y-0.5">
      {nodes.map((n) => (
        <Link
          key={n.id}
          href={`/instances/${n.id}`}
          className="flex items-center gap-2 py-1.5 px-2 rounded-md hover:bg-slate-800/60"
        >
          <Server className="w-3.5 h-3.5 text-slate-500 shrink-0" />
          <TierBadge tier={n.product_tier} />
          <span className="text-sm text-white truncate">{n.instance_id}</span>
          {n.display_name && n.display_name !== n.instance_id && (
            <span className="text-xs text-slate-500 truncate">· {n.display_name}</span>
          )}
          <span
            className={`ml-auto text-xs ${
              n.status === "online"
                ? "text-emerald-400/80"
                : n.status === "offline"
                ? "text-red-400/80"
                : n.status === "degraded"
                ? "text-amber-400/80"
                : "text-slate-500"
            }`}
          >
            {n.status}
          </span>
        </Link>
      ))}
      {hasNextPage && (
        <button
          onClick={() => fetchNextPage()}
          disabled={isFetchingNextPage}
          className="text-xs text-cyan-400 hover:underline px-2 py-1"
        >
          {isFetchingNextPage ? "Loading…" : "Load more nodes"}
        </button>
      )}
    </div>
  );
}
