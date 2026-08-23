"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useParams, useRouter } from "next/navigation";
import {
  Server,
  ArrowLeft,
  RefreshCw,
  Clock,
  Cpu,
  HardDrive,
  Activity,
  Shield,
  Settings,
  Power,
  Pencil,
  Check,
  X,
  Trash2,
  Globe,
  KeyRound,
  ArrowUpCircle,
  Monitor,
  Network,
} from "lucide-react";
import Link from "next/link";
import { formatDistanceToNow, format } from "date-fns";
import { useState, useEffect } from "react";
import { LoadingState, ErrorState, Switch } from "@/components/ui";
import { RequireRole } from "@/lib/auth-context";
import { effectiveUpdateGroup } from "@/lib/derive";
import { TierBadge } from "@/components/InstanceTree";
import type { Instance } from "@/lib/api";

export default function InstanceDetailPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const id = params.id as string;

  const { data: instance, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["instance", id],
    queryFn: () => api.getInstance(id),
    retry: false,
  });

  // The full fleet lets us resolve this node's parent chain and children by
  // instance_id (self-reported hierarchy: mysoc > siemcore > swf).
  const { data: allInstances } = useQuery({
    queryKey: ["instances"],
    queryFn: () => api.getInstances(),
  });

  const [selectedGroup, setSelectedGroup] = useState<string>("");
  const [isEditingName, setIsEditingName] = useState(false);
  const [displayName, setDisplayName] = useState("");
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [mutationError, setMutationError] = useState<string | null>(null);

  // Sync selectedGroup with instance data when loaded
  useEffect(() => {
    if (instance?.update_group) {
      setSelectedGroup(instance.update_group);
    }
  }, [instance?.update_group]);

  // Clear error after 5 seconds
  useEffect(() => {
    if (mutationError) {
      const timer = setTimeout(() => setMutationError(null), 5000);
      return () => clearTimeout(timer);
    }
  }, [mutationError]);

  const updateInstanceMutation = useMutation({
    mutationFn: (data: { display_name?: string }) => api.updateInstance(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["instance", id] });
      queryClient.invalidateQueries({ queryKey: ["instances"] });
      setIsEditingName(false);
      setMutationError(null);
    },
    onError: (error: Error) => {
      setMutationError(`Failed to update name: ${error.message}`);
    },
  });

  const deleteInstanceMutation = useMutation({
    mutationFn: () => api.deleteInstance(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["instances"] });
      router.push("/instances");
    },
    onError: (error: Error) => {
      setMutationError(`Failed to delete: ${error.message}`);
      setShowDeleteConfirm(false);
    },
  });

  const autoUpdateMutation = useMutation({
    mutationFn: (enabled: boolean) => api.setInstanceAutoUpdate(id, enabled),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["instance", id] });
      queryClient.invalidateQueries({ queryKey: ["instances"] });
      setMutationError(null);
    },
    onError: (error: Error) => {
      setMutationError(`Failed to update auto-update setting: ${error.message}`);
    },
  });

  const updateGroupMutation = useMutation({
    mutationFn: (group: string) => api.setInstanceUpdateGroup(id, group),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["instance", id] });
      queryClient.invalidateQueries({ queryKey: ["instances"] });
      setMutationError(null);
    },
    onError: (error: Error) => {
      setMutationError(`Failed to update group: ${error.message}`);
    },
  });

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
  };

  // Format OS/Arch
  const formatOsArch = (system?: { os?: string; arch?: string }) => {
    if (!system?.os && !system?.arch) return "Unknown";
    return `${system.os || "?"} / ${system.arch || "?"}`;
  };

  // Format uptime in human-readable form
  const formatUptime = (seconds?: number) => {
    if (!seconds) return "Unknown";
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${minutes}m`;
    return `${minutes}m`;
  };

  if (isLoading) {
    return <LoadingState label="Loading instance..." />;
  }

  if (isError) {
    return (
      <ErrorState
        title="Failed to load instance"
        error={error}
        onRetry={() => refetch()}
      />
    );
  }

  if (!instance) {
    return (
      <div className="text-center py-16">
        <Server className="w-12 h-12 text-slate-600 mx-auto mb-4" />
        <h3 className="text-lg font-medium text-white mb-2">Instance not found</h3>
        <button onClick={() => router.back()} className="btn btn-secondary mt-4">
          <ArrowLeft className="w-4 h-4" />
          Go Back
        </button>
      </div>
    );
  }

  const heartbeat = instance.last_heartbeat_data;

  // Agents serialize Go's zero time ("0001-01-01T00:00:00Z") when no license
  // expiry is set; treat anything implausibly old as "no expiry" instead of
  // rendering "January 1, 1".
  const licenseExpiry = (() => {
    const raw = heartbeat?.license?.expires_at;
    if (!raw) return null;
    const d = new Date(raw);
    return isNaN(d.getTime()) || d.getFullYear() <= 1970 ? null : d;
  })();

  // Cascade children (siemcore/swf) authenticate to their parent relay, not to
  // this server — only the tier-1 platform holds the operator license. Their
  // heartbeats carry an empty license block, which must not read as "Invalid".
  const viaRelay = Boolean(instance.reported_via || instance.parent_instance_id);

  // The updater always reports identity (OS/arch) but only collects host
  // measurements on supported platforms (updater >= 1.8.4). A real host always
  // has memory_total > 0 when measurements exist, so identity alone must NOT
  // count as telemetry — otherwise unmeasured zeros render as failing facts
  // (0% CPU, "Firewall Disabled", a process uptime posing as host uptime).
  const telemetryReported = Boolean(
    heartbeat?.system && heartbeat.system.memory_total > 0
  );

  // Security posture has its own collector (the cascade updater never runs
  // one), so it needs its own signal: a real scan always sets firewall_status
  // or a nonzero score. Gating it on system telemetry would flip the card to
  // zero-valued "facts" the moment host metrics start arriving.
  const securityReported = Boolean(
    heartbeat?.security &&
      (heartbeat.security.security_score > 0 || heartbeat.security.firewall_status)
  );

  // The group currently persisted on the instance, and the group the user has
  // chosen. Never allow submitting an empty group value.
  const currentGroup = instance.update_group || "stable";
  const effectiveGroup = effectiveUpdateGroup(selectedGroup, instance.update_group);

  // Resolve hierarchy relationships from the fleet snapshot.
  const fleet = allInstances || [];
  const byInstanceId = new Map(fleet.map((i) => [i.instance_id, i]));
  const ancestors: Instance[] = [];
  {
    let cursor = instance.parent_instance_id;
    const seen = new Set<string>([instance.instance_id]);
    while (cursor && byInstanceId.has(cursor) && !seen.has(cursor)) {
      seen.add(cursor);
      const parent = byInstanceId.get(cursor)!;
      ancestors.unshift(parent);
      cursor = parent.parent_instance_id;
    }
  }
  const children = fleet.filter((i) => i.parent_instance_id === instance.instance_id);
  const parentKnown = instance.parent_instance_id ? byInstanceId.get(instance.parent_instance_id) : undefined;
  const hasHierarchy = Boolean(instance.product_tier || instance.parent_instance_id || children.length > 0);

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <button
            onClick={() => router.back()}
            className="p-2 rounded-lg hover:bg-slate-800 transition-colors"
          >
            <ArrowLeft className="w-5 h-5 text-slate-400" />
          </button>
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-3xl font-bold text-white">{instance.instance_id}</h1>
              <StatusBadge status={instance.status} />
            </div>
            {isEditingName ? (
              <div className="flex items-center gap-2 mt-1">
                <input
                  type="text"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  placeholder="e.g., cloud.siemcore.ai"
                  className="px-2 py-1 text-sm rounded bg-slate-800 border border-slate-600 text-white focus:outline-none focus:ring-1 focus:ring-cyan-500"
                  autoFocus
                />
                <button
                  onClick={() => updateInstanceMutation.mutate({ display_name: displayName })}
                  disabled={updateInstanceMutation.isPending}
                  className="p-1 rounded hover:bg-emerald-500/20 text-emerald-400"
                >
                  <Check className="w-4 h-4" />
                </button>
                <button
                  onClick={() => {
                    setIsEditingName(false);
                    setDisplayName(instance.display_name || "");
                  }}
                  className="p-1 rounded hover:bg-red-500/20 text-red-400"
                >
                  <X className="w-4 h-4" />
                </button>
              </div>
            ) : (
              <div className="flex items-center gap-2 mt-1">
                <p className="text-slate-400">
                  {instance.display_name || instance.hostname || "No display name set"}
                </p>
                <RequireRole roles={["admin"]}>
                  <button
                    onClick={() => {
                      setDisplayName(instance.display_name || "");
                      setIsEditingName(true);
                    }}
                    className="p-1 rounded hover:bg-slate-700 text-slate-500 hover:text-white"
                    title="Edit display name"
                    aria-label="Edit display name"
                  >
                    <Pencil className="w-3 h-3" />
                  </button>
                </RequireRole>
              </div>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={() => refetch()} className="btn btn-secondary">
            <RefreshCw className="w-4 h-4" />
            Refresh
          </button>
          <RequireRole roles={["admin"]}>
            <button
              onClick={() => setShowDeleteConfirm(true)}
              className="btn bg-red-600 hover:bg-red-500 text-white"
            >
              <Trash2 className="w-4 h-4" />
              Delete
            </button>
          </RequireRole>
        </div>
      </div>

      {/* Hierarchy breadcrumb */}
      {hasHierarchy && (
        <div className="flex items-center flex-wrap gap-x-2 gap-y-1 text-sm">
          {ancestors.map((a) => (
            <span key={a.id} className="flex items-center gap-2">
              <TierBadge tier={a.product_tier} />
              <Link href={`/instances/${a.id}`} className="text-slate-300 hover:text-cyan-300">
                {a.instance_id}
              </Link>
              <span className="text-slate-600">&rsaquo;</span>
            </span>
          ))}
          {instance.parent_instance_id && !parentKnown && (
            <span className="flex items-center gap-2 text-slate-500">
              {instance.parent_instance_id} (not enrolled)
              <span className="text-slate-600">&rsaquo;</span>
            </span>
          )}
          <span className="flex items-center gap-2">
            <TierBadge tier={instance.product_tier} />
            <span className="text-white font-medium">{instance.instance_id}</span>
          </span>
        </div>
      )}

      {/* Error Banner */}
      {mutationError && (
        <div className="bg-red-900/50 border border-red-500 rounded-lg p-4 flex items-center justify-between">
          <span className="text-red-200">{mutationError}</span>
          <button
            onClick={() => setMutationError(null)}
            className="text-red-300 hover:text-white"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      )}

      {/* Main Content Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Instance Info */}
        <div className="card lg:col-span-2">
          <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
            <Server className="w-5 h-5 text-cyan-400" />
            Instance Details
          </h2>

          <div className="grid grid-cols-2 gap-4">
            <InfoRow label="Instance ID" value={instance.instance_id} />
            <InfoRow label="Type" value={instance.instance_type} />
            <InfoRow label="Hostname" value={instance.hostname} />
            <InfoRow label="Status" value={instance.status} />
            <InfoRow
              label="Last Heartbeat"
              value={
                instance.last_heartbeat
                  ? formatDistanceToNow(new Date(instance.last_heartbeat), { addSuffix: true })
                  : "Never"
              }
            />
            <InfoRow
              label="Created"
              value={format(new Date(instance.created_at), "PPp")}
            />
          </div>
        </div>

        {/* Update Settings */}
        <div className="card">
          <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
            <Settings className="w-5 h-5 text-cyan-400" />
            Update Settings
          </h2>

          <div className="space-y-4">
            {/* Auto Update Toggle */}
            <div className="flex items-center justify-between">
              <div>
                <p className="text-white font-medium">Auto Update</p>
                <p className="text-sm text-slate-400">Enable automatic updates</p>
              </div>
              <RequireRole
                roles={["admin"]}
                fallback={
                  <span className="text-sm text-slate-400">
                    {instance.auto_update_enabled ? "Enabled" : "Disabled"}
                  </span>
                }
              >
                <Switch
                  checked={instance.auto_update_enabled}
                  onChange={(next) => autoUpdateMutation.mutate(next)}
                  disabled={autoUpdateMutation.isPending}
                  label="Auto update"
                />
              </RequireRole>
            </div>

            {/* Update Group */}
            <div>
              <p className="text-white font-medium mb-2">Update Group</p>
              <RequireRole roles={["admin"]}>
                <div className="flex gap-2">
                  <select
                    aria-label="Update group"
                    value={effectiveGroup}
                    onChange={(e) => setSelectedGroup(e.target.value)}
                    className="input flex-1"
                  >
                    <option value="alpha">Alpha</option>
                    <option value="beta">Beta</option>
                    <option value="stable">Stable</option>
                    <option value="production">Production</option>
                  </select>
                  <button
                    onClick={() => updateGroupMutation.mutate(effectiveGroup)}
                    disabled={
                      updateGroupMutation.isPending ||
                      effectiveGroup === currentGroup
                    }
                    className="btn btn-primary"
                  >
                    Save
                  </button>
                </div>
              </RequireRole>
              <p className="text-xs text-slate-500 mt-1">
                Current: <span className="text-cyan-400">{instance.update_group || "stable"}</span>
              </p>
            </div>
          </div>
        </div>

        {/* Hierarchy */}
        {hasHierarchy && (
          <div className="card">
            <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
              <Network className="w-5 h-5 text-cyan-400" />
              Hierarchy
            </h2>
            <div className="space-y-3">
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-400">Product Tier</span>
                {instance.product_tier ? (
                  <TierBadge tier={instance.product_tier} />
                ) : (
                  <span className="text-slate-400">Untiered</span>
                )}
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-400">Parent</span>
                {parentKnown ? (
                  <Link href={`/instances/${parentKnown.id}`} className="text-cyan-400 hover:text-cyan-300">
                    {parentKnown.instance_id}
                  </Link>
                ) : instance.parent_instance_id ? (
                  <span className="text-amber-400" title="Declared parent not enrolled yet">
                    {instance.parent_instance_id} (orphan)
                  </span>
                ) : (
                  <span className="text-slate-400">
                    {(instance.product_tier || "").toLowerCase() === "mysoc" ? "Root" : "None"}
                  </span>
                )}
              </div>
              <div className="pt-2 border-t border-slate-700">
                <p className="text-xs text-slate-400 mb-2">Children ({children.length})</p>
                {children.length === 0 ? (
                  <p className="text-sm text-slate-500">No child nodes</p>
                ) : (
                  <ul className="space-y-1.5">
                    {children.map((child) => (
                      <li key={child.id} className="flex items-center gap-2">
                        <TierBadge tier={child.product_tier} />
                        <Link
                          href={`/instances/${child.id}`}
                          className="text-sm text-white hover:text-cyan-300 truncate"
                        >
                          {child.instance_id}
                        </Link>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
          </div>
        )}

        {/* Server Identification */}
        <div className="card">
          <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
            <Monitor className="w-5 h-5 text-cyan-400" />
            Server Identification
          </h2>
          <div className="space-y-3">
            <div className="flex items-center justify-between text-sm">
              <span className="text-slate-400">OS / Arch</span>
              <span className="text-white">{formatOsArch(heartbeat?.system)}</span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-slate-400 flex items-center gap-1">
                <Globe className="w-3 h-3" />
                IP Address
              </span>
              <span className="text-white">{instance.last_ip_address || "Unknown"}</span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-slate-400">Host Uptime</span>
              <span className="text-white">
                {/* Pre-1.8.4 updaters put their own process uptime here; only
                    trust it when real host measurements came with it. */}
                {telemetryReported ? formatUptime(heartbeat?.system?.uptime) : "Not reported"}
              </span>
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-slate-400">Updater Version</span>
              <span className="text-white">{heartbeat?.updater_version || "Unknown"}</span>
            </div>
            {heartbeat?.relay_guard && (
              <div className="flex items-center justify-between text-sm">
                <span
                  className="text-slate-400"
                  title="The relay listener protects its own open port automatically: unknown sources are restricted and rate-limited, repeated auth failures earn escalating temp-bans. Nothing to configure."
                >
                  Port Protection
                </span>
                <span className="text-white">
                  {heartbeat.relay_guard.learned_ips} trusted
                  {" · "}
                  {heartbeat.relay_guard.blocked + heartbeat.relay_guard.rate_limited + heartbeat.relay_guard.banned}{" "}
                  rejected
                  {heartbeat.relay_guard.active_bans > 0 && (
                    <span className="text-amber-400">
                      {" · "}
                      {heartbeat.relay_guard.active_bans} banned now
                    </span>
                  )}
                </span>
              </div>
            )}
          </div>
        </div>

        {/* License Information */}
        <div className="card">
          <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
            <KeyRound className="w-5 h-5 text-cyan-400" />
            License
          </h2>
          <div className="space-y-3">
            <div className="flex items-center justify-between text-sm">
              <span className="text-slate-400">Status</span>
              {viaRelay && !heartbeat?.license?.valid ? (
                <span className="text-slate-400" title="Cascade children authenticate to their parent relay; only the tier-1 platform licenses against this server">
                  N/A — via relay
                </span>
              ) : (
                <span
                  className={
                    heartbeat?.license?.valid
                      ? "text-emerald-400"
                      : heartbeat?.license?.valid === false
                      ? "text-red-400"
                      : "text-slate-400"
                  }
                >
                  {heartbeat?.license?.valid === undefined
                    ? instance.license_id
                      ? "Unknown"
                      : "No license"
                    : heartbeat.license.valid
                    ? "Valid"
                    : "Invalid"}
                </span>
              )}
            </div>
            {viaRelay && !heartbeat?.license?.valid && (
              <p className="text-xs text-slate-500">
                This node is covered by its operator&apos;s platform license and
                authenticates to its parent relay
                {instance.reported_via ? ` (${instance.reported_via})` : ""}.
              </p>
            )}
            {licenseExpiry && (
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-400">Expires</span>
                <span className="text-white">{format(licenseExpiry, "PPP")}</span>
              </div>
            )}
            {instance.license_id && (
              <div className="pt-2 border-t border-slate-700">
                <Link
                  href={`/licenses/${instance.license_id}`}
                  className="text-cyan-400 hover:text-cyan-300 text-sm flex items-center gap-1"
                >
                  View License Details
                  <ArrowLeft className="w-3 h-3 rotate-180" />
                </Link>
              </div>
            )}
          </div>
        </div>

        {/* Last Update Attempt */}
        {instance.last_update_at && (
          <div className="card">
            <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
              <ArrowUpCircle className="w-5 h-5 text-cyan-400" />
              Last Update
            </h2>
            <div className="space-y-3">
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-400">From Version</span>
                <span className="text-white">{instance.last_update_from_version || "—"}</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-400">To Version</span>
                <span className="text-white">{instance.last_update_target_version || "—"}</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-400">Status</span>
                <span
                  className={
                    instance.last_update_success === undefined
                      ? "text-slate-400"
                      : instance.last_update_success
                        ? "text-emerald-400"
                        : "text-red-400"
                  }
                >
                  {instance.last_update_success === undefined
                    ? "Unknown"
                    : instance.last_update_success
                      ? "Success"
                      : "Failed"}
                </span>
              </div>
              {instance.last_update_error && (
                <div className="text-sm">
                  <span className="text-slate-400">Error</span>
                  <p className="text-red-400 mt-1 text-xs">{instance.last_update_error}</p>
                </div>
              )}
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-400">When</span>
                <span className="text-white">
                  {formatDistanceToNow(new Date(instance.last_update_at), { addSuffix: true })}
                </span>
              </div>
            </div>
          </div>
        )}

        {/* System Metrics */}
        {heartbeat?.system && (
          <div className="card">
            <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
              <Activity className="w-5 h-5 text-cyan-400" />
              System Metrics
            </h2>

            {!telemetryReported ? (
              <p className="text-sm text-slate-500">
                Not reported — this updater does not collect host metrics.
              </p>
            ) : (
            <div className="space-y-4">
              <MetricBar
                label="CPU"
                value={heartbeat.system.cpu_usage}
                icon={<Cpu className="w-4 h-4" />}
              />
              <MetricBar
                label="Memory"
                value={
                  heartbeat.system.memory_total > 0
                    ? (heartbeat.system.memory_used / heartbeat.system.memory_total) * 100
                    : 0
                }
                subtext={`${formatBytes(heartbeat.system.memory_used)} / ${formatBytes(heartbeat.system.memory_total)}`}
                icon={<HardDrive className="w-4 h-4" />}
              />
              <MetricBar
                label="Disk"
                value={
                  heartbeat.system.disk_total > 0
                    ? (heartbeat.system.disk_used / heartbeat.system.disk_total) * 100
                    : 0
                }
                subtext={`${formatBytes(heartbeat.system.disk_used)} / ${formatBytes(heartbeat.system.disk_total)}`}
                icon={<HardDrive className="w-4 h-4" />}
              />
            </div>
            )}
          </div>
        )}

        {/* Products */}
        {heartbeat?.products && heartbeat.products.length > 0 && (
          <div className="card">
            <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
              <Power className="w-5 h-5 text-cyan-400" />
              Products
            </h2>

            <div className="space-y-3">
              {heartbeat.products.map((product) => (
                <div
                  key={product.name}
                  className="flex items-center justify-between p-3 rounded-lg bg-slate-800/50"
                >
                  <div>
                    <p className="text-white font-medium">{product.name}</p>
                    <p className="text-sm text-slate-400">v{product.version}</p>
                  </div>
                  <span
                    className={`text-xs px-2 py-1 rounded ${
                      product.status === "running"
                        ? "bg-emerald-500/20 text-emerald-400"
                        : "bg-red-500/20 text-red-400"
                    }`}
                  >
                    {product.status || "unknown"}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Security */}
        {heartbeat?.security && (
          <div className="card">
            <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
              <Shield className="w-5 h-5 text-cyan-400" />
              Security
            </h2>

            {/* A heartbeat without a security scan carries a zero-valued
                security block; rendering it would show a healthy host as
                "Firewall Disabled, score 0/100". */}
            {!securityReported ? (
              <p className="text-sm text-slate-500">
                Not reported — this updater does not collect security posture.
              </p>
            ) : (
            <div className="space-y-3">
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-400">Security Score</span>
                <span className="text-white">{heartbeat.security.security_score}/100</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-400">Firewall</span>
                <span className={heartbeat.security.firewall_enabled ? "text-emerald-400" : "text-red-400"}>
                  {heartbeat.security.firewall_enabled ? "Enabled" : "Disabled"}
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-400">SSH Hardened</span>
                <span className={heartbeat.security.ssh_hardened ? "text-emerald-400" : "text-yellow-400"}>
                  {heartbeat.security.ssh_hardened ? "Yes" : "No"}
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-400">Pending Updates</span>
                <span className="text-white">{heartbeat.security.pending_updates}</span>
              </div>
            </div>
            )}
          </div>
        )}
      </div>

      {/* Delete Confirmation Modal */}
      {showDeleteConfirm && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-700 rounded-xl p-6 w-full max-w-md mx-4 shadow-2xl">
            <div className="flex flex-col items-center text-center">
              <div className="w-14 h-14 rounded-full bg-red-500/20 flex items-center justify-center mb-4">
                <Trash2 className="w-7 h-7 text-red-400" />
              </div>
              
              <h2 className="text-xl font-semibold text-white mb-2">
                Delete Instance
              </h2>
              
              <p className="text-slate-400 mb-1">
                Are you sure you want to delete this instance?
              </p>
              
              <div className="bg-slate-800 rounded-lg px-4 py-2 my-3">
                <span className="text-cyan-400 font-medium">{instance.instance_id}</span>
              </div>
              
              <p className="text-sm text-slate-500 mb-6">
                This will remove all heartbeat history and settings for this instance.
              </p>

              <div className="flex gap-3 w-full">
                <button
                  onClick={() => setShowDeleteConfirm(false)}
                  className="flex-1 px-4 py-2.5 rounded-lg border border-slate-700 text-slate-300 hover:bg-slate-800 transition-colors font-medium"
                >
                  Cancel
                </button>
                <button
                  onClick={() => deleteInstanceMutation.mutate()}
                  disabled={deleteInstanceMutation.isPending}
                  className="flex-1 px-4 py-2.5 rounded-lg bg-red-600 hover:bg-red-500 text-white font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
                >
                  {deleteInstanceMutation.isPending ? (
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

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-sm text-slate-400">{label}</p>
      <p className="text-white font-medium">{value}</p>
    </div>
  );
}

function MetricBar({
  label,
  value,
  subtext,
  icon,
}: {
  label: string;
  value: number;
  subtext?: string;
  icon: React.ReactNode;
}) {
  const getColor = (v: number) => {
    if (v < 60) return "bg-emerald-500";
    if (v < 80) return "bg-yellow-500";
    return "bg-red-500";
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <span className="text-slate-400 flex items-center gap-2">
          {icon}
          {label}
        </span>
        <span className="text-white">{value.toFixed(1)}%</span>
      </div>
      <div className="h-2 bg-slate-700 rounded-full overflow-hidden">
        <div
          className={`h-full ${getColor(value)} transition-all duration-300`}
          style={{ width: `${Math.min(100, value)}%` }}
        />
      </div>
      {subtext && <p className="text-xs text-slate-500 mt-1">{subtext}</p>}
    </div>
  );
}
