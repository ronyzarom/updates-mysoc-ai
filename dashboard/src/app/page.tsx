"use client";

import { useQuery } from "@tanstack/react-query";
import { api, Instance, Release } from "@/lib/api";
import {
  Server,
  Package,
  Key,
  Shield,
  AlertTriangle,
  CheckCircle,
  XCircle,
  Clock,
} from "lucide-react";
import { formatDistanceToNow } from "date-fns";

export default function DashboardPage() {
  const { data: instances, isLoading: instancesLoading } = useQuery({
    queryKey: ["instances"],
    queryFn: () => api.getInstances(),
  });

  const { data: releases } = useQuery({
    queryKey: ["releases"],
    queryFn: () => api.getReleases(),
  });

  const { data: licenses } = useQuery({
    queryKey: ["licenses"],
    queryFn: () => api.getLicenses(),
  });

  const onlineCount = instances?.filter((i) => i.status === "online").length || 0;
  const offlineCount = instances?.filter((i) => i.status === "offline").length || 0;
  const degradedCount = instances?.filter((i) => i.status === "degraded").length || 0;

  const stats = [
    {
      name: "Total Instances",
      value: instances?.length || 0,
      icon: Server,
      color: "text-cyan-400",
      bgColor: "bg-cyan-500/20",
    },
    {
      name: "Total Releases",
      value: releases?.length || 0,
      icon: Package,
      color: "text-violet-400",
      bgColor: "bg-violet-500/20",
    },
    {
      name: "Active Licenses",
      value: licenses?.filter((l) => l.is_active).length || 0,
      icon: Key,
      color: "text-amber-400",
      bgColor: "bg-amber-500/20",
    },
    {
      name: "Online Instances",
      value: onlineCount,
      icon: CheckCircle,
      color: "text-emerald-400",
      bgColor: "bg-emerald-500/20",
    },
  ];

  return (
    <div className="space-y-8">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold text-white">Dashboard</h1>
        <p className="text-slate-400 mt-1">
          Fleet overview and system status
        </p>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        {stats.map((stat) => (
          <div key={stat.name} className="card">
            <div className="flex items-start justify-between">
              <div>
                <p className="text-slate-400 text-sm">{stat.name}</p>
                <p className="text-3xl font-bold text-white mt-2">{stat.value}</p>
              </div>
              <div className={`p-3 rounded-lg ${stat.bgColor}`}>
                <stat.icon className={`w-6 h-6 ${stat.color}`} />
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Status Overview */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Instance Status */}
        <div className="card">
          <h2 className="text-lg font-semibold text-white mb-4">Instance Status</h2>
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

        {/* Recent Activity */}
        <div className="card">
          <h2 className="text-lg font-semibold text-white mb-4">Recent Activity</h2>
          <div className="space-y-3">
            {instancesLoading ? (
              <div className="text-slate-400 text-sm">Loading...</div>
            ) : instances?.slice(0, 5).map((instance) => (
              <div
                key={instance.id}
                className="flex items-center justify-between p-3 rounded-lg bg-slate-800/50"
              >
                <div className="flex items-center gap-3">
                  <Server className="w-4 h-4 text-slate-400" />
                  <div>
                    <p className="text-sm font-medium text-white">
                      {instance.instance_id}
                    </p>
                    <p className="text-xs text-slate-500">{instance.hostname}</p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <StatusBadge status={instance.status} />
                  {instance.last_heartbeat && (
                    <span className="text-xs text-slate-500 flex items-center gap-1">
                      <Clock className="w-3 h-3" />
                      {formatDistanceToNow(new Date(instance.last_heartbeat), {
                        addSuffix: true,
                      })}
                    </span>
                  )}
                </div>
              </div>
            ))}
            {(!instances || instances.length === 0) && !instancesLoading && (
              <div className="text-slate-400 text-sm text-center py-8">
                No instances registered yet
              </div>
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

function StatusBadge({ status }: { status: string }) {
  const styles = {
    online: "status-badge status-online",
    offline: "status-badge status-offline",
    degraded: "status-badge status-degraded",
    unknown: "status-badge bg-slate-500/20 text-slate-400",
  };

  return (
    <span className={styles[status as keyof typeof styles] || styles.unknown}>
      {status}
    </span>
  );
}

