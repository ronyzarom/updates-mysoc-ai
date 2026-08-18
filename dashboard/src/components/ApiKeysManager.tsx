"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Plus,
  Trash2,
  Copy,
  Check,
  Loader2,
  AlertTriangle,
} from "lucide-react";
import {
  api,
  ApiKey,
  ApiKeyScope,
  CreatedApiKey,
} from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { Modal, LoadingState, ErrorState, EmptyState } from "@/components/ui";

const EXPIRY_OPTIONS: { label: string; days: number }[] = [
  { label: "No expiry", days: 0 },
  { label: "30 days", days: 30 },
  { label: "90 days", days: 90 },
  { label: "1 year", days: 365 },
];

const inputClass =
  "w-full px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-200 text-sm focus:outline-none focus:ring-2 focus:ring-cyan-500/50";

function formatDate(value?: string): string {
  if (!value) return "—";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleDateString();
}

function ScopeBadge({ scope }: { scope: ApiKeyScope }) {
  const styles =
    scope === "admin"
      ? "bg-rose-500/20 text-rose-300"
      : "bg-amber-500/20 text-amber-300";
  const label = scope === "admin" ? "Full admin" : "Releases only";
  return (
    <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${styles}`}>
      {label}
    </span>
  );
}

function StatusBadge({ status }: { status: ApiKey["status"] }) {
  const styles =
    status === "active"
      ? "bg-emerald-500/20 text-emerald-300"
      : status === "expired"
        ? "bg-slate-600/40 text-slate-300"
        : "bg-red-500/20 text-red-300";
  return (
    <span className={`px-2 py-0.5 rounded-full text-xs font-medium capitalize ${styles}`}>
      {status}
    </span>
  );
}

export function ApiKeysManager() {
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";
  const queryClient = useQueryClient();

  const [showCreate, setShowCreate] = useState(false);
  const [name, setName] = useState("");
  const [scope, setScope] = useState<ApiKeyScope>("releases");
  const [expiryDays, setExpiryDays] = useState(90);

  const [created, setCreated] = useState<CreatedApiKey | null>(null);
  const [copied, setCopied] = useState(false);
  const [revokeTarget, setRevokeTarget] = useState<ApiKey | null>(null);

  const { data: keys, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["api-keys"],
    queryFn: () => api.getApiKeys(),
    retry: false,
    enabled: isAdmin,
  });

  const createMutation = useMutation({
    mutationFn: () =>
      api.createApiKey({
        name: name.trim(),
        scope,
        expires_in_days: expiryDays > 0 ? expiryDays : undefined,
      }),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      setCreated(res);
      setShowCreate(false);
      setName("");
      setScope("releases");
      setExpiryDays(90);
    },
  });

  const revokeMutation = useMutation({
    mutationFn: (id: string) => api.revokeApiKey(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      setRevokeTarget(null);
    },
  });

  const copyKey = async () => {
    if (!created) return;
    try {
      await navigator.clipboard.writeText(created.api_key);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard may be unavailable (insecure context); the value is
      // selectable in the field as a fallback.
    }
  };

  if (!isAdmin) {
    return (
      <p className="text-sm text-slate-400">
        API keys are managed by administrators. Ask an admin to issue a scoped
        key for uploads or automation.
      </p>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-slate-400">
          Issue named, scoped keys for uploads and automation. Send them in the{" "}
          <code className="text-cyan-400">X-API-Key</code> header. The full key
          is shown once at creation.
        </p>
        <button
          type="button"
          onClick={() => setShowCreate((v) => !v)}
          className="btn btn-primary shrink-0 text-sm"
        >
          <Plus className="w-4 h-4" />
          New key
        </button>
      </div>

      {showCreate && (
        <div className="rounded-lg border border-slate-700 bg-slate-800/40 p-4 space-y-3">
          <div>
            <label htmlFor="apikey-name" className="block text-xs text-slate-400 mb-1">
              Name
            </label>
            <input
              id="apikey-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. SWF release upload"
              className={inputClass}
            />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label htmlFor="apikey-scope" className="block text-xs text-slate-400 mb-1">
                Scope
              </label>
              <select
                id="apikey-scope"
                value={scope}
                onChange={(e) => setScope(e.target.value as ApiKeyScope)}
                className={inputClass}
              >
                <option value="releases">Releases only (recommended)</option>
                <option value="admin">Full admin</option>
              </select>
            </div>
            <div>
              <label htmlFor="apikey-expiry" className="block text-xs text-slate-400 mb-1">
                Expiry
              </label>
              <select
                id="apikey-expiry"
                value={expiryDays}
                onChange={(e) => setExpiryDays(Number(e.target.value))}
                className={inputClass}
              >
                {EXPIRY_OPTIONS.map((o) => (
                  <option key={o.days} value={o.days}>
                    {o.label}
                  </option>
                ))}
              </select>
            </div>
          </div>
          {createMutation.isError && (
            <p className="text-sm text-red-400">
              {createMutation.error instanceof Error
                ? createMutation.error.message
                : "Failed to create key"}
            </p>
          )}
          <div className="flex items-center gap-2">
            <button
              type="button"
              disabled={!name.trim() || createMutation.isPending}
              onClick={() => createMutation.mutate()}
              className="btn btn-primary text-sm disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {createMutation.isPending ? (
                <Loader2 className="w-4 h-4 animate-spin" />
              ) : (
                <Plus className="w-4 h-4" />
              )}
              Create key
            </button>
            <button
              type="button"
              onClick={() => setShowCreate(false)}
              className="btn btn-secondary text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {isLoading ? (
        <LoadingState label="Loading API keys..." />
      ) : isError ? (
        <ErrorState title="Could not load API keys" error={error} onRetry={refetch} />
      ) : !keys || keys.length === 0 ? (
        <EmptyState
          title="No API keys yet"
          description="Create a scoped key to let a team or CI upload releases."
        />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-slate-500 border-b border-slate-800">
                <th className="py-2 pr-3 font-medium">Name</th>
                <th className="py-2 pr-3 font-medium">Prefix</th>
                <th className="py-2 pr-3 font-medium">Scope</th>
                <th className="py-2 pr-3 font-medium">Status</th>
                <th className="py-2 pr-3 font-medium">Last used</th>
                <th className="py-2 pr-3 font-medium">Expires</th>
                <th className="py-2 font-medium sr-only">Actions</th>
              </tr>
            </thead>
            <tbody>
              {keys.map((k) => (
                <tr key={k.id} className="border-b border-slate-800/60">
                  <td className="py-2 pr-3 text-white">{k.name}</td>
                  <td className="py-2 pr-3">
                    <code className="text-slate-300">{k.key_prefix}…</code>
                  </td>
                  <td className="py-2 pr-3">
                    <ScopeBadge scope={k.scope} />
                  </td>
                  <td className="py-2 pr-3">
                    <StatusBadge status={k.status} />
                  </td>
                  <td className="py-2 pr-3 text-slate-400">{formatDate(k.last_used_at)}</td>
                  <td className="py-2 pr-3 text-slate-400">
                    {k.expires_at ? formatDate(k.expires_at) : "Never"}
                  </td>
                  <td className="py-2 text-right">
                    {k.status === "active" && (
                      <button
                        type="button"
                        onClick={() => setRevokeTarget(k)}
                        aria-label={`Revoke ${k.name}`}
                        className="p-1.5 rounded-lg text-slate-400 hover:text-red-400 hover:bg-slate-800 transition-colors"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* One-time reveal of the created key */}
      {created && (
        <Modal title="API key created" onClose={() => setCreated(null)} maxWidth="max-w-lg">
          <div className="space-y-4">
            <div className="flex items-start gap-3 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3">
              <AlertTriangle className="w-5 h-5 text-amber-400 mt-0.5 shrink-0" />
              <p className="text-sm text-amber-200">{created.warning}</p>
            </div>
            <div>
              <label htmlFor="apikey-created-value" className="block text-xs text-slate-400 mb-1">
                {created.key.name} · <ScopeInline scope={created.key.scope} />
              </label>
              <div className="flex items-center gap-2">
                <input
                  id="apikey-created-value"
                  readOnly
                  value={created.api_key}
                  onFocus={(e) => e.currentTarget.select()}
                  className={`${inputClass} font-mono`}
                />
                <button
                  type="button"
                  onClick={copyKey}
                  className="btn btn-secondary text-sm shrink-0"
                >
                  {copied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                  {copied ? "Copied" : "Copy"}
                </button>
              </div>
            </div>
            <div className="flex justify-end">
              <button
                type="button"
                onClick={() => setCreated(null)}
                className="btn btn-primary text-sm"
              >
                Done
              </button>
            </div>
          </div>
        </Modal>
      )}

      {/* Revoke confirmation */}
      {revokeTarget && (
        <Modal title="Revoke API key" onClose={() => setRevokeTarget(null)}>
          <div className="space-y-4">
            <p className="text-sm text-slate-300">
              Revoke <span className="font-medium text-white">{revokeTarget.name}</span>{" "}
              (<code className="text-slate-400">{revokeTarget.key_prefix}…</code>)? Any
              client using this key will immediately stop working. This cannot be undone.
            </p>
            {revokeMutation.isError && (
              <p className="text-sm text-red-400">
                {revokeMutation.error instanceof Error
                  ? revokeMutation.error.message
                  : "Failed to revoke key"}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setRevokeTarget(null)}
                className="btn btn-secondary text-sm"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={revokeMutation.isPending}
                onClick={() => revokeMutation.mutate(revokeTarget.id)}
                className="btn text-sm bg-red-600 hover:bg-red-500 text-white disabled:opacity-50"
              >
                {revokeMutation.isPending ? (
                  <Loader2 className="w-4 h-4 animate-spin" />
                ) : (
                  <Trash2 className="w-4 h-4" />
                )}
                Revoke
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}

function ScopeInline({ scope }: { scope: ApiKeyScope }) {
  return (
    <span className="text-slate-400">
      {scope === "admin" ? "Full admin" : "Releases only"}
    </span>
  );
}
