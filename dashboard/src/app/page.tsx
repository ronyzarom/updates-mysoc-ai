"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import {
  Server,
  Package,
  AlertTriangle,
  CheckCircle,
  XCircle,
  Building2,
  Boxes,
  Shield,
  MonitorSmartphone,
  ChevronRight,
  Key,
  BadgeCheck,
} from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import Link from "next/link";
import { ErrorState } from "@/components/ui";
import { sealCoverage, ringHeads, isNormalRelease, RINGS, type Ring } from "@/lib/derive";
import type { Release } from "@/lib/api";

const RING_PILL: Record<string, string> = {
  alpha: "bg-purple-500/20 text-purple-400",
  beta: "bg-amber-500/20 text-amber-400",
  stable: "bg-blue-500/20 text-blue-400",
  production: "bg-emerald-500/20 text-emerald-400",
};

// The dashboard mirrors the licensing hierarchy: operators (one platform
// key each) at the top, their cascaded fleet (mysoc > siemcore > swf) below.
export default function DashboardPage() {
  // Headline counts come from the SQL aggregate endpoint — the dashboard must
  // never pull the whole fleet to count it (100k+ nodes at scale).
  const {
    data: stats,
    isLoading: instancesLoading,
    isError: instancesError,
    error: instancesErrorObj,
    refetch: refetchInstances,
  } = useQuery({
    queryKey: ["fleet-stats"],
    queryFn: () => api.getFleetStats(),
    retry: false,
  });

  const {
    data: releases,
    isLoading: releasesLoading,
    isError: releasesError,
    error: releasesErrorObj,
    refetch: refetchReleases,
  } = useQuery({
    queryKey: ["releases"],
    queryFn: () => api.getReleases(),
    retry: false,
  });

  const {
    data: operators,
    isLoading: operatorsLoading,
    isError: operatorsError,
    error: operatorsErrorObj,
    refetch: refetchOperators,
  } = useQuery({
    queryKey: ["operators"],
    queryFn: () => api.getOperators(),
    retry: false,
  });

  const onlineCount = stats?.online || 0;
  const offlineCount = stats?.offline || 0;
  const degradedCount = stats?.degraded || 0;

  const tierCount = (tier: string) => stats?.by_tier?.[tier] || 0;

  const activeOperators = operators?.filter((o) => o.is_active).length || 0;
  const coverage = sealCoverage(releases);
  const normalReleaseCount = (releases || []).filter(isNormalRelease).length;

  // Ring heads per product, and which products have no stable release (which
  // breaks fresh installs, since new servers enrol into stable).
  const releasesByProduct = (releases || []).reduce((acc, r) => {
    (acc[r.product_name] ||= []).push(r);
    return acc;
  }, {} as Record<string, Release[]>);
  const ringHeadProducts = Object.keys(releasesByProduct)
    .sort()
    .map((product) => ({ product, heads: ringHeads(releasesByProduct[product]!) }));
  const productsMissingStable = ringHeadProducts.filter((p) => !p.heads.stable).map((p) => p.product);

  const sortedOperators = [...(operators || [])].sort((a, b) => {
    const ta = a.last_heartbeat ? new Date(a.last_heartbeat).getTime() : 0;
    const tb = b.last_heartbeat ? new Date(b.last_heartbeat).getTime() : 0;
    return tb - ta;
  });

  return (
    <div className="space-y-8">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold text-white">Dashboard</h1>
        <p className="text-slate-400 mt-1">
          One license per operator, one cascade per fleet: mysoc &rsaquo;
          siemcore / widecore &rsaquo; swf
        </p>
      </div>

      {instancesError && (
        <ErrorState
          title="Failed to load fleet data"
          error={instancesErrorObj}
          onRetry={() => refetchInstances()}
        />
      )}

      {/* Hierarchy stats: operators first, then fleet by tier */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-6">
        <StatCard
          name="Operators"
          value={activeOperators}
          sub={
            operators && operators.length !== activeOperators
              ? `${operators.length - activeOperators} deactivated`
              : "all active"
          }
          icon={Building2}
          color="text-violet-400"
          bgColor="bg-violet-500/20"
          href="/operators"
        />
        <StatCard
          name="MySoc Platforms"
          value={tierCount("mysoc")}
          sub="direct to this server"
          icon={Server}
          color="text-cyan-400"
          bgColor="bg-cyan-500/20"
          href="/instances"
        />
        <StatCard
          name="SiemCore Servers"
          value={tierCount("siemcore")}
          sub="via mysoc relay"
          icon={Boxes}
          color="text-sky-400"
          bgColor="bg-sky-500/20"
          href="/instances"
        />
        <StatCard
          name="WideCore Gateways"
          value={tierCount("widecore")}
          sub="via mysoc relay"
          icon={Shield}
          color="text-indigo-400"
          bgColor="bg-indigo-500/20"
          href="/instances"
        />
        <StatCard
          name="SWF Agents"
          value={tierCount("swf")}
          sub="via siemcore relay"
          icon={MonitorSmartphone}
          color="text-emerald-400"
          bgColor="bg-emerald-500/20"
          href="/instances"
        />
        <StatCard
          name="Releases"
          value={releasesError ? 0 : normalReleaseCount}
          sub={releasesError ? "failed to load" : "normal releases"}
          icon={Package}
          color="text-amber-400"
          bgColor="bg-amber-500/20"
          href="/releases"
        />
      </div>

      {operatorsError && (
        <ErrorState title="Failed to load operators" error={operatorsErrorObj} onRetry={() => refetchOperators()} />
      )}

      {/* Ring heads: what each ring currently offers, and stable gaps */}
      <div className="card">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-white">Ring Heads</h2>
          <Link href="/releases" className="text-sm text-cyan-400 hover:underline">
            View releases
          </Link>
        </div>
        {productsMissingStable.length > 0 && (
          <div className="mb-4 p-3 rounded-lg bg-red-500/15 border border-red-500/50 text-red-300 text-sm flex items-start gap-2">
            <AlertTriangle className="w-4 h-4 mt-0.5 shrink-0" />
            <span>
              No stable release for {productsMissingStable.join(", ")}. A fresh install enrols into stable, so it would
              hang with nothing to install.
            </span>
          </div>
        )}
        {releasesLoading ? (
          <p className="text-sm text-slate-400">Loading…</p>
        ) : releasesError ? (
          <ErrorState title="Failed to load releases" error={releasesErrorObj} onRetry={() => refetchReleases()} />
        ) : ringHeadProducts.length === 0 ? (
          <p className="text-sm text-slate-400">No releases yet.</p>
        ) : (
          <div className="space-y-2">
            {ringHeadProducts.map(({ product, heads }) => (
              <div key={product} className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium text-white w-40 truncate">{product}</span>
                {RINGS.map((ring: Ring) => (
                  <span
                    key={ring}
                    className={`px-2.5 py-1 rounded text-xs font-medium ${
                      heads[ring] ? RING_PILL[ring] : "bg-red-500/20 text-red-400 border border-red-500/40"
                    }`}
                  >
                    {ring} → {heads[ring] ? heads[ring]!.version : "none"}
                  </span>
                ))}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Status Overview + Operator cascades */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Fleet health */}
        <div className="card">
          <h2 className="text-lg font-semibold text-white mb-4">Fleet Health</h2>
          <div className="space-y-4">
            <div className="flex items-center justify-between p-3 rounded-lg bg-emerald-500/10">
              <div className="flex items-center gap-3">
                <CheckCircle className="w-5 h-5 text-emerald-400" />
                <span className="text-slate-300">Online</span>
              </div>
              <span className="text-2xl font-bold text-emerald-400">{onlineCount}</span>
            </div>
            <div className="flex items-center justify-between p-3 rounded-lg bg-amber-500/10">
              <div className="flex items-center gap-3">
                <AlertTriangle className="w-5 h-5 text-amber-400" />
                <span className="text-slate-300">Degraded</span>
              </div>
              <span className="text-2xl font-bold text-amber-400">{degradedCount}</span>
            </div>
            <div className="flex items-center justify-between p-3 rounded-lg bg-red-500/10">
              <div className="flex items-center gap-3">
                <XCircle className="w-5 h-5 text-red-400" />
                <span className="text-slate-300">Offline</span>
              </div>
              <span className="text-2xl font-bold text-red-400">{offlineCount}</span>
            </div>
          </div>
        </div>

        {/* Operator cascades */}
        <div className="card">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold text-white">Operator Cascades</h2>
            <Link href="/operators" className="text-sm text-cyan-400 hover:underline">
              Manage licenses
            </Link>
          </div>
          <div className="space-y-3">
            {operatorsLoading || instancesLoading ? (
              <div className="text-slate-400 text-sm">Loading...</div>
            ) : sortedOperators.length === 0 ? (
              <div className="text-center py-8">
                <Key className="w-10 h-10 text-slate-600 mx-auto mb-3" />
                <p className="text-slate-400 text-sm mb-3">
                  No operators yet — issue the first platform license to bring a
                  fleet online.
                </p>
                <Link href="/operators" className="btn btn-primary btn-sm">
                  Create operator
                </Link>
              </div>
            ) : (
              sortedOperators.slice(0, 6).map((op) => (
                <Link
                  key={op.id}
                  href={`/operators/${encodeURIComponent(op.id)}`}
                  className="flex items-center gap-3 p-3 rounded-lg bg-slate-800/50 hover:bg-slate-800 transition-colors"
                >
                  <Building2 className="w-4 h-4 text-violet-400 shrink-0" />
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium text-white truncate">
                      {op.name}
                    </p>
                    <p className="text-xs text-slate-500">
                      {op.last_heartbeat
                        ? `reported ${formatDistanceToNow(new Date(op.last_heartbeat), { addSuffix: true })}`
                        : "never reported"}
                    </p>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-slate-400 shrink-0">
                    <span title="mysoc platforms">
                      {op.nodes_by_tier?.["mysoc"] ?? 0}
                    </span>
                    <ChevronRight className="w-3 h-3 text-slate-600" />
                    <span title="siemcore servers">
                      {op.nodes_by_tier?.["siemcore"] ?? 0}
                    </span>
                    <span className="text-slate-600">/</span>
                    <span title="widecore gateways">
                      {op.nodes_by_tier?.["widecore"] ?? 0}
                    </span>
                    <ChevronRight className="w-3 h-3 text-slate-600" />
                    <span title="swf agents">{op.nodes_by_tier?.["swf"] ?? 0}</span>
                  </div>
                  {!op.is_active && (
                    <span className="status-badge status-offline shrink-0">off</span>
                  )}
                </Link>
              ))
            )}
          </div>
        </div>
      </div>

      {/* Issuer seals */}
      <div className="card">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <BadgeCheck className="w-5 h-5 text-emerald-400" />
            <h2 className="text-lg font-semibold text-white">Issuer Seals</h2>
            <span className="text-sm text-slate-400">last 30 days</span>
          </div>
          <Link href="/releases" className="text-sm text-cyan-400 hover:underline">
            View releases
          </Link>
        </div>
        {coverage.length === 0 ? (
          <p className="text-sm text-slate-400">No releases published in the last 30 days.</p>
        ) : (
          <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-4">
            {coverage.map((row) => (
              <div key={row.issuer} className="p-4 rounded-lg bg-slate-800/50" data-issuer={row.issuer}>
                <p className="text-xs text-slate-500 mb-1">{row.issuer}</p>
                <p className="text-2xl font-bold text-white">{row.percentSealed}%</p>
                <p className="text-xs text-slate-500 mt-1">
                  {row.sealed} of {row.total} sealed
                </p>
                {row.invalid > 0 && (
                  <p className="text-xs text-red-400 mt-1">{row.invalid} invalid</p>
                )}
                <div className="mt-2 h-1.5 rounded bg-slate-700 overflow-hidden">
                  <div className="h-full bg-emerald-500" style={{ width: `${row.percentSealed}%` }} />
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Recent Releases */}
      <div className="card">
        <h2 className="text-lg font-semibold text-white mb-4">Recent Releases</h2>
        <div className="table-container">
          <table className="table">
            <thead>
              <tr>
                <th>Product</th>
                <th>Version</th>
                <th>Kind</th>
                <th>Target Groups</th>
                <th>Released</th>
              </tr>
            </thead>
            <tbody>
              {releases?.slice(0, 5).map((release) => {
                const kind = release.manifest?.artifact_variants?.length
                  ? "pair"
                  : release.manifest?.artifact_kind || "release";
                const groups = release.target_groups || [];
                return (
                  <tr key={release.id}>
                    <td className="font-medium text-white">{release.product_name}</td>
                    <td>
                      <code className="text-cyan-400">{release.version}</code>
                    </td>
                    <td className="text-slate-400 text-xs">{kind}</td>
                    <td>
                      {groups.length === 0 ? (
                        <span className="px-2 py-1 rounded text-xs bg-red-500/20 text-red-400 border border-red-500/40">
                          no ring
                        </span>
                      ) : (
                        <span className="px-2 py-1 rounded bg-slate-700 text-xs">{groups.join(", ")}</span>
                      )}
                    </td>
                    <td className="text-slate-400">
                      {formatDistanceToNow(new Date(release.released_at), { addSuffix: true })}
                    </td>
                  </tr>
                );
              })}
              {releasesError && (
                <tr>
                  <td colSpan={5} className="text-center text-red-400 py-8">
                    Failed to load releases
                  </td>
                </tr>
              )}
              {!releasesError && (!releases || releases.length === 0) && (
                <tr>
                  <td colSpan={5} className="text-center text-slate-400 py-8">
                    No releases yet
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}

function StatCard({
  name,
  value,
  sub,
  icon: Icon,
  color,
  bgColor,
  href,
}: {
  name: string;
  value: number;
  sub: string;
  icon: typeof Server;
  color: string;
  bgColor: string;
  href: string;
}) {
  return (
    <Link href={href} className="card card-hover">
      <div className="flex items-start justify-between">
        <div>
          <p className="text-slate-400 text-sm">{name}</p>
          <p className="text-3xl font-bold text-white mt-2">{value}</p>
          <p className="text-xs text-slate-500 mt-1">{sub}</p>
        </div>
        <div className={`p-3 rounded-lg ${bgColor}`}>
          <Icon className={`w-6 h-6 ${color}`} />
        </div>
      </div>
    </Link>
  );
}
