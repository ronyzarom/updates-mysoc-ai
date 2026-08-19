"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, OperatorSummary } from "@/lib/api";
import {
  Key,
  Plus,
  RefreshCw,
  Building2,
  CheckCircle,
  Copy,
  Check,
  Network,
  RotateCw,
  Power,
  AlertTriangle,
  ChevronRight,
  Server,
  Boxes,
  MonitorSmartphone,
} from "lucide-react";
import { formatDistanceToNow, format } from "date-fns";
import Link from "next/link";
import { useState } from "react";
import { LoadingState, ErrorState, Modal } from "@/components/ui";
import { RequireRole } from "@/lib/auth-context";

// Operators are the entire licensing surface of the cascade model: ONE
// platform key per operator. The operator's mysoc connects here with that
// key; every siemcore and swf below it is served by the operator's own
// cascade and never gets a key from this server.
export default function OperatorsPage() {
  const queryClient = useQueryClient();
  const { data: operators, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["operators"],
    queryFn: () => api.getOperators(),
    retry: false,
  });

  const [showCreateModal, setShowCreateModal] = useState(false);
  const [issuedKey, setIssuedKey] = useState<{
    title: string;
    operatorName: string;
    licenseKey: string;
    expiresAt: string;
  } | null>(null);
  const [rotateTarget, setRotateTarget] = useState<OperatorSummary | null>(null);

  const createMutation = useMutation({
    mutationFn: (data: { id?: string; name: string; expires_at?: string }) =>
      api.createOperator(data),
    onSuccess: (created) => {
      queryClient.invalidateQueries({ queryKey: ["operators"] });
      setShowCreateModal(false);
      setIssuedKey({
        title: "Operator platform key issued",
        operatorName: created.operator.name,
        licenseKey: created.license_key,
        expiresAt: created.expires_at,
      });
    },
  });

  const rotateMutation = useMutation({
    mutationFn: (id: string) => api.rotateOperatorKey(id),
    onSuccess: (rotated, id) => {
      queryClient.invalidateQueries({ queryKey: ["operators"] });
      const op = operators?.find((o) => o.id === id);
      setRotateTarget(null);
      setIssuedKey({
        title: "Platform key rotated",
        operatorName: op?.name || rotated.operator_id,
        licenseKey: rotated.license_key,
        expiresAt: rotated.expires_at,
      });
    },
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) =>
      api.updateOperator(id, { is_active: active }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["operators"] }),
  });

  const activeCount = operators?.filter((o) => o.is_active).length ?? 0;
  const totalNodes = operators?.reduce((sum, o) => sum + (o.total_nodes || 0), 0) ?? 0;

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-white">Operators</h1>
          <p className="text-slate-400 mt-1">
            One license per operator. Everything beneath it — siemcore servers
            and swf agents — updates through the operator&apos;s own cascade.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button onClick={() => refetch()} className="btn btn-secondary">
            <RefreshCw className="w-4 h-4" />
            Refresh
          </button>
          <RequireRole roles={["admin"]}>
            <button
              onClick={() => setShowCreateModal(true)}
              className="btn btn-primary"
            >
              <Plus className="w-4 h-4" />
              New Operator
            </button>
          </RequireRole>
        </div>
      </div>

      {/* How the hierarchy licenses */}
      <div className="card bg-gradient-to-r from-slate-900 to-slate-900/40 border-violet-500/20">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm">
          <span className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-violet-500/15 text-violet-300 border border-violet-500/30">
            <Key className="w-4 h-4" /> platform key
          </span>
          <ChevronRight className="w-4 h-4 text-slate-600" />
          <span className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-slate-800 text-slate-200 border border-slate-700">
            <Server className="w-4 h-4 text-violet-400" /> mysoc platform
          </span>
          <ChevronRight className="w-4 h-4 text-slate-600" />
          <span className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-slate-800 text-slate-200 border border-slate-700">
            <Boxes className="w-4 h-4 text-cyan-400" /> siemcore servers
          </span>
          <ChevronRight className="w-4 h-4 text-slate-600" />
          <span className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-slate-800 text-slate-200 border border-slate-700">
            <MonitorSmartphone className="w-4 h-4 text-emerald-400" /> swf agents
          </span>
          <span className="text-slate-500 ml-2">
            The key is issued once, to mysoc. The rest of the fleet enrolls
            with its parent updater — no per-customer keys on this server.
          </span>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <div className="card">
          <div className="flex items-center gap-3">
            <div className="p-3 rounded-lg bg-violet-500/20">
              <Building2 className="w-6 h-6 text-violet-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-white">
                {operators?.length ?? 0}
              </p>
              <p className="text-sm text-slate-400">Operators</p>
            </div>
          </div>
        </div>
        <div className="card">
          <div className="flex items-center gap-3">
            <div className="p-3 rounded-lg bg-emerald-500/20">
              <CheckCircle className="w-6 h-6 text-emerald-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-white">{activeCount}</p>
              <p className="text-sm text-slate-400">Active licenses</p>
            </div>
          </div>
        </div>
        <div className="card">
          <div className="flex items-center gap-3">
            <div className="p-3 rounded-lg bg-cyan-500/20">
              <Network className="w-6 h-6 text-cyan-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-white">{totalNodes}</p>
              <p className="text-sm text-slate-400">Fleet nodes reported</p>
            </div>
          </div>
        </div>
      </div>

      {/* Operator cards */}
      {isLoading ? (
        <LoadingState label="Loading operators..." />
      ) : isError ? (
        <ErrorState
          title="Failed to load operators"
          error={error}
          onRetry={() => refetch()}
        />
      ) : !operators || operators.length === 0 ? (
        <div className="card text-center py-16">
          <Building2 className="w-12 h-12 text-slate-600 mx-auto mb-4" />
          <h3 className="text-lg font-medium text-white mb-2">No operators yet</h3>
          <p className="text-slate-400 mb-6">
            Create an operator to issue its single platform license. Its mysoc
            connects with that key and relays updates to the whole fleet.
          </p>
          <RequireRole roles={["admin"]}>
            <button
              onClick={() => setShowCreateModal(true)}
              className="btn btn-primary"
            >
              <Plus className="w-4 h-4" />
              New Operator
            </button>
          </RequireRole>
        </div>
      ) : (
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
          {operators.map((op) => (
            <OperatorCard
              key={op.id}
              op={op}
              onRotate={() => setRotateTarget(op)}
              onToggle={() =>
                toggleMutation.mutate({ id: op.id, active: !op.is_active })
              }
            />
          ))}
        </div>
      )}

      {/* Legacy licenses entry point */}
      <div className="card border border-slate-800">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-slate-700/50">
              <Key className="w-5 h-5 text-slate-400" />
            </div>
            <div>
              <p className="text-sm font-medium text-white">
                Legacy licenses (pre-1.8.0)
              </p>
              <p className="text-xs text-slate-500">
                Per-customer keys issued before the cascade model. Kept working
                for existing agents; rotate customers onto their operator&apos;s
                cascade when convenient.
              </p>
            </div>
          </div>
          <Link href="/licenses" className="btn btn-secondary btn-sm">
            View legacy licenses
          </Link>
        </div>
      </div>

      {/* Create operator modal */}
      {showCreateModal && (
        <CreateOperatorModal
          onClose={() => setShowCreateModal(false)}
          onSubmit={(data) => createMutation.mutate(data)}
          isLoading={createMutation.isPending}
          error={createMutation.error?.message}
        />
      )}

      {/* Rotate confirmation */}
      {rotateTarget && (
        <Modal title="Rotate platform key" onClose={() => setRotateTarget(null)}>
          <div className="space-y-4">
            <div className="flex items-start gap-3 p-3 rounded bg-amber-500/10 border border-amber-500/40">
              <AlertTriangle className="w-5 h-5 text-amber-400 mt-0.5 shrink-0" />
              <p className="text-sm text-amber-200">
                Rotating issues a new platform key for{" "}
                <span className="font-semibold">{rotateTarget.name}</span> and
                deactivates the current one immediately. The operator&apos;s
                mysoc updater must be reconfigured with the new key before its
                next heartbeat, or the whole cascade goes stale.
              </p>
            </div>
            {rotateMutation.error && (
              <div className="p-3 rounded bg-red-500/20 border border-red-500/50 text-red-400 text-sm">
                {rotateMutation.error.message}
              </div>
            )}
            <div className="flex gap-3">
              <button
                onClick={() => setRotateTarget(null)}
                className="btn btn-secondary flex-1"
              >
                Cancel
              </button>
              <button
                onClick={() => rotateMutation.mutate(rotateTarget.id)}
                disabled={rotateMutation.isPending}
                className="btn btn-danger flex-1"
              >
                {rotateMutation.isPending ? "Rotating..." : "Rotate key"}
              </button>
            </div>
          </div>
        </Modal>
      )}

      {/* One-time key reveal */}
      {issuedKey && (
        <IssuedKeyModal issued={issuedKey} onClose={() => setIssuedKey(null)} />
      )}
    </div>
  );
}

