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
  MonitorSmartphone,
  ChevronRight,
  Key,
} from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import Link from "next/link";
import { ErrorState } from "@/components/ui";

// The dashboard mirrors the licensing hierarchy: operators (one platform
// key each) at the top, their cascaded fleet (mysoc > siemcore > swf) below.
export default function DashboardPage() {
  const {
    data: instances,
    isLoading: instancesLoading,
    isError: instancesError,
    error: instancesErrorObj,
    refetch: refetchInstances,
  } = useQuery({
    queryKey: ["instances"],
    queryFn: () => api.getInstances(),
    retry: false,
  });

  const { data: releases } = useQuery({
    queryKey: ["releases"],
    queryFn: () => api.getReleases(),
    retry: false,
  });

  const { data: operators, isLoading: operatorsLoading } = useQuery({
    queryKey: ["operators"],
    queryFn: () => api.getOperators(),
    retry: false,
  });

  const onlineCount = instances?.filter((i) => i.status === "online").length || 0;
  const offlineCount = instances?.filter((i) => i.status === "offline").length || 0;
  const degradedCount = instances?.filter((i) => i.status === "degraded").length || 0;

  const tierCount = (tier: string) =>
    instances?.filter((i) => (i.product_tier || "").toLowerCase() === tier).length || 0;

  const activeOperators = operators?.filter((o) => o.is_active).length || 0;

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
          siemcore &rsaquo; swf
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
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-6">
        <StatCard
          name="Operators"
          value={activeOperators}
          sub={
            operators && operators.length !== activeOperators
              ? `${operators.length - activeOperators} deactivated`
              : "active licenses"
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
          value={releases?.length || 0}
          sub="published artifacts"
          icon={Package}
          color="text-amber-400"
          bgColor="bg-amber-500/20"
          href="/releases"
        />
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

      {/* Recent Releases */}
      <div className="card">
        <h2 className="text-lg font-semibold text-white mb-4">Recent Releases</h2>
        <div className="table-container">
          <table className="table">
            <thead>
              <tr>
                <th>Product</th>
                <th>Version</th>
                <th>Channel</th>
                <th>Released</th>
              </tr>
            </thead>
            <tbody>
              {releases?.slice(0, 5).map((release) => (
                <tr key={release.id}>
                  <td className="font-medium text-white">{release.product_name}</td>
                  <td>
                    <code className="text-cyan-400">{release.version}</code>
                  </td>
                  <td>
                    <span className="px-2 py-1 rounded bg-slate-700 text-xs">
                      {release.channel}
                    </span>
                  </td>
                  <td className="text-slate-400">
                    {formatDistanceToNow(new Date(release.released_at), {
                      addSuffix: true,
                    })}
                  </td>
                </tr>
              ))}
              {(!releases || releases.length === 0) && (
                <tr>
                  <td colSpan={4} className="text-center text-slate-400 py-8">
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
