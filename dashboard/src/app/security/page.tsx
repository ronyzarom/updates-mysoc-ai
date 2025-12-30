"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import {
  Shield,
  AlertTriangle,
  CheckCircle,
  XCircle,
  RefreshCw,
  Flame,
  Lock,
  FileCheck,
  Wifi,
} from "lucide-react";

export default function SecurityPage() {
  const { data: instances, isLoading, refetch } = useQuery({
    queryKey: ["instances"],
    queryFn: () => api.getInstances(),
  });

  // Calculate security stats
  const securityStats = instances?.reduce(
    (acc, instance) => {
      const security = instance.last_heartbeat_data?.security;
      if (security) {
        acc.totalScore += security.security_score || 0;
        acc.count++;
        if (security.firewall_enabled) acc.firewallEnabled++;
        if (security.ssh_hardened) acc.sshHardened++;
        acc.pendingUpdates += security.pending_updates || 0;
        acc.securityUpdates += security.security_updates || 0;
        if (security.reboot_required) acc.rebootRequired++;
      }
      return acc;
    },
    {
      totalScore: 0,
      count: 0,
      firewallEnabled: 0,
      sshHardened: 0,
      pendingUpdates: 0,
      securityUpdates: 0,
      rebootRequired: 0,
    }
  ) || {
    totalScore: 0,
    count: 0,
    firewallEnabled: 0,
    sshHardened: 0,
    pendingUpdates: 0,
    securityUpdates: 0,
    rebootRequired: 0,
  };

  const avgScore =
    securityStats.count > 0
      ? Math.round(securityStats.totalScore / securityStats.count)
      : 0;

  const getScoreColor = (score: number) => {
    if (score >= 80) return "text-emerald-400";
    if (score >= 60) return "text-amber-400";
    return "text-red-400";
  };

  const getScoreBg = (score: number) => {
    if (score >= 80) return "bg-emerald-500/20";
    if (score >= 60) return "bg-amber-500/20";
    return "bg-red-500/20";
  };

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-white">Security</h1>
          <p className="text-slate-400 mt-1">
            Fleet security posture and compliance
          </p>
        </div>
        <button onClick={() => refetch()} className="btn btn-secondary">
          <RefreshCw className="w-4 h-4" />
          Refresh
        </button>
      </div>

      {/* Security Score Overview */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="card lg:col-span-1">
          <div className="flex flex-col items-center justify-center py-8">
            <div
              className={`w-32 h-32 rounded-full ${getScoreBg(avgScore)} flex items-center justify-center mb-4`}
            >
              <span className={`text-5xl font-bold ${getScoreColor(avgScore)}`}>
                {avgScore}
              </span>
            </div>
            <p className="text-slate-400 text-sm">Average Security Score</p>
            <p className="text-white font-medium mt-1">
              {securityStats.count} instances reporting
            </p>
          </div>
        </div>

        <div className="lg:col-span-2 grid grid-cols-2 gap-4">
          <div className="card">
            <div className="flex items-start gap-3">
              <div className="p-2 rounded-lg bg-emerald-500/20">
                <Flame className="w-5 h-5 text-emerald-400" />
              </div>
              <div>
                <p className="text-2xl font-bold text-white">
                  {securityStats.firewallEnabled}/{securityStats.count || instances?.length || 0}
                </p>
                <p className="text-sm text-slate-400">Firewall Enabled</p>
              </div>
            </div>
          </div>

          <div className="card">
            <div className="flex items-start gap-3">
              <div className="p-2 rounded-lg bg-cyan-500/20">
                <Lock className="w-5 h-5 text-cyan-400" />
              </div>
              <div>
                <p className="text-2xl font-bold text-white">
                  {securityStats.sshHardened}/{securityStats.count || instances?.length || 0}
                </p>
                <p className="text-sm text-slate-400">SSH Hardened</p>
              </div>
            </div>
          </div>

          <div className="card">
            <div className="flex items-start gap-3">
              <div className="p-2 rounded-lg bg-amber-500/20">
                <FileCheck className="w-5 h-5 text-amber-400" />
              </div>
              <div>
                <p className="text-2xl font-bold text-white">
                  {securityStats.pendingUpdates}
                </p>
                <p className="text-sm text-slate-400">Pending Updates</p>
              </div>
            </div>
          </div>

          <div className="card">
            <div className="flex items-start gap-3">
              <div className="p-2 rounded-lg bg-red-500/20">
                <AlertTriangle className="w-5 h-5 text-red-400" />
              </div>
              <div>
                <p className="text-2xl font-bold text-white">
                  {securityStats.rebootRequired}
                </p>
                <p className="text-sm text-slate-400">Reboot Required</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Instance Security Status */}
      <div className="card">
        <h2 className="text-lg font-semibold text-white mb-4">
          Instance Security Status
        </h2>

        {isLoading ? (
          <div className="text-slate-400">Loading...</div>
        ) : (
          <div className="table-container">
            <table className="table">
              <thead>
                <tr>
                  <th>Instance</th>
                  <th>Score</th>
                  <th>Firewall</th>
                  <th>SSH</th>
                  <th>Updates</th>
                  <th>Reboot</th>
                </tr>
              </thead>
              <tbody>
                {instances?.map((instance) => {
                  const security = instance.last_heartbeat_data?.security;
                  const score = security?.security_score || 0;

                  return (
                    <tr key={instance.id}>
                      <td>
                        <div className="flex items-center gap-2">
                          <Shield className="w-4 h-4 text-slate-400" />
                          <span className="font-medium text-white">
                            {instance.instance_id}
                          </span>
                        </div>
                      </td>
                      <td>
                        <div className="flex items-center gap-2">
                          <div
                            className={`w-8 h-8 rounded-full ${getScoreBg(
                              score
                            )} flex items-center justify-center`}
                          >
                            <span
                              className={`text-sm font-bold ${getScoreColor(
                                score
                              )}`}
                            >
                              {score}
                            </span>
                          </div>
                        </div>
                      </td>
                      <td>
                        {security?.firewall_enabled ? (
                          <CheckCircle className="w-5 h-5 text-emerald-400" />
                        ) : (
                          <XCircle className="w-5 h-5 text-red-400" />
                        )}
                      </td>
                      <td>
                        {security?.ssh_hardened ? (
                          <CheckCircle className="w-5 h-5 text-emerald-400" />
                        ) : (
                          <XCircle className="w-5 h-5 text-red-400" />
                        )}
                      </td>
                      <td>
                        {(security?.pending_updates || 0) > 0 ? (
                          <span className="text-amber-400">
                            {security?.pending_updates} pending
                          </span>
                        ) : (
                          <span className="text-emerald-400">Up to date</span>
                        )}
                      </td>
                      <td>
                        {security?.reboot_required ? (
                          <span className="status-badge status-degraded">
                            Required
                          </span>
                        ) : (
                          <span className="text-slate-400">No</span>
                        )}
                      </td>
                    </tr>
                  );
                })}

                {(!instances || instances.length === 0) && (
                  <tr>
                    <td colSpan={6} className="text-center py-16">
                      <Shield className="w-12 h-12 text-slate-600 mx-auto mb-4" />
                      <h3 className="text-lg font-medium text-white mb-2">
                        No security data
                      </h3>
                      <p className="text-slate-400">
                        Security status will appear once instances start
                        reporting.
                      </p>
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

