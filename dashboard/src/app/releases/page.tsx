"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type Release, type SealStatus } from "@/lib/api";
import { Package, Upload, RefreshCw, X, FileUp, Trash2, Pencil, AlertTriangle, Search, ShieldCheck, ShieldAlert } from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import { useState, useRef, useEffect } from "react";
import { LoadingState, ErrorState, EmptyState } from "@/components/ui";
import { RequireRole } from "@/lib/auth-context";
import { SealBadge, SEAL_LABELS } from "@/components/SealBadge";
import {
  sealStatusOf,
  isNormalRelease,
  ringHeads,
  ringsHeadedBy,
  compareVersionsDesc,
  prefixThrough,
  promoteThroughOf,
  RINGS,
  type Ring,
} from "@/lib/derive";

type Delivery = "release" | "bootstrap" | "update";

interface UploadFormData {
  product: string;
  version: string;
  release_notes: string;
  promote_through: Ring;
  artifact: File | null;
  delivery: Delivery;
  variant_manifest?: string;
}

// Canonical cascade products. Every release flows updates server -> mysoc
// relay -> siemcore relay -> swf; picking the right product places it at the
// right level of the cascade.
const CASCADE_PRODUCTS = ["mysoc", "siemcore", "swf"] as const;
const CUSTOM_PRODUCT = "__custom__";

const RING_PILL: Record<string, string> = {
  alpha: "bg-purple-500/20 text-purple-400",
  beta: "bg-amber-500/20 text-amber-400",
  stable: "bg-blue-500/20 text-blue-400",
  production: "bg-emerald-500/20 text-emerald-400",
};

const VERSION_RE = /^\d+\.\d+\.\d+\.\d+$/;

// The rings alpha..ring (inclusive). Promotion is always a cumulative prefix,
// so a higher ring never runs a release a lower ring was never offered.
// A human label for a row's artifact shape.
function kindLabel(release: Release): string {
  if (release.manifest?.artifact_variants?.length) return "bootstrap+update pair";
  if (release.manifest?.artifact_kind) return release.manifest.artifact_kind;
  return "release";
}

