"use client";

import { Settings, Server, Key, Bell, Shield, Database } from "lucide-react";

export default function SettingsPage() {
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
              <label className="block text-sm text-slate-400 mb-2">
                Server URL
              </label>
              <input
                type="text"
                value="https://updates.mysoc.ai"
                disabled
                className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-300"
              />
            </div>
            <div>
              <label className="block text-sm text-slate-400 mb-2">
                API Version
              </label>
              <input
                type="text"
                value="v1"
                disabled
                className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-300"
              />
            </div>
          </div>
        </div>

        {/* API Keys */}
        <div className="card">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-lg bg-amber-500/20">
              <Key className="w-5 h-5 text-amber-400" />
            </div>
            <h2 className="text-lg font-semibold text-white">API Keys</h2>
          </div>

          <div className="space-y-4">
            <div>
              <label className="block text-sm text-slate-400 mb-2">
                Admin API Key
              </label>
              <div className="flex gap-2">
                <input
                  type="password"
                  value="••••••••••••••••"
                  disabled
                  className="flex-1 px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-300"
                />
                <button className="btn btn-secondary text-sm">Regenerate</button>
              </div>
              <p className="text-xs text-slate-500 mt-2">
                Used for administrative operations like creating licenses and
                uploading releases.
              </p>
            </div>
          </div>
        </div>

        {/* Notifications */}
        <div className="card">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-lg bg-violet-500/20">
              <Bell className="w-5 h-5 text-violet-400" />
            </div>
            <h2 className="text-lg font-semibold text-white">Notifications</h2>
          </div>

          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-white">Instance Offline Alerts</p>
                <p className="text-xs text-slate-500">
                  Get notified when instances go offline
                </p>
              </div>
              <div className="w-12 h-6 rounded-full bg-slate-700 relative cursor-not-allowed opacity-50">
                <div className="w-5 h-5 rounded-full bg-slate-500 absolute top-0.5 left-0.5"></div>
              </div>
            </div>

            <div className="flex items-center justify-between">
              <div>
                <p className="text-white">License Expiry Warnings</p>
                <p className="text-xs text-slate-500">
                  Alert before licenses expire
                </p>
              </div>
              <div className="w-12 h-6 rounded-full bg-slate-700 relative cursor-not-allowed opacity-50">
                <div className="w-5 h-5 rounded-full bg-slate-500 absolute top-0.5 left-0.5"></div>
              </div>
            </div>

            <div className="flex items-center justify-between">
              <div>
                <p className="text-white">Security Alerts</p>
                <p className="text-xs text-slate-500">
                  Critical security notifications
                </p>
              </div>
              <div className="w-12 h-6 rounded-full bg-slate-700 relative cursor-not-allowed opacity-50">
                <div className="w-5 h-5 rounded-full bg-slate-500 absolute top-0.5 left-0.5"></div>
              </div>
            </div>
          </div>

          <p className="text-xs text-slate-500 mt-4 italic">
            Notification settings coming soon
          </p>
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
              <label className="block text-sm text-slate-400 mb-2">
                Session Timeout
              </label>
              <select
                disabled
                className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-300"
              >
                <option>30 minutes</option>
                <option>1 hour</option>
                <option>4 hours</option>
                <option>24 hours</option>
              </select>
            </div>

            <div>
              <label className="block text-sm text-slate-400 mb-2">
                Two-Factor Authentication
              </label>
              <button className="btn btn-secondary text-sm opacity-50 cursor-not-allowed">
                Enable 2FA (Coming Soon)
              </button>
            </div>
          </div>
        </div>

        {/* Database */}
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
              <p className="text-white font-medium">1.0.0</p>
            </div>
            <div className="p-4 rounded-lg bg-slate-800/50">
              <p className="text-xs text-slate-500 mb-1">Dashboard Version</p>
              <p className="text-white font-medium">1.0.0</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