const TIER_META = [
  { tier: "mysoc", label: "mysoc", icon: Server, color: "text-violet-400" },
  { tier: "siemcore", label: "siemcore", icon: Boxes, color: "text-cyan-400" },
  { tier: "swf", label: "swf", icon: MonitorSmartphone, color: "text-emerald-400" },
] as const;

function OperatorCard({
  op,
  onRotate,
  onToggle,
}: {
  op: OperatorSummary;
  onRotate: () => void;
  onToggle: () => void;
}) {
  const keyExpired =
    op.key_expires_at && new Date(op.key_expires_at).getTime() < Date.now();

  return (
    <div
      className={`card flex flex-col gap-4 ${
        op.is_active ? "" : "opacity-70 border-red-500/30"
      }`}
    >
      {/* Identity row */}
      <div className="flex items-start gap-3">
        <div className="p-2.5 rounded-lg bg-violet-500/15 shrink-0">
          <Building2 className="w-6 h-6 text-violet-400" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="text-lg font-semibold text-white truncate">{op.name}</h3>
            {op.is_active ? (
              <span className="status-badge status-online">Active</span>
            ) : (
              <span className="status-badge status-offline">Deactivated</span>
            )}
          </div>
          <p className="text-xs text-slate-500">
            {op.id}
            {op.last_heartbeat
              ? ` · last report ${formatDistanceToNow(new Date(op.last_heartbeat), { addSuffix: true })}`
              : " · never reported"}
          </p>
        </div>
        <Link
          href={`/operators/${encodeURIComponent(op.id)}`}
          className="btn btn-secondary btn-sm shrink-0"
        >
          Fleet
          <ChevronRight className="w-4 h-4" />
        </Link>
      </div>

      {/* The one license */}
      <div className="rounded-lg border border-slate-700 bg-slate-900/60 p-3">
        <div className="flex items-center gap-2 mb-1.5">
          <Key className="w-4 h-4 text-cyan-400" />
          <span className="text-xs uppercase tracking-wide text-slate-500">
            Platform license (the only key)
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
          {op.license_key ? (
            <code className="text-cyan-400 font-mono text-sm">{op.license_key}</code>
          ) : (
            <span className="text-sm text-slate-500">no key issued</span>
          )}
          {!op.key_active && op.license_key && (
            <span className="text-xs text-red-400">key deactivated</span>
          )}
          {op.key_expires_at && (
            <span className={`text-xs ${keyExpired ? "text-red-400" : "text-slate-500"}`}>
              {keyExpired ? "expired" : "expires"}{" "}
              {format(new Date(op.key_expires_at), "MMM d, yyyy")}
            </span>
          )}
        </div>
      </div>

      {/* Fleet breakdown */}
      <div className="grid grid-cols-3 gap-2">
        {TIER_META.map(({ tier, label, icon: Icon, color }) => (
          <div
            key={tier}
            className="rounded-lg bg-slate-800/60 border border-slate-700/60 px-3 py-2 text-center"
          >
            <Icon className={`w-4 h-4 mx-auto mb-1 ${color}`} />
            <p className="text-lg font-semibold text-white leading-none">
              {op.nodes_by_tier?.[tier] ?? 0}
            </p>
            <p className="text-[10px] uppercase tracking-wide text-slate-500 mt-1">
              {label}
            </p>
          </div>
        ))}
      </div>

      {/* Actions */}
      <RequireRole roles={["admin"]}>
        <div className="flex items-center gap-2 pt-1 border-t border-slate-800">
          <button
            onClick={onRotate}
            className="btn btn-secondary btn-sm"
            title="Rotate platform key (old key stops working immediately)"
          >
            <RotateCw className="w-3.5 h-3.5" />
            Rotate key
          </button>
          <button
            onClick={onToggle}
            className={`btn btn-sm ${op.is_active ? "btn-danger" : "btn-secondary"}`}
            title={
              op.is_active
                ? "Deactivate: the operator's whole cascade loses update access"
                : "Reactivate operator"
            }
          >
            <Power className="w-3.5 h-3.5" />
            {op.is_active ? "Deactivate" : "Activate"}
          </button>
        </div>
      </RequireRole>
    </div>
  );
}

