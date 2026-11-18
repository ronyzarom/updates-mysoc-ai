"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  User,
  Shield,
  Key,
  Clock,
  Monitor,
  AlertCircle,
  CheckCircle,
  Copy,
  Lock,
  Loader2,
  Eye,
  EyeOff,
  QrCode,
  LogOut,
} from "lucide-react";
import { api, Session, AuditEvent, MFASetupResponse } from "@/lib/api";
import { useAuth, RequireAuth } from "@/lib/auth-context";
import { isCompleteMfaCode, normalizeMfaInput } from "@/lib/mfa";

function ProfileContent() {
  const { user, refreshUser, logout } = useAuth();
  const queryClient = useQueryClient();
  const router = useRouter();

  // State
  const [name, setName] = useState(user?.name || "");

  // Keep the editable name in sync when the authenticated user loads or changes
  // (the user is null on first render while the profile is still loading).
  useEffect(() => {
    setName(user?.name || "");
  }, [user?.name]);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [showPasswords, setShowPasswords] = useState(false);
  const [mfaSetup, setMfaSetup] = useState<MFASetupResponse | null>(null);
  const [mfaCode, setMfaCode] = useState("");
  const [backupCodes, setBackupCodes] = useState<string[]>([]);
  const [disableMfaPassword, setDisableMfaPassword] = useState("");
  const [disableMfaCode, setDisableMfaCode] = useState("");
  const [disableWithBackup, setDisableWithBackup] = useState(false);
  const [showDisableMfa, setShowDisableMfa] = useState(false);
  const [message, setMessage] = useState<{ type: "success" | "error"; text: string } | null>(null);

  // Queries
  const { data: sessions } = useQuery({
    queryKey: ["sessions"],
    queryFn: () => api.getSessions(),
  });

  const { data: auditLog } = useQuery({
    queryKey: ["audit"],
    queryFn: () => api.getAuditLog(),
  });

  // Mutations
  const updateProfileMutation = useMutation({
    mutationFn: (data: { name: string }) => api.updateProfile(data.name),
    onSuccess: () => {
      refreshUser();
      setMessage({ type: "success", text: "Profile updated successfully" });
    },
    onError: (error) => {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "Failed to update profile" });
    },
  });

  const changePasswordMutation = useMutation({
    mutationFn: (data: { currentPassword: string; newPassword: string }) =>
      api.changePassword(data.currentPassword, data.newPassword),
    onSuccess: () => {
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setMessage({ type: "success", text: "Password changed successfully. Redirecting to sign in..." });
      // The server revokes all sessions on password change, so the current
      // tokens are now invalid. End the session through the auth context (so
      // `user` is cleared too — clearing only the tokens left the app
      // "authenticated" and bounced login straight back to a 401 dashboard).
      setTimeout(async () => {
        await logout();
        router.replace("/login");
      }, 1500);
    },
    onError: (error) => {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "Failed to change password" });
    },
  });

  const setupMfaMutation = useMutation({
    mutationFn: () => api.setupMFA(),
    onSuccess: (data) => {
      setMfaSetup(data);
    },
    onError: (error) => {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "Failed to setup MFA" });
    },
  });

  const enableMfaMutation = useMutation({
    mutationFn: (code: string) => api.enableMFA(code),
    onSuccess: (data) => {
      // Show the one-time backup codes and hold them on screen. refreshUser()
      // is deliberately deferred to "I saved them": refreshing flips
      // mfa_enabled, and the codes used to vanish with the setup branch.
      setBackupCodes(data.backup_codes);
      setMfaSetup(null);
      setMfaCode("");
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
    onError: (error) => {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "Failed to enable MFA" });
    },
  });

  const disableMfaMutation = useMutation({
    mutationFn: (data: { password: string; code: string }) =>
      api.disableMFA(data.password, data.code),
    onSuccess: () => {
      setShowDisableMfa(false);
      setDisableMfaPassword("");
      setDisableMfaCode("");
      setDisableWithBackup(false);
      refreshUser();
      setMessage({ type: "success", text: "MFA disabled successfully" });
    },
    onError: (error) => {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "Failed to disable MFA" });
    },
  });

  const handleUpdateProfile = (e: React.FormEvent) => {
    e.preventDefault();
    updateProfileMutation.mutate({ name });
  };

  const handleChangePassword = (e: React.FormEvent) => {
    e.preventDefault();
    if (newPassword !== confirmPassword) {
      setMessage({ type: "error", text: "Passwords do not match" });
      return;
    }
    if (newPassword.length < 8) {
      setMessage({ type: "error", text: "Password must be at least 8 characters" });
      return;
    }
    changePasswordMutation.mutate({ currentPassword, newPassword });
  };

  const handleEnableMFA = (e: React.FormEvent) => {
    e.preventDefault();
    enableMfaMutation.mutate(mfaCode);
  };

  const handleDisableMFA = (e: React.FormEvent) => {
    e.preventDefault();
    disableMfaMutation.mutate({ password: disableMfaPassword, code: disableMfaCode });
  };

  const logoutAllMutation = useMutation({
    mutationFn: () => api.logoutAll(),
    onSuccess: async () => {
      await logout();
      router.replace("/login");
    },
    onError: (error) => {
      setMessage({
        type: "error",
        text: error instanceof Error ? `Could not sign out other sessions: ${error.message}` : "Could not sign out other sessions",
      });
    },
  });

  // copyToClipboard reports what actually happened: navigator.clipboard is
  // missing on plain-HTTP origins and writeText can be refused.
  const copyToClipboard = async (text: string) => {
    try {
      if (!navigator.clipboard) throw new Error("clipboard unavailable");
      await navigator.clipboard.writeText(text);
      setMessage({ type: "success", text: "Copied to clipboard" });
    } catch {
      setMessage({ type: "error", text: "Could not copy automatically — select the text and copy it manually." });
    }
  };

  const finishBackupCodes = () => {
    setBackupCodes([]);
    refreshUser();
  };

  const formatDate = (date: string) => {
    return new Date(date).toLocaleString();
  };

  const getEventIcon = (eventType: string) => {
    switch (eventType) {
      case "login":
        return <CheckCircle className="w-4 h-4 text-green-400" />;
      case "logout":
        return <Clock className="w-4 h-4 text-slate-400" />;
      case "failed_login":
      case "failed_mfa":
        return <AlertCircle className="w-4 h-4 text-red-400" />;
      case "mfa_enable":
      case "mfa_disable":
        return <Shield className="w-4 h-4 text-violet-400" />;
      case "password_change":
        return <Key className="w-4 h-4 text-amber-400" />;
      default:
        return <Clock className="w-4 h-4 text-slate-400" />;
    }
  };

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-3xl font-bold text-white">Profile</h1>
        <p className="text-slate-400 mt-1">Manage your account settings and security</p>
      </div>

      {message && (
        <div
          className={`flex items-center gap-2 p-4 rounded-lg ${
            message.type === "success"
              ? "bg-green-500/10 border border-green-500/30 text-green-400"
              : "bg-red-500/10 border border-red-500/30 text-red-400"
          }`}
        >
          {message.type === "success" ? (
            <CheckCircle className="w-5 h-5" />
          ) : (
            <AlertCircle className="w-5 h-5" />
          )}
          {message.text}
          <button
            onClick={() => setMessage(null)}
            className="ml-auto text-current hover:opacity-70"
          >
            ×
          </button>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Profile Info */}
        <div className="card">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-lg bg-cyan-500/20">
              <User className="w-5 h-5 text-cyan-400" />
            </div>
            <h2 className="text-lg font-semibold text-white">Profile Information</h2>
          </div>

          <form onSubmit={handleUpdateProfile} className="space-y-4">
            <div>
              <label className="block text-sm text-slate-400 mb-2">Email</label>
              <input
                type="email"
                disabled
                className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-400"
                value={user?.email || ""}
              />
            </div>

            <div>
              <label className="block text-sm text-slate-400 mb-2">Name</label>
              <input
                type="text"
                className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>

            <div>
              <label className="block text-sm text-slate-400 mb-2">Role</label>
              <input
                type="text"
                disabled
                className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-400 capitalize"
                value={user?.role || ""}
              />
            </div>

            <button
              type="submit"
              disabled={updateProfileMutation.isPending}
              className="btn btn-primary w-full flex items-center justify-center gap-2"
            >
              {updateProfileMutation.isPending && <Loader2 className="w-4 h-4 animate-spin" />}
              Save Changes
            </button>
          </form>
        </div>

        {/* Change Password */}
        <div className="card">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-lg bg-amber-500/20">
              <Key className="w-5 h-5 text-amber-400" />
            </div>
            <h2 className="text-lg font-semibold text-white">Change Password</h2>
          </div>

          <form onSubmit={handleChangePassword} className="space-y-4">
            <div className="relative">
              <label className="block text-sm text-slate-400 mb-2">Current Password</label>
              <input
                type={showPasswords ? "text" : "password"}
                className="w-full px-4 py-2 pr-10 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
              />
            </div>

            <div className="relative">
              <label className="block text-sm text-slate-400 mb-2">New Password</label>
              <input
                type={showPasswords ? "text" : "password"}
                className="w-full px-4 py-2 pr-10 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
              />
            </div>

            <div className="relative">
              <label className="block text-sm text-slate-400 mb-2">Confirm New Password</label>
              <div className="relative">
                <input
                  type={showPasswords ? "text" : "password"}
                  className="w-full px-4 py-2 pr-10 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                />
                <button
                  type="button"
                  onClick={() => setShowPasswords(!showPasswords)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-slate-400 hover:text-white"
                >
                  {showPasswords ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
            </div>

            <button
              type="submit"
              disabled={changePasswordMutation.isPending}
              className="btn btn-primary w-full flex items-center justify-center gap-2"
            >
              {changePasswordMutation.isPending && <Loader2 className="w-4 h-4 animate-spin" />}
              Change Password
            </button>
          </form>
        </div>

        {/* MFA Settings */}
        <div className="card lg:col-span-2">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-lg bg-violet-500/20">
              <Shield className="w-5 h-5 text-violet-400" />
            </div>
            <h2 className="text-lg font-semibold text-white">Two-Factor Authentication</h2>
            {user?.mfa_enabled && (
              <span className="ml-auto px-2 py-1 rounded-full bg-green-500/20 text-green-400 text-xs font-medium">
                Enabled
              </span>
            )}
          </div>

          {backupCodes.length > 0 ? (
            // One-time backup codes: shown until the user confirms they saved
            // them, regardless of the (refreshed) mfa_enabled state.
            <div className="p-4 bg-amber-500/10 border border-amber-500/30 rounded-lg" role="alert">
              <div className="flex items-center gap-2 mb-3">
                <AlertCircle className="w-5 h-5 text-amber-400" />
                <h3 className="text-amber-400 font-medium">2FA is on — save your backup codes now</h3>
              </div>
              <p className="text-slate-400 text-sm mb-4">
                These codes are shown only once. Each one signs you in a single time (or turns 2FA off)
                if you lose your authenticator. Store them somewhere safe before continuing.
              </p>
              <div className="grid grid-cols-2 gap-2 select-all">
                {backupCodes.map((code, i) => (
                  <code key={i} className="px-3 py-2 bg-slate-800 rounded text-center font-mono text-sm text-white">
                    {code}
                  </code>
                ))}
              </div>
              <div className="mt-4 flex flex-wrap gap-3">
                <button
                  type="button"
                  onClick={() => copyToClipboard(backupCodes.join("\n"))}
                  className="btn btn-secondary flex items-center gap-2"
                >
                  <Copy className="w-4 h-4" />
                  Copy All Codes
                </button>
                <button type="button" onClick={finishBackupCodes} className="btn btn-primary flex items-center gap-2">
                  <CheckCircle className="w-4 h-4" />
                  I saved them
                </button>
              </div>
            </div>
          ) : !user?.mfa_enabled ? (
            <div className="space-y-4">
              {!mfaSetup ? (
                <div className="flex flex-col items-center gap-4 py-8">
                  <div className="p-4 rounded-full bg-slate-800">
                    <Lock className="w-8 h-8 text-slate-400" />
                  </div>
                  <div className="text-center">
                    <h3 className="text-white font-medium">Secure your account</h3>
                    <p className="text-slate-400 text-sm mt-1">
                      Add an extra layer of security with Google Authenticator
                    </p>
                  </div>
                  <button
                    onClick={() => setupMfaMutation.mutate()}
                    disabled={setupMfaMutation.isPending}
                    className="btn btn-primary flex items-center gap-2"
                  >
                    {setupMfaMutation.isPending && <Loader2 className="w-4 h-4 animate-spin" />}
                    <QrCode className="w-4 h-4" />
                    Enable 2FA
                  </button>
                </div>
              ) : (
                <div className="space-y-6">
                  <div className="flex flex-col md:flex-row gap-6 items-center">
                    <div className="flex-shrink-0">
                      {/* eslint-disable-next-line @next/next/no-img-element */}
                      <img
                        src={mfaSetup.qr_code_data}
                        alt="QR Code"
                        className="w-48 h-48 rounded-lg bg-white p-2"
                      />
                    </div>
                    <div className="flex-1 space-y-4">
                      <div>
                        <h3 className="text-white font-medium mb-2">
                          1. Scan the QR code
                        </h3>
                        <p className="text-slate-400 text-sm">
                          Open Google Authenticator and scan this QR code
                        </p>
                      </div>

                      <div>
                        <h3 className="text-white font-medium mb-2">
                          2. Or enter this code manually
                        </h3>
                        <div className="flex items-center gap-2">
                          <code className="px-3 py-2 bg-slate-800 rounded-lg text-cyan-400 font-mono text-sm">
                            {mfaSetup.secret}
                          </code>
                          <button
                            onClick={() => copyToClipboard(mfaSetup.secret)}
                            className="p-2 text-slate-400 hover:text-white"
                          >
                            <Copy className="w-4 h-4" />
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>

                  <form onSubmit={handleEnableMFA} className="space-y-4">
                    <div>
                      <label className="block text-sm text-slate-400 mb-2">
                        3. Enter the 6-digit code from your app
                      </label>
                      <input
                        type="text"
                        className="w-full max-w-xs px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-center text-xl tracking-widest font-mono focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500"
                        value={mfaCode}
                        onChange={(e) => setMfaCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
                        placeholder="000000"
                        maxLength={6}
                      />
                    </div>

                    <div className="flex gap-3">
                      <button
                        type="submit"
                        disabled={enableMfaMutation.isPending || mfaCode.length !== 6}
                        className="btn btn-primary flex items-center gap-2"
                      >
                        {enableMfaMutation.isPending && <Loader2 className="w-4 h-4 animate-spin" />}
                        Verify & Enable
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          setMfaSetup(null);
                          setMfaCode("");
                        }}
                        className="btn btn-secondary"
                      >
                        Cancel
                      </button>
                    </div>
                  </form>
                </div>
              )}
            </div>
          ) : (
            <div className="space-y-4">
              <p className="text-slate-400">
                Two-factor authentication is currently enabled on your account.
              </p>

              {!showDisableMfa ? (
                <button
                  onClick={() => setShowDisableMfa(true)}
                  className="btn btn-secondary text-red-400 border-red-500/30 hover:bg-red-500/10"
                >
                  Disable 2FA
                </button>
              ) : (
                <form onSubmit={handleDisableMFA} className="space-y-4 p-4 bg-red-500/10 border border-red-500/30 rounded-lg">
                  <p className="text-red-400 text-sm">
                    Warning: Disabling 2FA will make your account less secure.
                  </p>
                  <div>
                    <label className="block text-sm text-slate-400 mb-2">Password</label>
                    <input
                      type="password"
                      className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-red-500/50 focus:border-red-500"
                      value={disableMfaPassword}
                      onChange={(e) => setDisableMfaPassword(e.target.value)}
                    />
                  </div>
                  <div>
                    <label className="block text-sm text-slate-400 mb-2">
                      {disableWithBackup ? "Backup code (XXXX-XXXX)" : "2FA Code"}
                    </label>
                    <input
                      type="text"
                      key={disableWithBackup ? "backup" : "totp"}
                      inputMode={disableWithBackup ? "text" : "numeric"}
                      autoCapitalize={disableWithBackup ? "characters" : undefined}
                      spellCheck={false}
                      className="w-full max-w-xs px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-center font-mono focus:outline-none focus:ring-2 focus:ring-red-500/50 focus:border-red-500"
                      value={disableMfaCode}
                      onChange={(e) => setDisableMfaCode(normalizeMfaInput(e.target.value, disableWithBackup))}
                      placeholder={disableWithBackup ? "XXXX-XXXX" : "000000"}
                      maxLength={disableWithBackup ? 9 : 6}
                    />
                    <button
                      type="button"
                      onClick={() => {
                        setDisableWithBackup((b) => !b);
                        setDisableMfaCode("");
                      }}
                      className="block mt-2 text-xs text-cyan-400 hover:text-cyan-300"
                    >
                      {disableWithBackup ? "Use authenticator code instead" : "Use a backup code"}
                    </button>
                  </div>
                  <div className="flex gap-3">
                    <button
                      type="submit"
                      disabled={
                        disableMfaMutation.isPending ||
                        !disableMfaPassword ||
                        !isCompleteMfaCode(disableMfaCode, disableWithBackup)
                      }
                      className="btn bg-red-600 hover:bg-red-700 text-white flex items-center gap-2"
                    >
                      {disableMfaMutation.isPending && <Loader2 className="w-4 h-4 animate-spin" />}
                      Confirm Disable
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setShowDisableMfa(false);
                        setDisableMfaPassword("");
                        setDisableMfaCode("");
                        setDisableWithBackup(false);
                      }}
                      className="btn btn-secondary"
                    >
                      Cancel
                    </button>
                  </div>
                </form>
              )}
            </div>
          )}
        </div>

        {/* Active Sessions */}
        <div className="card">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-lg bg-emerald-500/20">
              <Monitor className="w-5 h-5 text-emerald-400" />
            </div>
            <h2 className="text-lg font-semibold text-white">Active Sessions</h2>
            <button
              type="button"
              onClick={() => {
                if (window.confirm("Sign out every session, including this one? You will need to sign in again.")) {
                  logoutAllMutation.mutate();
                }
              }}
              disabled={logoutAllMutation.isPending}
              className="ml-auto btn btn-secondary text-xs flex items-center gap-1.5 text-red-400 border-red-500/30 hover:bg-red-500/10"
            >
              {logoutAllMutation.isPending ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin" />
              ) : (
                <LogOut className="w-3.5 h-3.5" />
              )}
              Sign out all sessions
            </button>
          </div>

          <div className="space-y-3">
            {sessions && sessions.length > 0 ? (
              sessions.map((session: Session) => (
                <div
                  key={session.id}
                  className="p-3 bg-slate-800/50 rounded-lg flex items-center gap-3"
                >
                  <Monitor className="w-5 h-5 text-slate-400" />
                  <div className="flex-1 min-w-0">
                    <p className="text-white text-sm truncate">
                      {session.user_agent || "Unknown device"}
                    </p>
                    <p className="text-slate-500 text-xs">
                      {session.ip_address || "Unknown IP"} • {formatDate(session.created_at)}
                    </p>
                  </div>
                </div>
              ))
            ) : (
              <p className="text-slate-400 text-sm">No active sessions</p>
            )}
          </div>
        </div>

        {/* Audit Log */}
        <div className="card">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-lg bg-rose-500/20">
              <Clock className="w-5 h-5 text-rose-400" />
            </div>
            <h2 className="text-lg font-semibold text-white">Recent Activity</h2>
          </div>

          <div className="space-y-3">
            {auditLog && auditLog.length > 0 ? (
              auditLog.slice(0, 10).map((event: AuditEvent) => (
                <div
                  key={event.id}
                  className="p-3 bg-slate-800/50 rounded-lg flex items-center gap-3"
                >
                  {getEventIcon(event.event_type)}
                  <div className="flex-1 min-w-0">
                    <p className="text-white text-sm capitalize">
                      {event.event_type.replace(/_/g, " ")}
                    </p>
                    <p className="text-slate-500 text-xs">
                      {event.ip_address || "Unknown IP"} • {formatDate(event.created_at)}
                    </p>
                  </div>
                </div>
              ))
            ) : (
              <p className="text-slate-400 text-sm">No recent activity</p>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

export default function ProfilePage() {
  return (
    <RequireAuth>
      <ProfileContent />
    </RequireAuth>
  );
}
