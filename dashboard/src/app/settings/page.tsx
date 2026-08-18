"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { Server, Key, Bell, Shield, Database } from "lucide-react";
import { api } from "@/lib/api";
import { Switch } from "@/components/ui";
import { ApiKeysManager } from "@/components/ApiKeysManager";

const DASHBOARD_VERSION = process.env.NEXT_PUBLIC_APP_VERSION || "dev";
const API_ORIGIN = process.env.NEXT_PUBLIC_API_URL || "same origin";

export default function SettingsPage() {
  const { data: health } = useQuery({
    queryKey: ["health"],
    queryFn: () => api.getHealth(),
    retry: false,
    staleTime: 60_000,
  });

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-3xl font-bold text-white">Settings</h1>
        <p className="text-slate-400 mt-1">
          Configure update server and dashboard settings
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Server Configuration */}
        <div className="card">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-lg bg-cyan-500/20">
              <Server className="w-5 h-5 text-cyan-400" />
            </div>
            <h2 className="text-lg font-semibold text-white">
              Server Configuration
            </h2>
          </div>

          <div className="space-y-4">
            <div>
              <label
                htmlFor="settings-server-url"
                className="block text-sm text-slate-400 mb-2"
              >
                API Endpoint
              </label>
              <input
                id="settings-server-url"
                type="text"
                value={API_ORIGIN}
                readOnly
                className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-300"
              />
            </div>
            <div>
              <label
                htmlFor="settings-api-version"
                className="block text-sm text-slate-400 mb-2"
              >
                API Version
              </label>
              <input
                id="settings-api-version"
                type="text"
                value="v1"
                readOnly
                className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-300"
              />
            </div>
          </div>
        </div>

        {/* API Keys */}
        <div className="card lg:col-span-2">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-lg bg-amber-500/20">
              <Key className="w-5 h-5 text-amber-400" />
            </div>
            <h2 className="text-lg font-semibold text-white">API Keys</h2>
          </div>

          <ApiKeysManager />
        </div>

        {/* Notifications */}
        <div className="card">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-lg bg-violet-500/20">
              <Bell className="w-5 h-5 text-violet-400" />
            </div>
            <h2 className="text-lg font-semibold text-white">Notifications</h2>
            <span className="ml-auto px-2 py-1 rounded-full bg-slate-700 text-slate-400 text-xs">
              Coming soon
            </span>
          </div>

          <div className="space-y-4 opacity-60">
            {[
              {
                title: "Instance Offline Alerts",
                desc: "Get notified when instances go offline",
              },
              {
                title: "License Expiry Warnings",
                desc: "Alert before licenses expire",
              },
              {
                title: "Security Alerts",
                desc: "Critical security notifications",
              },
            ].map((n) => (
              <div key={n.title} className="flex items-center justify-between">
                <div>
                  <p className="text-white">{n.title}</p>
                  <p className="text-xs text-slate-500">{n.desc}</p>
                </div>
                <Switch
                  checked={false}
                  onChange={() => {}}
                  disabled
                  label={n.title}
                />
              </div>
            ))}
          </div>
        </div>

        {/* Security */}
        <div className="card">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-lg bg-emerald-500/20">
              <Shield className="w-5 h-5 text-emerald-400" />
            </div>
            <h2 className="text-lg font-semibold text-white">Security</h2>
          </div>

          <div className="space-y-4">
            <div>
              <p className="text-sm text-slate-400 mb-2">
                Two-Factor Authentication
              </p>
              <Link href="/profile" className="btn btn-secondary text-sm">
                Manage 2FA in your profile
              </Link>
            </div>
          </div>
        </div>

        {/* System Info */}
        <div className="card lg:col-span-2">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-lg bg-rose-500/20">
              <Database className="w-5 h-5 text-rose-400" />
            </div>
            <h2 className="text-lg font-semibold text-white">System Info</h2>
          </div>

          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div className="p-4 rounded-lg bg-slate-800/50">
              <p className="text-xs text-slate-500 mb-1">Database</p>
              <p className="text-white font-medium">PostgreSQL</p>
            </div>
            <div className="p-4 rounded-lg bg-slate-800/50">
              <p className="text-xs text-slate-500 mb-1">Storage</p>
              <p className="text-white font-medium">Local Filesystem</p>
            </div>
            <div className="p-4 rounded-lg bg-slate-800/50">
              <p className="text-xs text-slate-500 mb-1">Server Version</p>
              <p className="text-white font-medium">
                {health?.version || "Unavailable"}
              </p>
            </div>
            <div className="p-4 rounded-lg bg-slate-800/50">
              <p className="text-xs text-slate-500 mb-1">Dashboard Version</p>
              <p className="text-white font-medium">{DASHBOARD_VERSION}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