function CreateOperatorModal({
  onClose,
  onSubmit,
  isLoading,
  error,
}: {
  onClose: () => void;
  onSubmit: (data: { id?: string; name: string; expires_at?: string }) => void;
  isLoading: boolean;
  error?: string;
}) {
  const [name, setName] = useState("");
  const [id, setId] = useState("");
  const [expiresAt, setExpiresAt] = useState(
    new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString().split("T")[0]
  );

  const slugPreview =
    id ||
    name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSubmit({
      id: id || undefined,
      name,
      expires_at: new Date(expiresAt).toISOString(),
    });
  };

  return (
    <Modal title="New Operator" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <p className="text-sm text-slate-400">
          This issues the operator&apos;s <strong>single</strong> platform
          license — the only key this server will ever hand out for the whole
          fleet. Configure it in the mysoc updater; siemcore and swf enroll
          with their parent updater, never here.
        </p>

        <div>
          <label htmlFor="op-name" className="block text-sm font-medium text-slate-300 mb-1">
            Operator Name
          </label>
          <input
            id="op-name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="input w-full"
            placeholder="e.g., Cyfox SOC Ltd"
            required
          />
        </div>

        <div>
          <label htmlFor="op-id" className="block text-sm font-medium text-slate-300 mb-1">
            Operator ID <span className="text-slate-500">(optional, derived from name)</span>
          </label>
          <input
            id="op-id"
            type="text"
            value={id}
            onChange={(e) => setId(e.target.value)}
            className="input w-full"
            placeholder={slugPreview || "e.g., cyfox-soc"}
          />
          {slugPreview && (
            <p className="text-xs text-slate-500 mt-1">
              Will be created as <code className="text-cyan-400">{slugPreview}</code>
            </p>
          )}
        </div>

        <div>
          <label htmlFor="op-expires" className="block text-sm font-medium text-slate-300 mb-1">
            Key Expires At
          </label>
          <input
            id="op-expires"
            type="date"
            value={expiresAt}
            onChange={(e) => setExpiresAt(e.target.value)}
            className="input w-full"
            required
          />
        </div>

        {error && (
          <div className="p-3 rounded bg-red-500/20 border border-red-500/50 text-red-400 text-sm">
            {error}
          </div>
        )}

        <div className="flex gap-3 pt-2">
          <button type="button" onClick={onClose} className="btn btn-secondary flex-1">
            Cancel
          </button>
          <button type="submit" disabled={isLoading} className="btn btn-primary flex-1">
            {isLoading ? "Issuing key..." : "Create & issue key"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function IssuedKeyModal({
  issued,
  onClose,
}: {
  issued: {
    title: string;
    operatorName: string;
    licenseKey: string;
    expiresAt: string;
  };
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false);

  const copyKey = async () => {
    await navigator.clipboard.writeText(issued.licenseKey);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Modal title={issued.title} onClose={onClose}>
      <div className="space-y-4">
        <p className="text-sm text-slate-400">
          Platform key for{" "}
          <span className="text-white font-medium">{issued.operatorName}</span>.
          Deliver it through the approved secrets channel and configure it as
          the mysoc updater&apos;s <code>license_key</code>.
        </p>

        <div className="flex items-center gap-2 p-3 rounded bg-slate-800 border border-slate-700">
          <code className="flex-1 text-cyan-400 font-mono text-sm break-all">
            {issued.licenseKey}
          </code>
          <button
            onClick={copyKey}
            className="btn btn-secondary btn-sm shrink-0"
            title="Copy key"
          >
            {copied ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4" />}
          </button>
        </div>

        <div className="flex items-start gap-3 p-3 rounded bg-amber-500/10 border border-amber-500/40">
          <AlertTriangle className="w-5 h-5 text-amber-400 mt-0.5 shrink-0" />
          <p className="text-sm text-amber-200">
            This is the only time the full key is shown. It is stored for
            validation but the dashboard will only display a masked form.
          </p>
        </div>

        <p className="text-xs text-slate-500">
          Expires {format(new Date(issued.expiresAt), "MMM d, yyyy")}
        </p>

        <button onClick={onClose} className="btn btn-primary w-full">
          Done
        </button>
      </div>
    </Modal>
  );
}