export default function ReleasesPage() {
  const queryClient = useQueryClient();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const { data: releases, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["releases"],
    queryFn: () => api.getReleases(),
    retry: false,
  });

  const [filter, setFilter] = useState("");
  const [sealFilter, setSealFilter] = useState<SealStatus | "all">("all");
  const [showUploadModal, setShowUploadModal] = useState(false);
  useEffect(() => {
    if (!showUploadModal) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => { document.body.style.overflow = previous; };
  }, [showUploadModal]);
  const emptyUpload: UploadFormData = {
    product: "",
    version: "",
    release_notes: "",
    promote_through: "alpha",
    artifact: null,
    delivery: "release",
  };
  const [uploadForm, setUploadForm] = useState<UploadFormData>(emptyUpload);
  const [productChoice, setProductChoice] = useState<string>("");
  const [uploadError, setUploadError] = useState("");

  const uploadMutation = useMutation({
    mutationFn: async (data: UploadFormData) => {
      if (!data.artifact) throw new Error("No file selected");
      const independent = data.delivery !== "release";
      if (independent) {
        if (!data.variant_manifest) throw new Error("Artifact metadata from the product build is required");
        const metadata = JSON.parse(data.variant_manifest);
        if (metadata.kind !== data.delivery) {
          throw new Error("Manifest kind must match the selected artifact type");
        }
      }
      return api.uploadRelease({
        product: data.product,
        version: data.version,
        channel: independent ? `dual-alpha-${data.product}` : undefined,
        release_notes: data.release_notes || undefined,
        target_groups: independent ? ["alpha"] : prefixThrough(data.promote_through),
        artifact: data.artifact,
        artifact_kind: independent ? (data.delivery as "bootstrap" | "update") : undefined,
        artifact_metadata: independent ? data.variant_manifest : undefined,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["releases"] });
      setShowUploadModal(false);
      setUploadForm(emptyUpload);
      setProductChoice("");
      setUploadError("");
    },
    onError: (error: Error) => {
      setUploadError(error.message);
    },
  });

  // Delete confirmation modal state
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<
    { product: string; version: string; kind?: string; label: string; headRings: Ring[] } | null
  >(null);
  const [deleteError, setDeleteError] = useState("");

  const deleteMutation = useMutation({
    mutationFn: async ({ product, version, kind }: { product: string; version: string; kind?: string }) => {
      return api.deleteRelease(product, version, kind);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["releases"] });
      setShowDeleteModal(false);
      setDeleteTarget(null);
      setDeleteError("");
    },
    onError: (error: Error) => {
      setDeleteError(error.message);
    },
  });

  const handleDeleteRelease = (release: Release, productReleases: Release[]) => {
    setDeleteError("");
    setDeleteTarget({
      product: release.product_name,
      version: release.version,
      kind: release.manifest?.artifact_kind,
      label: kindLabel(release),
      headRings: ringsHeadedBy(release, productReleases),
    });
    setShowDeleteModal(true);
  };

  const confirmDelete = () => {
    if (deleteTarget) deleteMutation.mutate(deleteTarget);
  };

  // Edit release state and mutation
  const [showEditModal, setShowEditModal] = useState(false);
  const [editForm, setEditForm] = useState({
    product: "",
    version: "",
    release_notes: "",
    original_notes: "",
    promote_through: "alpha" as Ring,
    original_through: "alpha" as Ring,
    artifact_kind: "",
    label: "release",
  });
  const [editError, setEditError] = useState("");
  const [overridePrompt, setOverridePrompt] = useState("");
  const [overrideReason, setOverrideReason] = useState("");

  const editMutation = useMutation({
    mutationFn: async (data: { override?: string }) => {
      const kind = editForm.artifact_kind || undefined;
      if (editForm.release_notes !== editForm.original_notes) {
        await api.updateRelease(editForm.product, editForm.version, {
          release_notes: editForm.release_notes,
          artifact_kind: kind,
        });
      }
      if (editForm.promote_through !== editForm.original_through) {
        await api.updateReleaseTargetGroups(
          editForm.product,
          editForm.version,
          prefixThrough(editForm.promote_through),
          { kind, override: data.override }
        );
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["releases"] });
      setShowEditModal(false);
      setEditError("");
      setOverridePrompt("");
      setOverrideReason("");
    },
    onError: (error: Error) => {
      if (error instanceof ApiError && error.status === 409) {
        setOverridePrompt(error.message);
      } else {
        setEditError(error.message);
      }
    },
  });

  const handleEditRelease = (release: Release) => {
    const through = promoteThroughOf(release.target_groups);
    setEditForm({
      product: release.product_name,
      version: release.version,
      release_notes: release.release_notes || "",
      original_notes: release.release_notes || "",
      promote_through: through,
      original_through: through,
      artifact_kind: release.manifest?.artifact_kind || "",
      label: kindLabel(release),
    });
    setEditError("");
    setOverridePrompt("");
    setOverrideReason("");
    setShowEditModal(true);
  };

  const handleEditSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setEditError("");
    editMutation.mutate({ override: undefined });
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
    if (file) setUploadForm({ ...uploadForm, artifact: file });
  };

  const handleUploadSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setUploadError("");
    if (!uploadForm.product) {
      setUploadError("Product name is required");
      return;
    }
    if (!VERSION_RE.test(uploadForm.version.trim())) {
      setUploadError("Version must be four numeric parts, e.g. 1.5.2.1");
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
      (release.product_name.toLowerCase().includes(filter.toLowerCase()) ||
        release.version.toLowerCase().includes(filter.toLowerCase())) &&
      (sealFilter === "all" || sealStatusOf(release) === sealFilter)
  );

  // Group by product, products alphabetical.
  const productGroups = filteredReleases?.reduce((acc, release) => {
    (acc[release.product_name] ||= []).push(release);
    return acc;
  }, {} as Record<string, Release[]>);
  const productNames = Object.keys(productGroups || {}).sort();

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-white">Releases</h1>
          <p className="text-slate-400 mt-1">
            Dev uploads land here and cascade out: mysoc relays pull for their
            fleet, siemcore relays serve swf. Rings are the only update selector.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button onClick={() => refetch()} className="btn btn-secondary">
            <RefreshCw className="w-4 h-4" />
            Refresh
          </button>
          <RequireRole roles={["admin"]}>
            <button onClick={() => setShowUploadModal(true)} className="btn btn-primary">
              <Upload className="w-4 h-4" />
              Upload Release
            </button>
          </RequireRole>
        </div>
      </div>

      {/* Filter */}
      <div className="flex flex-wrap gap-3">
        <input
          type="text"
          placeholder="Search releases..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="w-full max-w-md px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500"
        />
        <select
          aria-label="Filter by issuer seal"
          value={sealFilter}
          onChange={(e) => setSealFilter(e.target.value as SealStatus | "all")}
          className="px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500"
        >
          <option value="all">All seal states</option>
          {(Object.keys(SEAL_LABELS) as SealStatus[]).map((status) => (
            <option key={status} value={status}>
              {SEAL_LABELS[status]}
            </option>
          ))}
        </select>
      </div>

      {/* Releases */}
      {isLoading ? (
        <LoadingState label="Loading releases..." />
      ) : isError ? (
        <ErrorState title="Failed to load releases" error={error} onRetry={() => refetch()} />
      ) : (
        <div className="space-y-8">
          {productNames.map((product) => {
            const productReleases = productGroups![product]!;
            const heads = ringHeads(productReleases);
            const normal = productReleases
              .filter(isNormalRelease)
              .sort((a, b) => compareVersionsDesc(a.version, b.version));
            const qualification = productReleases
              .filter((r) => !isNormalRelease(r))
              .sort((a, b) => compareVersionsDesc(a.version, b.version));
            return (
              <div key={product} className="card">
                <div className="flex items-center gap-3 mb-4">
                  <div className="p-2 rounded-lg bg-violet-500/20">
                    <Package className="w-5 h-5 text-violet-400" />
                  </div>
                  <h2 className="text-xl font-semibold text-white">{product}</h2>
                  <span className="text-sm text-slate-400">({productReleases.length} releases)</span>
                </div>

                {/* Ring-head strip: what each ring currently gets. */}
                <div className="flex flex-wrap gap-2 mb-6">
                  {RINGS.map((ring) => {
                    const head = heads[ring];
                    return (
                      <span
                        key={ring}
                        className={`px-2.5 py-1 rounded text-xs font-medium ${
                          head ? RING_PILL[ring] : "bg-red-500/20 text-red-400 border border-red-500/40"
                        }`}
                        title={
                          head
                            ? `${ring} currently offers ${head.version}`
                            : ring === "stable"
                            ? "No release on stable — fresh installs (which enrol into stable) would hang"
                            : `No release on ${ring}`
                        }
                      >
                        {ring} → {head ? head.version : "none"}
                      </span>
                    );
                  })}
                </div>

                <div className="space-y-6">
                  <ReleaseTable
                    title="Releases"
                    subtitle="Normal releases served to rings. The newest on a ring is that ring's head."
                    rows={normal}
                    heads={heads}
                    productReleases={productReleases}
                    formatBytes={formatBytes}
                    onEdit={handleEditRelease}
                    onDelete={handleDeleteRelease}
                    deleteDisabled={deleteMutation.isPending}
                  />
                  {qualification.length > 0 && (
                    <ReleaseTable
                      title="Isolated qualification (alpha-only)"
                      subtitle="Bootstrap/update qualification artifacts. Never offered on the normal update path."
                      rows={qualification}
                      heads={heads}
                      productReleases={productReleases}
                      formatBytes={formatBytes}
                      onEdit={handleEditRelease}
                      onDelete={handleDeleteRelease}
                      deleteDisabled={deleteMutation.isPending}
                    />
                  )}
                </div>
              </div>
            );
          })}

          {(!releases || releases.length === 0) && (
            <div className="card">
              <EmptyState
                icon={<Package className="w-12 h-12" />}
                title="No releases yet"
                description="Upload your first release to get started."
                action={
                  <RequireRole roles={["admin"]}>
                    <button onClick={() => setShowUploadModal(true)} className="btn btn-primary">
                      <Upload className="w-4 h-4" />
                      Upload Release
                    </button>
                  </RequireRole>
                }
              />
            </div>
          )}

          {releases && releases.length > 0 && (!filteredReleases || filteredReleases.length === 0) && (
            <div className="card">
              <EmptyState
                icon={<Search className="w-12 h-12" />}
                title="No matching releases"
                description={
                  sealFilter === "all"
                    ? `No releases match "${filter}". Try a different search.`
                    : `No ${SEAL_LABELS[sealFilter].toLowerCase()} releases match the current search.`
                }
                action={
                  <button
                    onClick={() => {
                      setFilter("");
                      setSealFilter("all");
                    }}
                    className="btn btn-secondary"
                  >
                    Clear filters
                  </button>
                }
              />
            </div>
          )}
        </div>
      )}

      {/* Upload Modal */}
      {showUploadModal && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4 z-50">
          <div role="dialog" aria-modal="true" aria-labelledby="upload-release-title" className="bg-slate-900 border border-slate-700 rounded-xl w-full max-w-2xl max-h-[calc(100dvh-2rem)] flex flex-col overflow-hidden shadow-2xl">
            <div className="flex shrink-0 items-center justify-between px-6 py-4 border-b border-slate-700">
              <h2 id="upload-release-title" className="text-xl font-semibold text-white">Upload Release</h2>
              <button
                aria-label="Close upload dialog"
                onClick={() => {
                  setShowUploadModal(false);
                  setUploadError("");
                }}
                className="p-1 rounded-lg hover:bg-slate-800 text-slate-400 hover:text-white transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleUploadSubmit} className="flex flex-col min-h-0">
              <div className="min-h-0 overflow-y-auto overscroll-contain px-6 py-4 space-y-4 [color-scheme:dark] [scrollbar-width:thin]">
                {uploadError && (
                  <div className="p-3 rounded-lg bg-red-500/20 border border-red-500/50 text-red-400 text-sm">
                    {uploadError}
                  </div>
                )}

                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">Delivery</label>
                  <select
                    aria-label="Delivery"
                    value={uploadForm.delivery}
                    onChange={(e) =>
                      setUploadForm({ ...uploadForm, delivery: e.target.value as Delivery, promote_through: "alpha" })
                    }
                    className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white"
                  >
                    <option value="release">Release (rings)</option>
                    <option value="bootstrap">Qualification: bootstrap (alpha-only)</option>
                    <option value="update">Qualification: update (alpha-only)</option>
                  </select>
                  <p className="text-xs text-slate-500 mt-1">
                    {uploadForm.delivery === "release"
                      ? "A normal release, selected by ring."
                      : "An isolated qualification artifact: alpha-only and never offered on the normal update path."}
                  </p>
                </div>

                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">Product *</label>
                  <select
                    value={productChoice}
                    onChange={(e) => {
                      const choice = e.target.value;
                      setProductChoice(choice);
                      setUploadForm({ ...uploadForm, product: choice === CUSTOM_PRODUCT ? "" : choice });
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
                    mysoc updates the platform relays; siemcore and swf cascade down through them.
                  </p>
                </div>

                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">Version *</label>
                  <input
                    type="text"
                    placeholder="e.g., 1.5.2.1"
                    value={uploadForm.version}
                    onChange={(e) => setUploadForm({ ...uploadForm, version: e.target.value })}
                    className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500"
                  />
                  <p className="text-xs text-slate-500 mt-1">Four numeric parts: MAJOR.MINOR.PATCH.BUILD.</p>
                </div>

                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">Rings</label>
                  {uploadForm.delivery === "release" ? (
                    <>
                      <select
                        aria-label="Promote through ring"
                        value={uploadForm.promote_through}
                        onChange={(e) => setUploadForm({ ...uploadForm, promote_through: e.target.value as Ring })}
                        className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white"
                      >
                        <option value="alpha">alpha</option>
                        <option value="beta">alpha → beta</option>
                        <option value="stable">alpha → beta → stable</option>
                        <option value="production">alpha → beta → stable → production</option>
                      </select>
                      <p className="text-xs text-slate-500 mt-1">
                        Cumulative from alpha. Offered to: {prefixThrough(uploadForm.promote_through).join(", ")}.
                      </p>
                    </>
                  ) : (
                    <p className="text-sm text-slate-400">Qualification artifacts are alpha-only.</p>
                  )}
                </div>

                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">Release Notes</label>
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
                    {uploadForm.delivery === "release" ? "Artifact file *" : `${uploadForm.delivery} artifact *`}
                  </label>
                  <input ref={fileInputRef} type="file" onChange={handleFileChange} className="hidden" />
                  <button
                    type="button"
                    onClick={() => fileInputRef.current?.click()}
                    className="w-full p-4 rounded-lg border-2 border-dashed border-slate-700 hover:border-cyan-500 transition-colors flex flex-col items-center gap-2 text-slate-400 hover:text-white"
                  >
                    <FileUp className="w-8 h-8" />
                    {uploadForm.artifact ? (
                      <span className="text-cyan-400 break-all">
                        {uploadForm.artifact.name} ({formatBytes(uploadForm.artifact.size)})
                      </span>
                    ) : (
                      <span>Click to select a file</span>
                    )}
                  </button>
                </div>

                {uploadForm.delivery !== "release" && (
                  <label className="block text-sm text-slate-300">
                    Artifact metadata (JSON from product build)
                    <textarea
                      required
                      value={uploadForm.variant_manifest || ""}
                      onChange={(e) => setUploadForm({ ...uploadForm, variant_manifest: e.target.value })}
                      className="block w-full mt-2 p-2 rounded bg-slate-800 font-mono text-xs"
                      rows={5}
                    />
                  </label>
                )}
              </div>
              <div className="flex shrink-0 justify-end gap-3 px-6 py-4 border-t border-slate-700 bg-slate-900">
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
                Edit {editForm.product} {editForm.version}
                <span className="ml-2 text-xs font-normal px-2 py-0.5 rounded bg-slate-800 text-slate-400">{editForm.label}</span>
              </h2>
              <button
                onClick={() => {
                  setShowEditModal(false);
                  setEditError("");
                  setOverridePrompt("");
                }}
                className="p-1 rounded-lg hover:bg-slate-800 text-slate-400 hover:text-white transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleEditSubmit} className="space-y-4">
              {editError && (
                <div className="p-3 rounded-lg bg-red-500/20 border border-red-500/50 text-red-400 text-sm break-words">
                  {editError}
                </div>
              )}
              {overridePrompt && (
                <div className="p-3 rounded-lg bg-amber-500/15 border border-amber-500/50 text-amber-300 text-sm space-y-2">
                  <p className="break-words">{overridePrompt}</p>
                  <input
                    type="text"
                    placeholder="Override reason (audited)"
                    value={overrideReason}
                    onChange={(e) => setOverrideReason(e.target.value)}
                    className="w-full px-3 py-1.5 rounded bg-slate-800 border border-slate-700 text-white text-sm"
                  />
                  <button
                    type="button"
                    disabled={!overrideReason.trim() || editMutation.isPending}
                    onClick={() => editMutation.mutate({ override: overrideReason.trim() })}
                    className="px-3 py-1.5 rounded bg-amber-600 hover:bg-amber-500 text-white text-sm font-medium disabled:opacity-50"
                  >
                    Promote anyway (audited)
                  </button>
                </div>
              )}

              {editForm.artifact_kind ? (
                <p className="text-sm text-slate-400">Qualification artifacts stay alpha-only; only notes are editable here.</p>
              ) : (
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-2">Rings</label>
                  <select
                    aria-label="Promote through ring"
                    value={editForm.promote_through}
                    onChange={(e) => setEditForm({ ...editForm, promote_through: e.target.value as Ring })}
                    className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white"
                  >
                    <option value="alpha">alpha</option>
                    <option value="beta">alpha → beta</option>
                    <option value="stable">alpha → beta → stable</option>
                    <option value="production">alpha → beta → stable → production</option>
                  </select>
                  <p className="text-xs text-slate-500 mt-1">
                    Cumulative from alpha. Offered to: {prefixThrough(editForm.promote_through).join(", ")}.
                  </p>
                </div>
              )}

              <div>
                <label className="block text-sm font-medium text-slate-300 mb-2">Release Notes</label>
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
                    setOverridePrompt("");
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

              <h2 className="text-xl font-semibold text-white mb-2">Delete Release</h2>

              <p className="text-slate-400 mb-1">Are you sure you want to delete this release?</p>

              <div className="bg-slate-800 rounded-lg px-4 py-2 my-3">
                <span className="text-cyan-400 font-medium">{deleteTarget.product}</span>
                <span className="text-slate-500 mx-2">•</span>
                <span className="text-white font-mono">{deleteTarget.version}</span>
                <span className="ml-2 text-xs px-2 py-0.5 rounded bg-slate-700 text-slate-300">{deleteTarget.label}</span>
              </div>

              {deleteTarget.headRings.length > 0 && (
                <div className="w-full p-3 mb-3 rounded-lg bg-amber-500/15 border border-amber-500/50 text-amber-300 text-sm">
                  This release is currently offered to {deleteTarget.headRings.join(", ")}. Deleting it leaves{" "}
                  {deleteTarget.headRings.length > 1 ? "those rings" : "that ring"} on the next-highest version, or on nothing.
                </div>
              )}

              <p className="text-sm text-slate-500 mb-4">
                This removes the record and its files. It cannot be undone.
              </p>

              {deleteError && (
                <div className="w-full p-3 mb-4 rounded-lg bg-red-500/20 border border-red-500/50 text-red-400 text-sm break-words">
                  {deleteError}
                </div>
              )}

              <div className="flex gap-3 w-full">
                <button
                  onClick={() => {
                    setShowDeleteModal(false);
                    setDeleteTarget(null);
                    setDeleteError("");
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

function ReleaseTable({
  title,
  subtitle,
  rows,
  heads,
  productReleases,
  formatBytes,
  onEdit,
  onDelete,
  deleteDisabled,
}: {
  title: string;
  subtitle: string;
  rows: Release[];
  heads: Record<Ring, Release | null>;
  productReleases: Release[];
  formatBytes: (n: number) => string;
  onEdit: (release: Release) => void;
  onDelete: (release: Release, productReleases: Release[]) => void;
  deleteDisabled: boolean;
}) {
  return (
    <section aria-label={title} className="rounded-lg border border-slate-700 overflow-hidden">
      <div className="px-4 py-3 bg-slate-900/40 border-b border-slate-700">
        <h3 className="font-semibold text-white">
          {title} <span className="text-sm font-normal text-slate-400">({rows.length})</span>
        </h3>
        <p className="text-sm text-slate-400 mt-1">{subtitle}</p>
      </div>
      {rows.length === 0 ? (
        <p className="px-4 py-6 text-sm text-slate-400">None published.</p>
      ) : (
        <div className="table-container">
          <table className="table">
            <thead>
              <tr>
                <th>Version</th>
                <th>Target Groups</th>
                <th>Integrity</th>
                <th>Issuer seal</th>
                <th>Size</th>
                <th>Released</th>
                <th>Notes</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((release) => {
                const headRings = (Object.keys(heads) as Ring[]).filter((r) => heads[r]?.id === release.id);
                const groups = release.target_groups || [];
                return (
                  <tr key={release.id}>
                    <td>
                      <code className="text-cyan-400 font-medium">{release.version}</code>
                      {headRings.length > 0 && (
                        <div className="text-xs text-emerald-400 mt-1" title="Currently offered to these rings">
                          current for: {headRings.join(", ")}
                        </div>
                      )}
                      {release.channel?.startsWith("dual-alpha-") && (
                        <div className="text-xs text-slate-400 mt-1" title="Published to the isolated qualification namespace; never offered on the normal update path.">
                          Isolated qualification
                        </div>
                      )}
                    </td>
                    <td>
                      {groups.length === 0 ? (
                        <span className="px-2 py-0.5 rounded text-xs bg-red-500/20 text-red-400 border border-red-500/40">no ring</span>
                      ) : (
                        <div className="flex flex-wrap gap-1">
                          {groups.map((group) => (
                            <span
                              key={group}
                              className={`px-2 py-0.5 rounded text-xs ${RING_PILL[group] || "bg-slate-700 text-slate-400"}`}
                            >
                              {group}
                            </span>
                          ))}
                        </div>
                      )}
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
                    <td>
                      <SealBadge release={release} />
                    </td>
                    <td className="text-slate-300">{formatBytes(release.artifact_size)}</td>
                    <td className="text-slate-400">
                      {formatDistanceToNow(new Date(release.released_at), { addSuffix: true })}
                    </td>
                    <td className="text-slate-400 max-w-xs truncate">{release.release_notes || "-"}</td>
                    <td>
                      <RequireRole roles={["admin"]} fallback={<span className="text-slate-600">—</span>}>
                        <div className="flex items-center gap-1">
                          <button
                            onClick={() => onEdit(release)}
                            className="p-1.5 rounded-lg hover:bg-blue-500/20 text-slate-400 hover:text-blue-400 transition-colors"
                            title="Edit release"
                            aria-label={`Edit ${release.product_name} ${release.version} (${kindLabel(release)})`}
                          >
                            <Pencil className="w-4 h-4" />
                          </button>
                          <button
                            onClick={() => onDelete(release, productReleases)}
                            disabled={deleteDisabled}
                            className="p-1.5 rounded-lg hover:bg-red-500/20 text-slate-400 hover:text-red-400 transition-colors disabled:opacity-50"
                            title="Delete release"
                            aria-label={`Delete ${release.product_name} ${release.version} (${kindLabel(release)})`}
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </div>
                      </RequireRole>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
