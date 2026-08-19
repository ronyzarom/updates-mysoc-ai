"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import {
  ArrowLeft,
  AlertTriangle,
  Boxes,
  Building2,
  Check,
  Copy,
  Key,
  MonitorSmartphone,
  Power,
  RefreshCw,
  RotateCw,
  Search,
  Server,
} from "lucide-react";
import { format, formatDistanceToNow } from "date-fns";
import Link from "next/link";
import { LoadingState, ErrorState, Modal } from "@/components/ui";
import { RequireRole } from "@/lib/auth-context";
import { InstanceTree } from "@/components/InstanceTree";

const TIER_FILTERS = [
  { value: "all", label: "All tiers" },
  { value: "mysoc", label: "MySoc" },
  { value: "siemcore", label: "SiemCore" },
  { value: "swf", label: "SWF" },
];

// Drill-down for one operator: its single platform license and the whole
// cascade it relays updates to.
export default function OperatorDetailPage() {
  const params = useParams();
  const operatorId = decodeURIComponent(params.id as string);
  const queryClient = useQueryClient();

  const { data: operators, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["operators"],
    queryFn: () => api.getOperators(),
    retry: false,
  });
  const op = operators?.find((o) => o.id === operatorId);

  const [tierFilter, setTierFilter] = useState("all");
  const [search, setSearch] = useState("");
  const [showRotate, setShowRotate] = useState(false);
  const [issuedKey, setIssuedKey] = useState<{
    licenseKey: string;
    expiresAt: string;
  } | null>(null);

  const rotateMutation = useMutation({
    mutationFn: () => api.rotateOperatorKey(operatorId),
    onSuccess: (rotated) => {
      queryClient.invalidateQueries({ queryKey: ["operators"] });
      setShowRotate(false);
      setIssuedKey({
        licenseKey: rotated.license_key,
        expiresAt: rotated.expires_at,
      });
    },
  });

  const toggleMutation = useMutation({
    mutationFn: (active: boolean) =>
      api.updateOperator(operatorId, { is_active: active }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["operators"] }),
  });

  if (isLoading) return <LoadingState label="Loading operator..." />;
  if (isError)
    return (
      <ErrorState
        title="Failed to load operator"
        error={error}
        onRetry={() => refetch()}
      />
    );
  if (!op)
    return (
      <div className="card text-center py-16">
        <Building2 className="w-12 h-12 text-slate-600 mx-auto mb-4" />
        <h3 className="text-lg font-medium text-white mb-2">Operator not found</h3>
        <p className="text-slate-400 mb-6">
          No operator with ID <code className="text-cyan-400">{operatorId}</code>.
        </p>
        <Link href="/operators" className="btn btn-secondary">
          <ArrowLeft className="w-4 h-4" />
          Back to Operators
        </Link>
      </div>
    );

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-center gap-4 min-w-0">
          <Link href="/operators" className="btn btn-secondary btn-sm shrink-0">
            <ArrowLeft className="w-4 h-4" />
          </Link>
          <div className="p-3 rounded-lg bg-violet-500/15 shrink-0">
            <Building2 className="w-7 h-7 text-violet-400" />
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-3">
              <h1 className="text-3xl font-bold text-white truncate">{op.name}</h1>
              {op.is_active ? (
                <span className="status-badge status-online">Active</span>
              ) : (
                <span className="status-badge status-offline">Deactivated</span>
              )}
            </div>
            <p className="text-slate-400 mt-1">
              {op.id}
              {op.last_heartbeat
                ? ` · last report ${formatDistanceToNow(new Date(op.last_heartbeat), { addSuffix: true })}`
                : " · never reported"}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3 shrink-0">
          <button onClick={() => refetch()} className="btn btn-secondary">
            <RefreshCw className="w-4 h-4" />
            Refresh
          </button>
          <RequireRole roles={["admin"]}>
            <button onClick={() => setShowRotate(true)} className="btn btn-secondary">
              <RotateCw className="w-4 h-4" />
              Rotate key
            </button>
            <button
              onClick={() => toggleMutation.mutate(!op.is_active)}
              disabled={toggleMutation.isPending}
              className={`btn ${op.is_active ? "btn-danger" : "btn-primary"}`}
            >
              <Power className="w-4 h-4" />
              {op.is_active ? "Deactivate" : "Activate"}
            </button>
          </RequireRole>
        </div>
      </div>

      {/* License + fleet stats */}
      <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
        <div className="card lg:col-span-2">
          <div className="flex items-center gap-2 mb-2">
            <Key className="w-4 h-4 text-cyan-400" />
            <span className="text-xs uppercase tracking-wide text-slate-500">
              Platform license (the only key)
            </span>
          </div>
          {op.license_key ? (
            <>
              <code className="text-cyan-400 font-mono">{op.license_key}</code>
              <p className="text-xs text-slate-500 mt-2">
                {!op.key_active && (
                  <span className="text-red-400 mr-2">key deactivated ·</span>
                )}
                {op.key_issued_at &&
                  `issued ${format(new Date(op.key_issued_at), "MMM d, yyyy")} · `}
                {op.key_expires_at
                  ? `expires ${format(new Date(op.key_expires_at), "MMM d, yyyy")} (${formatDistanceToNow(new Date(op.key_expires_at), { addSuffix: true })})`
                  : "no expiry"}
              </p>
            </>
          ) : (
            <p className="text-sm text-slate-500">No key issued.</p>
          )}
        </div>
        <TierStat
          icon={Server}
          color="text-violet-400"
          bg="bg-violet-500/15"
          label="mysoc / siemcore"
          value={`${op.nodes_by_tier?.["mysoc"] ?? 0} / ${op.nodes_by_tier?.["siemcore"] ?? 0}`}
        />
        <TierStat
          icon={MonitorSmartphone}
          color="text-emerald-400"
          bg="bg-emerald-500/15"
          label="swf agents"
          value={String(op.nodes_by_tier?.["swf"] ?? 0)}
        />
      </div>

      {/* Cascade tree */}
      <div className="space-y-4">
        <div className="flex items-center justify-between gap-3 flex-wrap">
          <div>
            <h2 className="text-xl font-semibold text-white flex items-center gap-2">
              <Boxes className="w-5 h-5 text-cyan-400" />
              Update cascade
            </h2>
            <p className="text-sm text-slate-500 mt-0.5">
              As reported by the operator&apos;s mysoc updater via heartbeat rollup.
            </p>
          </div>
          <div className="flex items-center gap-3">
            <div className="relative">
              <Search className="w-4 h-4 text-slate-500 absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" />
              <input
                type="search"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Search instance, host, customer…"
                className="input py-2 pl-9 w-64"
                aria-label="Search fleet"
              />
            </div>
            <select
              value={tierFilter}
              onChange={(e) => setTierFilter(e.target.value)}
              className="input py-2"
              aria-label="Filter by product tier"
            >
              {TIER_FILTERS.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>
          </div>
        </div>
        <InstanceTree
          tierFilter={tierFilter}
          search={search}
          operatorFilter={op.id}
          hideOperatorHeader
        />
      </div>

      {/* Rotate confirmation */}
      {showRotate && (
        <Modal title="Rotate platform key" onClose={() => setShowRotate(false)}>
          <div className="space-y-4">
            <div className="flex items-start gap-3 p-3 rounded bg-amber-500/10 border border-amber-500/40">
              <AlertTriangle className="w-5 h-5 text-amber-400 mt-0.5 shrink-0" />
              <p className="text-sm text-amber-200">
                Rotating issues a new platform key for{" "}
                <span className="font-semibold">{op.name}</span> and deactivates
                the current one immediately. The operator&apos;s mysoc updater
                must be reconfigured with the new key before its next heartbeat,
                or the whole cascade goes stale.
              </p>
            </div>
            {rotateMutation.error && (
              <div className="p-3 rounded bg-red-500/20 border border-red-500/50 text-red-400 text-sm">
                {rotateMutation.error.message}
              </div>
            )}
            <div className="flex gap-3">
              <button
                onClick={() => setShowRotate(false)}
                className="btn btn-secondary flex-1"
              >
                Cancel
              </button>
              <button
                onClick={() => rotateMutation.mutate()}
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
        <RevealKeyModal
          operatorName={op.name}
          licenseKey={issuedKey.licenseKey}
          expiresAt={issuedKey.expiresAt}
          onClose={() => setIssuedKey(null)}
        />
      )}
    </div>
  );
}

