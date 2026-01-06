"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Package, Upload, RefreshCw, X, FileUp } from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import { useState, useRef } from "react";

interface UploadFormData {
  product: string;
  version: string;
  channel: string;
  release_notes: string;
  artifact: File | null;
}

export default function ReleasesPage() {
  const queryClient = useQueryClient();
  const fileInputRef = useRef<HTMLInputElement>(null);
  
  const { data: releases, isLoading, refetch } = useQuery({
    queryKey: ["releases"],
    queryFn: () => api.getReleases(),
  });

  const [filter, setFilter] = useState("");
  const [showUploadModal, setShowUploadModal] = useState(false);
  const [uploadForm, setUploadForm] = useState<UploadFormData>({
    product: "",
    version: "",
    channel: "stable",
    release_notes: "",
    artifact: null,
  });
  const [uploadError, setUploadError] = useState("");

  const uploadMutation = useMutation({
    mutationFn: async (data: UploadFormData) => {
      if (!data.artifact) throw new Error("No file selected");
      return api.uploadRelease({
        product: data.product,
        version: data.version,
        channel: data.channel,
        release_notes: data.release_notes || undefined,
        artifact: data.artifact,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["releases"] });
      setShowUploadModal(false);
      setUploadForm({
        product: "",
        version: "",
        channel: "stable",
        release_notes: "",
        artifact: null,
      });
      setUploadError("");
    },
    onError: (error: Error) => {
      setUploadError(error.message);
    },
  });

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setUploadForm({ ...uploadForm, artifact: file });
    }
  };

  const handleUploadSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setUploadError("");
    
    if (!uploadForm.product) {
      setUploadError("Product name is required");
      return;
    }
    if (!uploadForm.version) {
      setUploadError("Version is required");
      return;
    }
    if (!uploadForm.artifact) {
      setUploadError("Please select a file to upload");
      return;
    }
    
    uploadMutation.mutate(uploadForm);
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
          <button 
            onClick={() => setShowUploadModal(true)} 
            className="btn btn-primary"
          >
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
          className="w-full max-w-md px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500"
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
              <button 
                onClick={() => setShowUploadModal(true)} 
                className="btn btn-primary"
              >
                <Upload className="w-4 h-4" />
                Upload Release
              </button>
            </div>
          )}
        </div>
      )}

      {/* Upload Modal */}
      {showUploadModal && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-700 rounded-xl p-6 w-full max-w-lg mx-4 shadow-2xl">
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-xl font-semibold text-white">Upload Release</h2>
              <button
                onClick={() => {
                  setShowUploadModal(false);
                  setUploadError("");
                }}
                className="p-1 rounded-lg hover:bg-slate-800 text-slate-400 hover:text-white transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleUploadSubmit} className="space-y-4">
              {uploadError && (
                <div className="p-3 rounded-lg bg-red-500/20 border border-red-500/50 text-red-400 text-sm">
                  {uploadError}
                </div>
              )}

              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">
                  Product Name *
                </label>
                <input
                  type="text"
                  placeholder="e.g., siemcore, mysoc"
                  value={uploadForm.product}
                  onChange={(e) => setUploadForm({ ...uploadForm, product: e.target.value })}
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">
                  Version *
                </label>
                <input
                  type="text"
                  placeholder="e.g., 1.5.2"
                  value={uploadForm.version}
                  onChange={(e) => setUploadForm({ ...uploadForm, version: e.target.value })}
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">
                  Channel
                </label>
                <select
                  value={uploadForm.channel}
                  onChange={(e) => setUploadForm({ ...uploadForm, channel: e.target.value })}
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500"
                >
                  <option value="stable">Stable</option>
                  <option value="beta">Beta</option>
                  <option value="alpha">Alpha</option>
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">
                  Release Notes
                </label>
                <textarea
                  placeholder="What's new in this release..."
                  value={uploadForm.release_notes}
                  onChange={(e) => setUploadForm({ ...uploadForm, release_notes: e.target.value })}
                  rows={3}
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500 resize-none"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">
                  Artifact File *
                </label>
                <input
                  ref={fileInputRef}
                  type="file"
                  onChange={handleFileChange}
                  className="hidden"
                />
                <button
                  type="button"
                  onClick={() => fileInputRef.current?.click()}
                  className="w-full p-4 rounded-lg border-2 border-dashed border-slate-700 hover:border-cyan-500 transition-colors flex flex-col items-center gap-2 text-slate-400 hover:text-white"
                >
                  <FileUp className="w-8 h-8" />
                  {uploadForm.artifact ? (
                    <span className="text-cyan-400">
                      {uploadForm.artifact.name} ({formatBytes(uploadForm.artifact.size)})
                    </span>
                  ) : (
                    <span>Click to select a file</span>
                  )}
                </button>
              </div>

              <div className="flex justify-end gap-3 pt-4">
                <button
                  type="button"
                  onClick={() => {
                    setShowUploadModal(false);
                    setUploadError("");
                  }}
                  className="px-4 py-2 rounded-lg border border-slate-700 text-slate-300 hover:bg-slate-800 transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={uploadMutation.isPending}
                  className="px-4 py-2 rounded-lg bg-cyan-600 hover:bg-cyan-500 text-white font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
                >
                  {uploadMutation.isPending ? (
                    <>
                      <RefreshCw className="w-4 h-4 animate-spin" />
                      Uploading...
                    </>
                  ) : (
                    <>
                      <Upload className="w-4 h-4" />
                      Upload
                    </>
                  )}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
