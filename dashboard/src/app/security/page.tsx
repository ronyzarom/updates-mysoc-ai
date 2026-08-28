"use client";

import { useEffect, useRef } from "react";
import { useQuery, useInfiniteQuery } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { api, SecurityPagedResponse } from "@/lib/api";
import {
  Shield,
  AlertTriangle,
  CheckCircle,
  XCircle,
  RefreshCw,
  Flame,
  Lock,
  FileCheck,
} from "lucide-react";
import { ErrorState } from "@/components/ui";

const PAGE_SIZE = 100;
const ROW_HEIGHT = 56;

export default function SecurityPage() {
  const {
    data: stats,
    isError: statsError,
    error: statsErrObj,
    refetch: refetchStats,
  } = useQuery({
    queryKey: ["security-stats"],
    queryFn: () => api.getSecurityStats(),
    retry: false,
  });

  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useInfiniteQuery({
    queryKey: ["security-list"],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => api.getSecurityPaged(PAGE_SIZE, pageParam as number),
    getNextPageParam: (last: SecurityPagedResponse) => {
      const loaded = last.offset + last.items.length;
      return loaded < last.total ? loaded : undefined;
    },
    retry: false,
  });

  const rows = data?.pages.flatMap((p) => p.items) ?? [];
  const reporting = stats?.reporting ?? 0;
  const avgScore = stats?.avg_score ?? 0;

  const parentRef = useRef<HTMLDivElement>(null);
  const rowCount = hasNextPage ? rows.length + 1 : rows.length;
  const virtualizer = useVirtualizer({
    count: rowCount,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 10,
  });
  const virtualItems = virtualizer.getVirtualItems();
  useEffect(() => {
    const last = virtualItems[virtualItems.length - 1];
    if (!last) return;
    if (last.index >= rows.length - 1 && hasNextPage && !isFetchingNextPage) {
      fetchNextPage();
    }
  }, [virtualItems, rows.length, hasNextPage, isFetchingNextPage, fetchNextPage]);

  const getScoreColor = (score: number) => {
    if (score >= 80) return "text-emerald-400";
    if (score >= 60) return "text-amber-400";
    return "text-red-400";
  };
  const getScoreBg = (score: number) => {
    if (score >= 80) return "bg-emerald-500/20";
    if (score >= 60) return "bg-amber-500/20";
    return "bg-red-500/20";
  };

  const refreshAll = () => {
    refetchStats();
    refetch();
  };

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-white">Security</h1>
          <p className="text-slate-400 mt-1">Fleet security posture and compliance</p>
        </div>
        <button onClick={refreshAll} className="btn btn-secondary">
          <RefreshCw className="w-4 h-4" />
          Refresh
        </button>
      </div>

      {statsError && (
        <ErrorState
          title="Failed to load security summary"
          error={statsErrObj}
          onRetry={() => refetchStats()}
        />
      )}

      {/* Security Score Overview — SQL-aggregated over the whole fleet. */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="card lg:col-span-1">
          <div className="flex flex-col items-center justify-center py-8">
            <div
              className={`w-32 h-32 rounded-full ${
                reporting > 0 ? getScoreBg(avgScore) : "bg-slate-700/40"
              } flex items-center justify-center mb-4`}
            >
              <span
                className={`text-5xl font-bold ${
                  reporting > 0 ? getScoreColor(avgScore) : "text-slate-500"
                }`}
              >
                {reporting > 0 ? avgScore : "—"}
              </span>
            </div>
            <p className="text-slate-400 text-sm">Average Security Score</p>
            <p className="text-white font-medium mt-1">
              {reporting > 0
                ? `${reporting.toLocaleString()} instance${reporting === 1 ? "" : "s"} reporting`
                : "No telemetry yet"}
            </p>
          </div>
        </div>

        <div className="lg:col-span-2 grid grid-cols-2 gap-4">
          <PostureCard
            icon={<Flame className="w-5 h-5 text-emerald-400" />}
            bg="bg-emerald-500/20"
            value={`${(stats?.firewall_enabled ?? 0).toLocaleString()}/${reporting.toLocaleString()}`}
            label="Firewall Enabled"
          />
          <PostureCard
            icon={<Lock className="w-5 h-5 text-cyan-400" />}
            bg="bg-cyan-500/20"
            value={`${(stats?.ssh_hardened ?? 0).toLocaleString()}/${reporting.toLocaleString()}`}
            label="SSH Hardened"
          />
          <PostureCard
            icon={<FileCheck className="w-5 h-5 text-amber-400" />}
            bg="bg-amber-500/20"
            value={(stats?.pending_updates ?? 0).toLocaleString()}
            label="Pending Updates"
          />
          <PostureCard
            icon={<AlertTriangle className="w-5 h-5 text-red-400" />}
            bg="bg-red-500/20"
            value={(stats?.reboot_required ?? 0).toLocaleString()}
            label="Reboot Required"
          />
        </div>
      </div>

      {/* Per-instance security status — paged, worst score first. */}
      <div className="card">
        <h2 className="text-lg font-semibold text-white mb-4">
          Instance Security Status
          <span className="text-sm font-normal text-slate-500 ml-2">
            worst score first
          </span>
        </h2>

        {isLoading ? (
          <div className="text-slate-400">Loading…</div>
        ) : isError ? (
          <ErrorState
            title="Failed to load security data"
            error={error}
            onRetry={() => refetch()}
          />
        ) : rows.length === 0 ? (
          <div className="text-center py-16">
            <Shield className="w-12 h-12 text-slate-600 mx-auto mb-4" />
            <h3 className="text-lg font-medium text-white mb-2">No security data</h3>
            <p className="text-slate-400">
              Security status will appear once instances start reporting.
            </p>
          </div>
        ) : (
          <div
            ref={parentRef}
            className="overflow-auto rounded-lg border border-slate-800"
            style={{ height: "60vh", contain: "strict" }}
          >
            <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
              {virtualItems.map((v) => {
                const isSentinel = v.index >= rows.length;
                const row = rows[v.index];
                const score = row?.security?.security_score ?? 0;
                return (
                  <div
                    key={v.key}
                    style={{
                      position: "absolute",
                      top: 0,
                      left: 0,
                      width: "100%",
                      height: ROW_HEIGHT,
                      transform: `translateY(${v.start}px)`,
                    }}
                    className="flex items-center gap-4 px-4 border-b border-slate-800"
                  >
                    {isSentinel ? (
                      <span className="text-sm text-slate-500 mx-auto">
                        {isFetchingNextPage ? "Loading more…" : "Scroll for more"}
                      </span>
                    ) : (
                      <>
                        <div className="flex items-center gap-2 min-w-0 flex-1">
                          <Shield className="w-4 h-4 text-slate-400 shrink-0" />
                          <span className="font-medium text-white truncate">
                            {row.instance_id}
                          </span>
                        </div>
                        <div
                          className={`w-8 h-8 rounded-full ${getScoreBg(score)} flex items-center justify-center shrink-0`}
                        >
                          <span className={`text-sm font-bold ${getScoreColor(score)}`}>
                            {score}
                          </span>
                        </div>
                        <BoolIcon on={row.security?.firewall_enabled} title="Firewall" />
                        <BoolIcon on={row.security?.ssh_hardened} title="SSH hardened" />
                        <span className="w-24 text-right text-sm shrink-0">
                          {(row.security?.pending_updates || 0) > 0 ? (
                            <span className="text-amber-400">
                              {row.security?.pending_updates} pending
                            </span>
                          ) : (
                            <span className="text-emerald-400">Up to date</span>
                          )}
                        </span>
                        <span className="w-20 text-right shrink-0">
                          {row.security?.reboot_required ? (
                            <span className="status-badge status-degraded">Reboot</span>
                          ) : (
                            <span className="text-slate-500 text-sm">—</span>
                          )}
                        </span>
                      </>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function PostureCard({
  icon,
  bg,
  value,
  label,
}: {
  icon: React.ReactNode;
  bg: string;
  value: string;
  label: string;
}) {
  return (
    <div className="card">
      <div className="flex items-start gap-3">
        <div className={`p-2 rounded-lg ${bg}`}>{icon}</div>
        <div>
          <p className="text-2xl font-bold text-white">{value}</p>
          <p className="text-sm text-slate-400">{label}</p>
        </div>
      </div>
    </div>
  );
}

function BoolIcon({ on, title }: { on?: boolean; title: string }) {
  return (
    <span className="shrink-0" title={title}>
      {on ? (
        <CheckCircle className="w-5 h-5 text-emerald-400" />
      ) : (
        <XCircle className="w-5 h-5 text-red-400" />
      )}
    </span>
  );
}
