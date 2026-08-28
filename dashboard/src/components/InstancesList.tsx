"use client";

import { useEffect, useRef } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { api, InstancesPagedResponse } from "@/lib/api";
import { Server, Clock } from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import Link from "next/link";
import { TierBadge } from "@/components/InstanceTree";

const PAGE_SIZE = 100;
const ROW_HEIGHT = 84; // px, must match the rendered row height for windowing

export interface InstancesListProps {
  tier: string; // "all" or a product tier
  status: string; // "all" or a derived status
  search: string;
  sort: string;
  dir: "asc" | "desc";
}

// InstancesList renders the fleet as a virtualized, server-paged list. Only a
// page of rows is fetched at a time and only the visible window is in the DOM,
// so the view stays flat whether the fleet is 50 nodes or 500,000.
export function InstancesList({ tier, status, search, sort, dir }: InstancesListProps) {
  const parentRef = useRef<HTMLDivElement>(null);

  const {
    data,
    isLoading,
    isError,
    error,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useInfiniteQuery({
    queryKey: ["instances-list", tier, status, search, sort, dir],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api.getInstancesFiltered({
        tier: tier === "all" ? undefined : tier,
        status: status === "all" ? undefined : status,
        search: search.trim() || undefined,
        sort,
        dir,
        limit: PAGE_SIZE,
        offset: pageParam as number,
      }),
    getNextPageParam: (last: InstancesPagedResponse) => {
      const loaded = last.offset + last.items.length;
      return loaded < last.total ? loaded : undefined;
    },
  });

  const items = data?.pages.flatMap((p) => p.items) ?? [];
  const total = data?.pages[0]?.total ?? 0;

  // Render one extra virtual row as a sentinel while more pages remain, so the
  // "loading more" affordance and the fetch trigger have a slot.
  const rowCount = hasNextPage ? items.length + 1 : items.length;

  const virtualizer = useVirtualizer({
    count: rowCount,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  });

  // Fetch the next page as the sentinel row scrolls into view.
  const virtualItems = virtualizer.getVirtualItems();
  useEffect(() => {
    const last = virtualItems[virtualItems.length - 1];
    if (!last) return;
    if (last.index >= items.length - 1 && hasNextPage && !isFetchingNextPage) {
      fetchNextPage();
    }
  }, [virtualItems, items.length, hasNextPage, isFetchingNextPage, fetchNextPage]);

  if (isLoading) {
    return <div className="text-slate-400">Loading instances...</div>;
  }
  if (isError) {
    return (
      <div className="card border-red-500/50 bg-red-900/20">
        <p className="text-red-400 font-medium">Failed to load instances</p>
        <p className="text-sm text-slate-400 mt-1">
          {error instanceof Error ? error.message : "Unknown error"}
        </p>
      </div>
    );
  }
  if (items.length === 0) {
    return (
      <div className="text-center py-16">
        <Server className="w-12 h-12 text-slate-600 mx-auto mb-4" />
        <h3 className="text-lg font-medium text-white mb-2">No matching instances</h3>
        <p className="text-slate-400">
          Adjust the filters, or wait for instances to connect to the update server.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-slate-500">
        {total.toLocaleString()} instance{total === 1 ? "" : "s"}
        {" · showing "}
        {items.length.toLocaleString()}
      </p>
      <div
        ref={parentRef}
        className="rounded-xl border border-slate-800 overflow-auto"
        style={{ height: "70vh", contain: "strict" }}
      >
        <div style={{ height: virtualizer.getTotalSize(), position: "relative", width: "100%" }}>
          {virtualItems.map((vRow) => {
            const isSentinel = vRow.index >= items.length;
            const instance = items[vRow.index];
            return (
              <div
                key={vRow.key}
                data-index={vRow.index}
                ref={virtualizer.measureElement}
                style={{
                  position: "absolute",
                  top: 0,
                  left: 0,
                  width: "100%",
                  transform: `translateY(${vRow.start}px)`,
                }}
              >
                {isSentinel ? (
                  <div className="px-4 py-6 text-center text-sm text-slate-500">
                    {isFetchingNextPage ? "Loading more…" : "Scroll for more"}
                  </div>
                ) : (
                  <Link
                    href={`/instances/${instance.id}`}
                    className="flex items-center justify-between gap-4 px-4 py-4 border-b border-slate-800 hover:bg-slate-800/50 transition-colors"
                    style={{ height: ROW_HEIGHT }}
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="p-2 rounded-lg bg-slate-800 shrink-0">
                        <Server className="w-5 h-5 text-cyan-400" />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <h3 className="font-semibold text-white truncate">
                            {instance.instance_id}
                          </h3>
                          {instance.product_tier && <TierBadge tier={instance.product_tier} />}
                        </div>
                        <p className="text-sm text-slate-500 truncate">
                          {instance.display_name || instance.hostname || instance.last_ip_address || "—"}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-4 shrink-0">
                      {instance.last_heartbeat && (
                        <span className="text-xs text-slate-400 hidden sm:flex items-center gap-1">
                          <Clock className="w-3 h-3" />
                          {formatDistanceToNow(new Date(instance.last_heartbeat), {
                            addSuffix: true,
                          })}
                        </span>
                      )}
                      <StatusBadge status={instance.status} />
                    </div>
                  </Link>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const styles = {
    online: "status-badge status-online",
    offline: "status-badge status-offline",
    degraded: "status-badge status-degraded",
    decommissioned: "status-badge bg-slate-500/20 text-slate-400",
    unknown: "status-badge bg-slate-500/20 text-slate-400",
  };
  return (
    <span className={styles[status as keyof typeof styles] || styles.unknown}>
      {status}
    </span>
  );
}