function TierStat({
  icon: Icon,
  color,
  bg,
  label,
  value,
}: {
  icon: typeof Server;
  color: string;
  bg: string;
  label: string;
  value: string;
}) {
  return (
    <div className="card">
      <div className="flex items-center gap-3">
        <div className={`p-3 rounded-lg ${bg}`}>
          <Icon className={`w-6 h-6 ${color}`} />
        </div>
        <div>
          <p className="text-2xl font-bold text-white">{value}</p>
          <p className="text-sm text-slate-400">{label}</p>
        </div>
      </div>
    </div>
  );
}

function RevealKeyModal({
  operatorName,
  licenseKey,
  expiresAt,
  onClose,
}: {
  operatorName: string;
  licenseKey: string;
  expiresAt: string;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false);

  const copyKey = async () => {
    await navigator.clipboard.writeText(licenseKey);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Modal title="Platform key rotated" onClose={onClose}>
      <div className="space-y-4">
        <p className="text-sm text-slate-400">
          New platform key for{" "}
          <span className="text-white font-medium">{operatorName}</span>.
          Deliver it through the approved secrets channel and configure it as
          the mysoc updater&apos;s <code>license_key</code>.
        </p>
        <div className="flex items-center gap-2 p-3 rounded bg-slate-800 border border-slate-700">
          <code className="flex-1 text-cyan-400 font-mono text-sm break-all">
            {licenseKey}
          </code>
          <button onClick={copyKey} className="btn btn-secondary btn-sm shrink-0" title="Copy key">
            {copied ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4" />}
          </button>
        </div>
        <div className="flex items-start gap-3 p-3 rounded bg-amber-500/10 border border-amber-500/40">
          <AlertTriangle className="w-5 h-5 text-amber-400 mt-0.5 shrink-0" />
          <p className="text-sm text-amber-200">
            This is the only time the full key is shown.
          </p>
        </div>
        <p className="text-xs text-slate-500">
          Expires {format(new Date(expiresAt), "MMM d, yyyy")}
        </p>
        <button onClick={onClose} className="btn btn-primary w-full">
          Done
        </button>
      </div>
    </Modal>
  );
}
