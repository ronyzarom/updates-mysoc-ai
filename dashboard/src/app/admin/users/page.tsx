"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Users,
  Plus,
  Trash2,
  Edit2,
  Shield,
  ShieldCheck,
  ShieldOff,
  AlertCircle,
  Loader2,
  X,
} from "lucide-react";
import { api, User } from "@/lib/api";
import { RequireAuth, RequireRole } from "@/lib/auth-context";

function UsersContent() {
  const queryClient = useQueryClient();
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [editUser, setEditUser] = useState<User | null>(null);
  const [deleteUser, setDeleteUser] = useState<User | null>(null);

  // Form state
  const [formData, setFormData] = useState({
    email: "",
    password: "",
    name: "",
    role: "viewer",
  });

  const { data: users, isLoading } = useQuery({
    queryKey: ["users"],
    queryFn: () => api.getUsers(),
  });

  const createMutation = useMutation({
    mutationFn: (data: typeof formData) => api.createUser(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      setShowCreateModal(false);
      resetForm();
    },
  });

  const updateMutation = useMutation({
    mutationFn: (data: { id: string; name: string; role: string; is_active: boolean }) =>
      api.updateUser(data.id, { name: data.name, role: data.role, is_active: data.is_active }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      setEditUser(null);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteUser(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
      setDeleteUser(null);
    },
  });

  const resetForm = () => {
    setFormData({ email: "", password: "", name: "", role: "viewer" });
  };

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault();
    createMutation.mutate(formData);
  };

  const handleUpdate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!editUser) return;
    updateMutation.mutate({
      id: editUser.id,
      name: formData.name,
      role: formData.role,
      is_active: editUser.is_active,
    });
  };

  const getRoleIcon = (role: string) => {
    switch (role) {
      case "admin":
        return <ShieldCheck className="w-4 h-4 text-violet-400" />;
      case "operator":
        return <Shield className="w-4 h-4 text-cyan-400" />;
      default:
        return <ShieldOff className="w-4 h-4 text-slate-400" />;
    }
  };

  const getRoleBadge = (role: string) => {
    const colors: Record<string, string> = {
      admin: "bg-violet-500/20 text-violet-400",
      operator: "bg-cyan-500/20 text-cyan-400",
      viewer: "bg-slate-500/20 text-slate-400",
    };
    return colors[role] || colors.viewer;
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-cyan-500" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-white">User Management</h1>
          <p className="text-slate-400 mt-1">Manage dashboard users and permissions</p>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
          className="btn btn-primary flex items-center gap-2"
        >
          <Plus className="w-4 h-4" />
          Add User
        </button>
      </div>

      {/* Users Table */}
      <div className="card overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="border-b border-slate-800">
              <th className="text-left text-xs font-semibold text-slate-400 uppercase tracking-wider px-6 py-4">
                User
              </th>
              <th className="text-left text-xs font-semibold text-slate-400 uppercase tracking-wider px-6 py-4">
                Role
              </th>
              <th className="text-left text-xs font-semibold text-slate-400 uppercase tracking-wider px-6 py-4">
                Status
              </th>
              <th className="text-left text-xs font-semibold text-slate-400 uppercase tracking-wider px-6 py-4">
                MFA
              </th>
              <th className="text-left text-xs font-semibold text-slate-400 uppercase tracking-wider px-6 py-4">
                Last Login
              </th>
              <th className="text-right text-xs font-semibold text-slate-400 uppercase tracking-wider px-6 py-4">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {users?.map((user: User) => (
              <tr
                key={user.id}
                className="border-b border-slate-800/50 hover:bg-slate-800/30 transition-colors"
              >
                <td className="px-6 py-4">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-full bg-gradient-to-br from-violet-500 to-purple-600 flex items-center justify-center text-white font-bold">
                      {user.name.charAt(0).toUpperCase()}
                    </div>
                    <div>
                      <p className="text-white font-medium">{user.name}</p>
                      <p className="text-slate-500 text-sm">{user.email}</p>
                    </div>
                  </div>
                </td>
                <td className="px-6 py-4">
                  <span
                    className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium capitalize ${getRoleBadge(
                      user.role
                    )}`}
                  >
                    {getRoleIcon(user.role)}
                    {user.role}
                  </span>
                </td>
                <td className="px-6 py-4">
                  <span
                    className={`inline-block px-2.5 py-1 rounded-full text-xs font-medium ${
                      user.is_active
                        ? "bg-green-500/20 text-green-400"
                        : "bg-red-500/20 text-red-400"
                    }`}
                  >
                    {user.is_active ? "Active" : "Inactive"}
                  </span>
                </td>
                <td className="px-6 py-4">
                  <span
                    className={`inline-block px-2.5 py-1 rounded-full text-xs font-medium ${
                      user.mfa_enabled
                        ? "bg-violet-500/20 text-violet-400"
                        : "bg-slate-500/20 text-slate-400"
                    }`}
                  >
                    {user.mfa_enabled ? "Enabled" : "Disabled"}
                  </span>
                </td>
                <td className="px-6 py-4 text-slate-400 text-sm">
                  {user.last_login_at
                    ? new Date(user.last_login_at).toLocaleString()
                    : "Never"}
                </td>
                <td className="px-6 py-4">
                  <div className="flex items-center justify-end gap-2">
                    <button
                      onClick={() => {
                        setEditUser(user);
                        setFormData({
                          email: user.email,
                          password: "",
                          name: user.name,
                          role: user.role,
                        });
                      }}
                      className="p-2 text-slate-400 hover:text-white hover:bg-slate-800 rounded-lg transition-colors"
                    >
                      <Edit2 className="w-4 h-4" />
                    </button>
                    <button
                      onClick={() => setDeleteUser(user)}
                      className="p-2 text-slate-400 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Create Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 w-full max-w-md">
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-xl font-bold text-white flex items-center gap-2">
                <Users className="w-5 h-5 text-cyan-400" />
                Create User
              </h2>
              <button
                onClick={() => {
                  setShowCreateModal(false);
                  resetForm();
                }}
                className="p-2 text-slate-400 hover:text-white"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label className="block text-sm text-slate-400 mb-2">Email</label>
                <input
                  type="email"
                  required
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500"
                  value={formData.email}
                  onChange={(e) => setFormData({ ...formData, email: e.target.value })}
                />
              </div>

              <div>
                <label className="block text-sm text-slate-400 mb-2">Name</label>
                <input
                  type="text"
                  required
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                />
              </div>

              <div>
                <label className="block text-sm text-slate-400 mb-2">Password</label>
                <input
                  type="password"
                  required
                  minLength={8}
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500"
                  value={formData.password}
                  onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                />
                <p className="text-xs text-slate-500 mt-1">Minimum 8 characters</p>
              </div>

              <div>
                <label className="block text-sm text-slate-400 mb-2">Role</label>
                <select
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500"
                  value={formData.role}
                  onChange={(e) => setFormData({ ...formData, role: e.target.value })}
                >
                  <option value="viewer">Viewer</option>
                  <option value="operator">Operator</option>
                  <option value="admin">Admin</option>
                </select>
              </div>

              {createMutation.error && (
                <div className="flex items-center gap-2 p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 text-sm">
                  <AlertCircle className="w-4 h-4" />
                  {createMutation.error instanceof Error
                    ? createMutation.error.message
                    : "Failed to create user"}
                </div>
              )}

              <div className="flex gap-3 pt-2">
                <button
                  type="submit"
                  disabled={createMutation.isPending}
                  className="btn btn-primary flex-1 flex items-center justify-center gap-2"
                >
                  {createMutation.isPending && <Loader2 className="w-4 h-4 animate-spin" />}
                  Create User
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setShowCreateModal(false);
                    resetForm();
                  }}
                  className="btn btn-secondary"
                >
                  Cancel
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Modal */}
      {editUser && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 w-full max-w-md">
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-xl font-bold text-white flex items-center gap-2">
                <Edit2 className="w-5 h-5 text-cyan-400" />
                Edit User
              </h2>
              <button
                onClick={() => {
                  setEditUser(null);
                  resetForm();
                }}
                className="p-2 text-slate-400 hover:text-white"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleUpdate} className="space-y-4">
              <div>
                <label className="block text-sm text-slate-400 mb-2">Email</label>
                <input
                  type="email"
                  disabled
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-slate-400"
                  value={editUser.email}
                />
              </div>

              <div>
                <label className="block text-sm text-slate-400 mb-2">Name</label>
                <input
                  type="text"
                  required
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                />
              </div>

              <div>
                <label className="block text-sm text-slate-400 mb-2">Role</label>
                <select
                  className="w-full px-4 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500"
                  value={formData.role}
                  onChange={(e) => setFormData({ ...formData, role: e.target.value })}
                >
                  <option value="viewer">Viewer</option>
                  <option value="operator">Operator</option>
                  <option value="admin">Admin</option>
                </select>
              </div>

              {updateMutation.error && (
                <div className="flex items-center gap-2 p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 text-sm">
                  <AlertCircle className="w-4 h-4" />
                  {updateMutation.error instanceof Error
                    ? updateMutation.error.message
                    : "Failed to update user"}
                </div>
              )}

              <div className="flex gap-3 pt-2">
                <button
                  type="submit"
                  disabled={updateMutation.isPending}
                  className="btn btn-primary flex-1 flex items-center justify-center gap-2"
                >
                  {updateMutation.isPending && <Loader2 className="w-4 h-4 animate-spin" />}
                  Save Changes
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setEditUser(null);
                    resetForm();
                  }}
                  className="btn btn-secondary"
                >
                  Cancel
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Delete Confirmation */}
      {deleteUser && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 w-full max-w-md">
            <div className="flex items-center gap-3 mb-4">
              <div className="p-3 rounded-full bg-red-500/20">
                <AlertCircle className="w-6 h-6 text-red-400" />
              </div>
              <div>
                <h2 className="text-xl font-bold text-white">Delete User</h2>
                <p className="text-slate-400 text-sm">This action cannot be undone</p>
              </div>
            </div>

            <p className="text-slate-300 mb-6">
              Are you sure you want to delete <strong>{deleteUser.name}</strong> ({deleteUser.email})?
            </p>

            <div className="flex gap-3">
              <button
                onClick={() => deleteMutation.mutate(deleteUser.id)}
                disabled={deleteMutation.isPending}
                className="btn bg-red-600 hover:bg-red-700 text-white flex-1 flex items-center justify-center gap-2"
              >
                {deleteMutation.isPending && <Loader2 className="w-4 h-4 animate-spin" />}
                Delete User
              </button>
              <button
                onClick={() => setDeleteUser(null)}
                className="btn btn-secondary"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export default function UsersPage() {
  return (
    <RequireAuth>
      <RequireRole roles={["admin"]} fallback={
        <div className="flex flex-col items-center justify-center h-64 text-center">
          <Shield className="w-12 h-12 text-slate-600 mb-4" />
          <h2 className="text-xl font-bold text-white mb-2">Access Denied</h2>
          <p className="text-slate-400">You don't have permission to view this page.</p>
        </div>
      }>
        <UsersContent />
      </RequireRole>
    </RequireAuth>
  );
}
