"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Package, Upload, RefreshCw } from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import { useState } from "react";

export default function ReleasesPage() {
  const { data: releases, isLoading, refetch } = useQuery({
    queryKey: ["releases"],
    queryFn: () => api.getReleases(),
  });

  const [filter, setFilter] = useState("");

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
  };

  const filteredReleases = releases?.filter(
    (release) =>
      release.product_name.toLowerCase().includes(filter.toLowerCase()) ||
      release.version.toLowerCase().includes(filter.toLowerCase())
  );

  // Group by product
  const productGroups = filteredReleases?.reduce((acc, release) => {
    if (!acc[release.product_name]) {
      acc[release.product_name] = [];
    }
    acc[release.product_name]!.push(release);
    return acc;
  }, {} as Record<string, typeof filteredReleases>);

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-white">Releases</h1>
          <p className="text-slate-400 mt-1">
            Manage product releases and versions
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button onClick={() => refetch()} className="btn btn-secondary">
            <RefreshCw className="w-4 h-4" />
            Refresh
          </button>
          <button className="btn btn-primary">
            <Upload className="w-4 h-4" />
            Upload Release
          </button>
        </div>
      </div>

      {/* Filter */}
      <div>
        <input
          type="text"
          placeholder="Search releases..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="w-full max-w-md px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
        />
      </div>

      {/* Releases */}
      {isLoading ? (
        <div className="text-slate-400">Loading releases...</div>
      ) : (
        <div className="space-y-8">
          {productGroups &&
            Object.entries(productGroups).map(([product, productReleases]) => (
              <div key={product} className="card">
                <div className="flex items-center gap-3 mb-6">
                  <div className="p-2 rounded-lg bg-violet-500/20">
                    <Package className="w-5 h-5 text-violet-400" />
                  </div>
                  <h2 className="text-xl font-semibold text-white">{product}</h2>
                  <span className="text-sm text-slate-400">
                    ({productReleases?.length} releases)
                  </span>
                </div>

                <div className="table-container">
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Version</th>
                        <th>Channel</th>
                        <th>Size</th>
                        <th>Checksum</th>
                        <th>Released</th>
                        <th>Notes</th>
                      </tr>
                    </thead>
                    <tbody>
                      {productReleases?.map((release) => (
                        <tr key={release.id}>
                          <td>
                            <code className="text-cyan-400 font-medium">
                              {release.version}
                            </code>
                          </td>
                          <td>
                            <span
                              className={`px-2 py-1 rounded text-xs ${
                                release.channel === "stable"
                                  ? "bg-emerald-500/20 text-emerald-400"
                                  : release.channel === "beta"
                                  ? "bg-amber-500/20 text-amber-400"
                                  : "bg-slate-700 text-slate-400"
                              }`}
                            >
                              {release.channel}
                            </span>
                          </td>
                          <td className="text-slate-300">
                            {formatBytes(release.artifact_size)}
                          </td>
                          <td>
                            <code className="text-xs text-slate-500 font-mono">
                              {release.checksum?.substring(0, 12)}...
                            </code>
                          </td>
                          <td className="text-slate-400">
                            {formatDistanceToNow(new Date(release.released_at), {
                              addSuffix: true,
                            })}
                          </td>
                          <td className="text-slate-400 max-w-xs truncate">
                            {release.release_notes || "-"}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            ))}

          {(!releases || releases.length === 0) && (
            <div className="card text-center py-16">
              <Package className="w-12 h-12 text-slate-600 mx-auto mb-4" />
              <h3 className="text-lg font-medium text-white mb-2">
                No releases yet
              </h3>
              <p className="text-slate-400 mb-6">
                Upload your first release to get started.
              </p>
              <button className="btn btn-primary">
                <Upload className="w-4 h-4" />
                Upload Release
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

