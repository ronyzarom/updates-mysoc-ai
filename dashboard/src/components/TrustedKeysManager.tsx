"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, Archive, Loader2 } from "lucide-react";
import { api, TrustedKey } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { Modal, LoadingState, ErrorState, EmptyState } from "@/components/ui";

const ISSUER_SCOPES = [
  { value: "", label: "All issuers" },
  { value: "mysoc", label: "mysoc" },
  { value: "siemcore", label: "siemcore" },
  { value: "swf", label: "swf" },
  { value: "updates", label: "updates (updater and relay kits)" },
];

const inputClass =
  "w-full px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-200 text-sm focus:outline-none focus:ring-2 focus:ring-cyan-500/50";

function formatDate(value?: string): string {
  if (!value) return "—";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleDateString();
}

export function TrustedKeysManager() {
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";
  const queryClient = useQueryClient();

  const [showAdd, setShowAdd] = useState(false);
  const [publicKey, setPublicKey] = useState("");
  const [issuer, setIssuer] = useState("");
  const [label, setLabel] = useState("");
  const [retireTarget, setRetireTarget] = useState<TrustedKey | null>(null);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["trusted-keys"],
    queryFn: () => api.getTrustedKeys(),
    retry: false,
    enabled: isAdmin,
  });

  const addMutation = useMutation({
    mutationFn: () =>
      api.addTrustedKey({
        public_key: publicKey.trim(),
        issuer: issuer || undefined,
        label: label.trim() || undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["trusted-keys"] });
      setShowAdd(false);
      setPublicKey("");
      setIssuer("");
      setLabel("");
    },
  });

  const retireMutation = useMutation({
    mutationFn: (id: string) => api.retireTrustedKey(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["trusted-keys"] });
      setRetireTarget(null);
    },
  });

  if (!isAdmin) {
    return (
      <p className="text-sm text-slate-400">
        Trusted issuer keys are managed by administrators.
      </p>
    );
  }

  const keys = data?.keys ?? [];
  const maxActive = data?.max_active_keys_per_scope ?? 2;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-slate-400">
          Public keys that uploads may be sealed with. Uploads are checked and
          badged, never rejected. Up to {maxActive} active keys per issuer scope
          so the next key can be added before the current one is retired.
        </p>
        <button
          type="button"
          onClick={() => setShowAdd((v) => !v)}
          className="btn btn-primary shrink-0 text-sm"
        >
          <Plus className="w-4 h-4" />
          Add key
        </button>
      </div>

      {showAdd && (
        <div className="rounded-lg border border-slate-700 bg-slate-800/40 p-4 space-y-3">
          <div>
            <label htmlFor="trusted-key-public" className="block text-xs text-slate-400 mb-1">
              Public key (hex ed25519)
            </label>
            <input
              id="trusted-key-public"
              type="text"
              value={publicKey}
              onChange={(e) => setPublicKey(e.target.value)}
              placeholder="64 hex characters"
              className={`${inputClass} font-mono`}
            />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label htmlFor="trusted-key-issuer" className="block text-xs text-slate-400 mb-1">
                Issuer scope
              </label>
              <select
                id="trusted-key-issuer"
                value={issuer}
                onChange={(e) => setIssuer(e.target.value)}
                className={inputClass}
              >
                {ISSUER_SCOPES.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor="trusted-key-label" className="block text-xs text-slate-400 mb-1">
                Label
              </label>
              <input
                id="trusted-key-label"
                type="text"
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                placeholder="e.g. 2027 release key"
                className={inputClass}
              />
            </div>
          </div>
          {addMutation.isError && (
            <p className="text-sm text-red-400">
              {addMutation.error instanceof Error ? addMutation.error.message : "Failed to add key"}
            </p>
          )}
          <div className="flex items-center gap-2">
            <button
              type="button"
              disabled={!publicKey.trim() || addMutation.isPending}
              onClick={() => addMutation.mutate()}
              className="btn btn-primary text-sm disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {addMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : <Plus className="w-4 h-4" />}
              Add key
            </button>
            <button type="button" onClick={() => setShowAdd(false)} className="btn btn-secondary text-sm">
              Cancel
            </button>
          </div>
        </div>
      )}

      {isLoading ? (
        <LoadingState label="Loading trusted keys..." />
      ) : isError ? (
        <ErrorState title="Could not load trusted keys" error={error} onRetry={refetch} />
      ) : keys.length === 0 ? (
        <EmptyState
          title="No trusted keys"
          description="The server registers its release key on startup when signing is enabled."
        />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-slate-500 border-b border-slate-800">
                <th className="py-2 pr-3 font-medium">Key id</th>
                <th className="py-2 pr-3 font-medium">Label</th>
                <th className="py-2 pr-3 font-medium">Issuer scope</th>
                <th className="py-2 pr-3 font-medium">Status</th>
                <th className="py-2 pr-3 font-medium">Added</th>
                <th className="py-2 font-medium sr-only">Actions</th>
              </tr>
            </thead>
            <tbody>
              {keys.map((k) => (
                <tr key={k.id} className="border-b border-slate-800/60">
                  <td className="py-2 pr-3">
                    <code className="text-slate-300" title={k.public_key}>{k.key_id}</code>
                  </td>
                  <td className="py-2 pr-3 text-white">{k.label || "—"}</td>
                  <td className="py-2 pr-3 text-slate-300">{k.issuer || "All issuers"}</td>
                  <td className="py-2 pr-3">
                    <span
                      className={`px-2 py-0.5 rounded-full text-xs font-medium capitalize ${
                        k.status === "active" ? "bg-emerald-500/20 text-emerald-300" : "bg-slate-600/40 text-slate-300"
                      }`}
                    >
                      {k.status}
                    </span>
                  </td>
                  <td className="py-2 pr-3 text-slate-400">
                    {formatDate(k.created_at)}
                    {k.created_by ? ` · ${k.created_by}` : ""}
                  </td>
                  <td className="py-2 text-right">
                    {k.status === "active" && (
                      <button
                        type="button"
                        onClick={() => setRetireTarget(k)}
                        aria-label={`Retire key ${k.key_id}`}
                        className="p-1.5 rounded-lg text-slate-400 hover:text-red-400 hover:bg-slate-800 transition-colors"
                      >
                        <Archive className="w-4 h-4" />
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {retireTarget && (
        <Modal title="Retire trusted key" onClose={() => setRetireTarget(null)}>
          <div className="space-y-4">
            <p className="text-sm text-slate-300">
              Retire key <code className="text-white">{retireTarget.key_id}</code>? New uploads
              sealed with it will show <span className="text-amber-400">Unknown key</span>. They
              are still accepted and server-signed, and existing releases are unchanged.
            </p>
            {retireMutation.isError && (
              <p className="text-sm text-red-400">
                {retireMutation.error instanceof Error ? retireMutation.error.message : "Failed to retire key"}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => setRetireTarget(null)} className="btn btn-secondary text-sm">
                Cancel
              </button>
              <button
                type="button"
                disabled={retireMutation.isPending}
                onClick={() => retireMutation.mutate(retireTarget.id)}
                className="btn text-sm bg-red-600 hover:bg-red-500 text-white disabled:opacity-50"
              >
                {retireMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : <Archive className="w-4 h-4" />}
                Retire
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
