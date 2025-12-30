"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Server, RefreshCw, Clock, Cpu, HardDrive } from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import Link from "next/link";

export default function InstancesPage() {
  const { data: instances, isLoading, refetch } = useQuery({
    queryKey: ["instances"],
    queryFn: () => api.getInstances(),
  });

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
  };

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-white">Instances</h1>
          <p className="text-slate-400 mt-1">
            Manage MySoc and SIEMCore instances
          </p>
        </div>
        <button
          onClick={() => refetch()}
          className="btn btn-secondary"
        >
          <RefreshCw className="w-4 h-4" />
          Refresh
        </button>
      </div>

      {/* Instances Grid */}
      {isLoading ? (
        <div className="text-slate-400">Loading instances...</div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-6">
          {instances?.map((instance) => (
            <Link
              key={instance.id}
              href={`/instances/${instance.id}`}
              className="card card-hover"
            >
              <div className="flex items-start justify-between mb-4">
                <div className="flex items-center gap-3">
                  <div className="p-2 rounded-lg bg-slate-800">
                    <Server className="w-5 h-5 text-cyan-400" />
                  </div>
                  <div>
                    <h3 className="font-semibold text-white">
                      {instance.instance_id}
                    </h3>
                    <p className="text-sm text-slate-500">{instance.hostname}</p>
                  </div>
                </div>
                <StatusBadge status={instance.status} />
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between text-sm">
                  <span className="text-slate-400">Type</span>
                  <span className="text-white capitalize">
                    {instance.instance_type}
                  </span>
                </div>

                {instance.last_heartbeat && (
                  <div className="flex items-center justify-between text-sm">
                    <span className="text-slate-400 flex items-center gap-1">
                      <Clock className="w-3 h-3" />
                      Last Heartbeat
                    </span>
                    <span className="text-white">
                      {formatDistanceToNow(new Date(instance.last_heartbeat), {
                        addSuffix: true,
                      })}
                    </span>
                  </div>
                )}

                {instance.last_heartbeat_data?.system && (
                  <>
                    <div className="flex items-center justify-between text-sm">
                      <span className="text-slate-400 flex items-center gap-1">
                        <Cpu className="w-3 h-3" />
                        CPU
                      </span>
                      <span className="text-white">
                        {instance.last_heartbeat_data.system.cpu_usage.toFixed(1)}%
                      </span>
                    </div>
                    <div className="flex items-center justify-between text-sm">
                      <span className="text-slate-400 flex items-center gap-1">
                        <HardDrive className="w-3 h-3" />
                        Memory
                      </span>
                      <span className="text-white">
                        {formatBytes(instance.last_heartbeat_data.system.memory_used)} /{" "}
                        {formatBytes(instance.last_heartbeat_data.system.memory_total)}
                      </span>
                    </div>
                  </>
                )}

                {instance.last_heartbeat_data?.products && (
                  <div className="pt-3 border-t border-slate-700">
                    <p className="text-xs text-slate-400 mb-2">Products</p>
                    <div className="flex flex-wrap gap-1">
                      {instance.last_heartbeat_data.products.map((product) => (
                        <span
                          key={product.name}
                          className={`text-xs px-2 py-1 rounded ${
                            product.status === "running"
                              ? "bg-emerald-500/20 text-emerald-400"
                              : "bg-red-500/20 text-red-400"
                          }`}
                        >
                          {product.name}
                        </span>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            </Link>
          ))}

          {(!instances || instances.length === 0) && (
            <div className="col-span-full text-center py-16">
              <Server className="w-12 h-12 text-slate-600 mx-auto mb-4" />
              <h3 className="text-lg font-medium text-white mb-2">
                No instances yet
              </h3>
              <p className="text-slate-400">
                Instances will appear here once they connect to the update server.
              </p>
            </div>
          )}
        </div>
      )}
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

