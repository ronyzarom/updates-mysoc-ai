"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Package, Upload, RefreshCw, X, FileUp, Trash2, Pencil, AlertTriangle, Search, ShieldCheck, ShieldAlert } from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import { useState, useRef } from "react";
import { LoadingState, ErrorState, EmptyState } from "@/components/ui";
import { RequireRole } from "@/lib/auth-context";

interface UploadFormData {
  product: string;
  version: string;
  channel: string;
  release_notes: string;
  target_groups: string[];
  artifact: File | null;
}

// Canonical cascade products. Every release flows updates server -> mysoc
// relay -> siemcore relay -> swf; picking the right product places it at the
// right level of the cascade.
const CASCADE_PRODUCTS = ["mysoc", "siemcore", "swf"] as const;
const CUSTOM_PRODUCT = "__custom__";

// Safe rollout default: new releases start in alpha; promote to further
// groups explicitly after validation.
const DEFAULT_TARGET_GROUPS = ["alpha"];

export default function ReleasesPage() {
  const queryClient = useQueryClient();
  const fileInputRef = useRef<HTMLInputElement>(null);
  
  const { data: releases, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["releases"],
    queryFn: () => api.getReleases(),
    retry: false,
  });

  const [filter, setFilter] = useState("");
  const [showUploadModal, setShowUploadModal] = useState(false);
  const [uploadForm, setUploadForm] = useState<UploadFormData>({
    product: "",
    version: "",
    channel: "stable",
    release_notes: "",
    target_groups: [...DEFAULT_TARGET_GROUPS],
    artifact: null,
  });
  const [productChoice, setProductChoice] = useState<string>("");
  const [uploadError, setUploadError] = useState("");

  const uploadMutation = useMutation({
    mutationFn: async (data: UploadFormData) => {
      if (!data.artifact) throw new Error("No file selected");
      return api.uploadRelease({
        product: data.product,
        version: data.version,
        channel: data.channel,
        release_notes: data.release_notes || undefined,
        target_groups: data.target_groups.length > 0 ? data.target_groups : undefined,
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
        target_groups: [...DEFAULT_TARGET_GROUPS],
        artifact: null,
      });
      setProductChoice("");
      setUploadError("");
    },
    onError: (error: Error) => {
      setUploadError(error.message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async ({ product, version }: { product: string; version: string }) => {
      return api.deleteRelease(product, version);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["releases"] });
      setShowDeleteModal(false);
      setDeleteTarget(null);
    },
  });

  // Delete confirmation modal state
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<{ product: string; version: string } | null>(null);

  const handleDeleteRelease = (product: string, version: string) => {
    setDeleteTarget({ product, version });
    setShowDeleteModal(true);
  };

  const confirmDelete = () => {
    if (deleteTarget) {
      deleteMutation.mutate(deleteTarget);
    }
  };

  // Edit release state and mutation
  const [showEditModal, setShowEditModal] = useState(false);
  const [editForm, setEditForm] = useState({
    product: "",
    version: "",
    release_notes: "",
    target_groups: [] as string[],
  });
  const [editError, setEditError] = useState("");

  const editMutation = useMutation({
    mutationFn: async (data: { product: string; version: string; release_notes: string; target_groups: string[] }) => {
      return api.updateRelease(data.product, data.version, {
        release_notes: data.release_notes,
        target_groups: data.target_groups,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["releases"] });
      setShowEditModal(false);
      setEditError("");
    },
    onError: (error: Error) => {
      setEditError(error.message);
    },
  });

  const handleEditRelease = (release: { product_name: string; version: string; release_notes?: string; target_groups?: string[] }) => {
    setEditForm({
      product: release.product_name,
      version: release.version,
      release_notes: release.release_notes || "",
      target_groups: release.target_groups || [],
    });
    setEditError("");
    setShowEditModal(true);
  };

  const handleEditSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setEditError("");
    editMutation.mutate(editForm);
  };

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
            Dev uploads land here, get signed, and cascade out: mysoc relays
            pull for their fleet, siemcore relays serve swf
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button onClick={() => refetch()} className="btn btn-secondary">
            <RefreshCw className="w-4 h-4" />
            Refresh
          </button>
          <RequireRole roles={["admin"]}>
            <button
              onClick={() => setShowUploadModal(true)}
              className="btn btn-primary"
            >
              <Upload className="w-4 h-4" />
              Upload Release
            </button>
          </RequireRole>
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
        <LoadingState label="Loading releases..." />
      ) : isError ? (
        <ErrorState
          title="Failed to load releases"
          error={error}
          onRetry={() => refetch()}
        />
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
                        <th>Target Groups</th>
                        <th>Integrity</th>
                        <th>Size</th>
                        <th>Released</th>
                        <th>Notes</th>
                        <th>Actions</th>
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
                          <td>
                            <div className="flex flex-wrap gap-1">
                              {(release.target_groups || ["all"]).map((group) => (
                                <span
                                  key={group}
                                  className={`px-2 py-0.5 rounded text-xs ${
                                    group === "alpha"
                                      ? "bg-purple-500/20 text-purple-400"
                                      : group === "beta"
                                      ? "bg-amber-500/20 text-amber-400"
                                      : group === "stable"
                                      ? "bg-blue-500/20 text-blue-400"
                                      : group === "production"
                                      ? "bg-emerald-500/20 text-emerald-400"
                                      : "bg-slate-700 text-slate-400"
                                  }`}
                                >
                                  {group}
                                </span>
                              ))}
                            </div>
                          </td>
                          <td>
                            {release.signature ? (
                              <span
                                className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs bg-emerald-500/20 text-emerald-400"
                                title="ed25519-signed at publish; every cascade hop verifies before install"
                              >
                                <ShieldCheck className="w-3.5 h-3.5" />
                                signed
                              </span>
                            ) : (
                              <span
                                className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs bg-amber-500/20 text-amber-400"
                                title="Published before signing was enabled; updaters with signing.require will reject it"
                              >
                                <ShieldAlert className="w-3.5 h-3.5" />
                                unsigned
                              </span>
                            )}
                          </td>
                          <td className="text-slate-300">
                            {formatBytes(release.artifact_size)}
                          </td>
                          <td className="text-slate-400">
                            {formatDistanceToNow(new Date(release.released_at), {
                              addSuffix: true,
                            })}
                          </td>
                          <td className="text-slate-400 max-w-xs truncate">
                            {release.release_notes || "-"}
                          </td>
                          <td>
                            <RequireRole
                              roles={["admin"]}
                              fallback={<span className="text-slate-600">—</span>}
                            >
                              <div className="flex items-center gap-1">
                                <button
                                  onClick={() => handleEditRelease(release)}
                                  className="p-1.5 rounded-lg hover:bg-blue-500/20 text-slate-400 hover:text-blue-400 transition-colors"
                                  title="Edit release"
                                  aria-label={`Edit ${release.product_name} ${release.version}`}
                                >
                                  <Pencil className="w-4 h-4" />
                                </button>
                                <button
                                  onClick={() => handleDeleteRelease(release.product_name, release.version)}
                                  disabled={deleteMutation.isPending}
                                  className="p-1.5 rounded-lg hover:bg-red-500/20 text-slate-400 hover:text-red-400 transition-colors disabled:opacity-50"
                                  title="Delete release"
                                  aria-label={`Delete ${release.product_name} ${release.version}`}
                                >
                                  <Trash2 className="w-4 h-4" />
                                </button>
                              </div>
                            </RequireRole>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            ))}

          {(!releases || releases.length === 0) && (
            <div className="card">
              <EmptyState
                icon={<Package className="w-12 h-12" />}
                title="No releases yet"
                description="Upload your first release to get started."
                action={
                  <RequireRole roles={["admin"]}>
                    <button
                      onClick={() => setShowUploadModal(true)}
                      className="btn btn-primary"
                    >
                      <Upload className="w-4 h-4" />
                      Upload Release
                    </button>
                  </RequireRole>
                }
              />
            </div>
          )}

          {releases &&
            releases.length > 0 &&
            (!filteredReleases || filteredReleases.length === 0) && (
              <div className="card">
                <EmptyState
                  icon={<Search className="w-12 h-12" />}
                  title="No matching releases"
                  description={`No releases match "${filter}". Try a different search.`}
                  action={
                    <button
                      onClick={() => setFilter("")}
                      className="btn btn-secondary"
                    >
                      Clear search
                    </button>
                  }
                />
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
                  Product *
                </label>
                <select
                  value={productChoice}
                  onChange={(e) => {
                    const choice = e.target.value;
                    setProductChoice(choice);
                    setUploadForm({
                      ...uploadForm,
                      product: choice === CUSTOM_PRODUCT ? "" : choice,
                    });
                  }}
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500"
                >
                  <option value="" disabled>
                    Select a product…
                  </option>
                  {CASCADE_PRODUCTS.map((product) => (
                    <option key={product} value={product}>
                      {product}
                    </option>
                  ))}
                  <option value={CUSTOM_PRODUCT}>Other (custom name)</option>
                </select>
                {productChoice === CUSTOM_PRODUCT && (
                  <input
                    type="text"
                    placeholder="custom product name, e.g. siemcore-collector"
                    value={uploadForm.product}
                    onChange={(e) => setUploadForm({ ...uploadForm, product: e.target.value })}
                    className="mt-2 w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500"
                  />
                )}
                <p className="text-xs text-slate-500 mt-1">
                  mysoc updates the platform relays, siemcore and swf cascade
                  down through them
                </p>
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
                  Target Groups
                </label>
                <div className="flex flex-wrap gap-2">
                  {["alpha", "beta", "stable", "production"].map((group) => (
                    <button
                      key={group}
                      type="button"
                      onClick={() => {
                        const groups = uploadForm.target_groups.includes(group)
                          ? uploadForm.target_groups.filter((g) => g !== group)
                          : [...uploadForm.target_groups, group];
                        setUploadForm({ ...uploadForm, target_groups: groups });
                      }}
                      className={`px-3 py-1.5 rounded text-sm transition-colors ${
                        uploadForm.target_groups.includes(group)
                          ? group === "alpha"
                            ? "bg-purple-500/30 text-purple-300 border border-purple-500/50"
                            : group === "beta"
                            ? "bg-amber-500/30 text-amber-300 border border-amber-500/50"
                            : group === "stable"
                            ? "bg-blue-500/30 text-blue-300 border border-blue-500/50"
                            : "bg-emerald-500/30 text-emerald-300 border border-emerald-500/50"
                          : "bg-slate-800 text-slate-400 border border-slate-700"
                      }`}
                    >
                      {group}
                    </button>
                  ))}
                </div>
                <p className="text-xs text-slate-500 mt-1">
                  Safe rollout: releases start in alpha; promote to beta,
                  stable, and production after validation
                </p>
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

      {/* Edit Modal */}
      {showEditModal && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-700 rounded-xl p-6 w-full max-w-lg mx-4 shadow-2xl">
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-xl font-semibold text-white">
                Edit Release: {editForm.product} {editForm.version}
              </h2>
              <button
                onClick={() => {
                  setShowEditModal(false);
                  setEditError("");
                }}
                className="p-1 rounded-lg hover:bg-slate-800 text-slate-400 hover:text-white transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleEditSubmit} className="space-y-4">
              {editError && (
                <div className="p-3 rounded-lg bg-red-500/20 border border-red-500/50 text-red-400 text-sm">
                  {editError}
                </div>
              )}

              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">
                  Target Groups
                </label>
                <div className="flex flex-wrap gap-2">
                  {["alpha", "beta", "stable", "production"].map((group) => (
                    <button
                      key={group}
                      type="button"
                      onClick={() => {
                        const newGroups = editForm.target_groups.includes(group)
                          ? editForm.target_groups.filter((g) => g !== group)
                          : [...editForm.target_groups, group];
                        setEditForm({ ...editForm, target_groups: newGroups });
                      }}
                      className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                        editForm.target_groups.includes(group)
                          ? group === "alpha"
                            ? "bg-purple-500/30 text-purple-300 border border-purple-500"
                            : group === "beta"
                            ? "bg-amber-500/30 text-amber-300 border border-amber-500"
                            : group === "stable"
                            ? "bg-blue-500/30 text-blue-300 border border-blue-500"
                            : "bg-emerald-500/30 text-emerald-300 border border-emerald-500"
                          : "bg-slate-800 text-slate-400 border border-slate-700 hover:border-slate-600"
                      }`}
                    >
                      {group}
                    </button>
                  ))}
                </div>
                <p className="text-xs text-slate-500 mt-1">
                  Select which instance groups will receive this release
                </p>
              </div>

              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">
                  Release Notes
                </label>
                <textarea
                  placeholder="What's new in this release..."
                  value={editForm.release_notes}
                  onChange={(e) => setEditForm({ ...editForm, release_notes: e.target.value })}
                  rows={4}
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500 resize-none"
                />
              </div>

              <div className="flex justify-end gap-3 pt-4">
                <button
                  type="button"
                  onClick={() => {
                    setShowEditModal(false);
                    setEditError("");
                  }}
                  className="px-4 py-2 rounded-lg border border-slate-700 text-slate-300 hover:bg-slate-800 transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={editMutation.isPending}
                  className="px-4 py-2 rounded-lg bg-cyan-600 hover:bg-cyan-500 text-white font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
                >
                  {editMutation.isPending ? (
                    <>
                      <RefreshCw className="w-4 h-4 animate-spin" />
                      Saving...
                    </>
                  ) : (
                    "Save Changes"
                  )}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {showDeleteModal && deleteTarget && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-700 rounded-xl p-6 w-full max-w-md mx-4 shadow-2xl">
            <div className="flex flex-col items-center text-center">
              <div className="w-14 h-14 rounded-full bg-red-500/20 flex items-center justify-center mb-4">
                <AlertTriangle className="w-7 h-7 text-red-400" />
              </div>
              
              <h2 className="text-xl font-semibold text-white mb-2">
                Delete Release
              </h2>
              
              <p className="text-slate-400 mb-1">
                Are you sure you want to delete this release?
              </p>
              
              <div className="bg-slate-800 rounded-lg px-4 py-2 my-3">
                <span className="text-cyan-400 font-medium">{deleteTarget.product}</span>
                <span className="text-slate-500 mx-2">•</span>
                <span className="text-white font-mono">{deleteTarget.version}</span>
              </div>
              
              <p className="text-sm text-slate-500 mb-6">
                This action cannot be undone. Instances will no longer be able to download this version.
              </p>

              <div className="flex gap-3 w-full">
                <button
                  onClick={() => {
                    setShowDeleteModal(false);
                    setDeleteTarget(null);
                  }}
                  className="flex-1 px-4 py-2.5 rounded-lg border border-slate-700 text-slate-300 hover:bg-slate-800 transition-colors font-medium"
                >
                  Cancel
                </button>
                <button
                  onClick={confirmDelete}
                  disabled={deleteMutation.isPending}
                  className="flex-1 px-4 py-2.5 rounded-lg bg-red-600 hover:bg-red-500 text-white font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
                >
                  {deleteMutation.isPending ? (
                    <>
                      <RefreshCw className="w-4 h-4 animate-spin" />
                      Deleting...
                    </>
                  ) : (
                    <>
                      <Trash2 className="w-4 h-4" />
                      Delete
                    </>
                  )}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
