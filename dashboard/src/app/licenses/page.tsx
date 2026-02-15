"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Key, Plus, RefreshCw, Calendar, CheckCircle, XCircle, X } from "lucide-react";
import { formatDistanceToNow, format } from "date-fns";
import { useState } from "react";

export default function LicensesPage() {
  const queryClient = useQueryClient();
  const { data: licenses, isLoading, refetch } = useQuery({
    queryKey: ["licenses"],
    queryFn: () => api.getLicenses(),
  });

  const [showCreateModal, setShowCreateModal] = useState(false);

  const createMutation = useMutation({
    mutationFn: (data: {
      customer_id: string;
      customer_name: string;
      type: string;
      products: string[];
      expires_at: string;
    }) => api.createLicense(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["licenses"] });
      setShowCreateModal(false);
    },
  });

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-white">Licenses</h1>
          <p className="text-slate-400 mt-1">
            Manage customer licenses
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button onClick={() => refetch()} className="btn btn-secondary">
            <RefreshCw className="w-4 h-4" />
            Refresh
          </button>
          <button
            onClick={() => setShowCreateModal(true)}
            className="btn btn-primary"
          >
            <Plus className="w-4 h-4" />
            Create License
          </button>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <div className="card">
          <div className="flex items-center gap-3">
            <div className="p-3 rounded-lg bg-emerald-500/20">
              <CheckCircle className="w-6 h-6 text-emerald-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-white">
                {licenses?.filter((l) => l.is_active).length || 0}
              </p>
              <p className="text-sm text-slate-400">Active Licenses</p>
            </div>
          </div>
        </div>
        <div className="card">
          <div className="flex items-center gap-3">
            <div className="p-3 rounded-lg bg-amber-500/20">
              <Calendar className="w-6 h-6 text-amber-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-white">
                {licenses?.filter(
                  (l) =>
                    new Date(l.expires_at) <
                    new Date(Date.now() + 30 * 24 * 60 * 60 * 1000)
                ).length || 0}
              </p>
              <p className="text-sm text-slate-400">Expiring Soon</p>
            </div>
          </div>
        </div>
        <div className="card">
          <div className="flex items-center gap-3">
            <div className="p-3 rounded-lg bg-red-500/20">
              <XCircle className="w-6 h-6 text-red-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-white">
                {licenses?.filter(
                  (l) => !l.is_active || new Date(l.expires_at) < new Date()
                ).length || 0}
              </p>
              <p className="text-sm text-slate-400">Inactive/Expired</p>
            </div>
          </div>
        </div>
      </div>

      {/* Licenses Table */}
      {isLoading ? (
        <div className="text-slate-400">Loading licenses...</div>
      ) : (
        <div className="card">
          <div className="table-container">
            <table className="table">
              <thead>
                <tr>
                  <th>Customer</th>
                  <th>License Key</th>
                  <th>Type</th>
                  <th>Products</th>
                  <th>Expires</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {licenses?.map((license) => {
                  const isExpired = new Date(license.expires_at) < new Date();
                  const isExpiringSoon =
                    new Date(license.expires_at) <
                    new Date(Date.now() + 30 * 24 * 60 * 60 * 1000);

                  return (
                    <tr key={license.id}>
                      <td>
                        <div>
                          <p className="font-medium text-white">
                            {license.customer_name}
                          </p>
                          <p className="text-xs text-slate-500">
                            {license.customer_id}
                          </p>
                        </div>
                      </td>
                      <td>
                        <code className="text-cyan-400 font-mono text-sm">
                          {license.license_key}
                        </code>
                      </td>
                      <td>
                        <span className="px-2 py-1 rounded bg-slate-700 text-xs capitalize">
                          {license.type}
                        </span>
                      </td>
                      <td>
                        <div className="flex flex-wrap gap-1">
                          {license.products?.slice(0, 2).map((product) => (
                            <span
                              key={product}
                              className="px-2 py-0.5 rounded bg-slate-700 text-xs"
                            >
                              {product}
                            </span>
                          ))}
                          {license.products?.length > 2 && (
                            <span className="text-xs text-slate-500">
                              +{license.products.length - 2}
                            </span>
                          )}
                        </div>
                      </td>
                      <td>
                        <div
                          className={`${
                            isExpired
                              ? "text-red-400"
                              : isExpiringSoon
                              ? "text-amber-400"
                              : "text-slate-300"
                          }`}
                        >
                          <p className="text-sm">
                            {format(new Date(license.expires_at), "MMM d, yyyy")}
                          </p>
                          <p className="text-xs">
                            {formatDistanceToNow(new Date(license.expires_at), {
                              addSuffix: true,
                            })}
                          </p>
                        </div>
                      </td>
                      <td>
                        {license.is_active && !isExpired ? (
                          <span className="status-badge status-online">
                            Active
                          </span>
                        ) : isExpired ? (
                          <span className="status-badge status-offline">
                            Expired
                          </span>
                        ) : (
                          <span className="status-badge bg-slate-500/20 text-slate-400">
                            Inactive
                          </span>
                        )}
                      </td>
                    </tr>
                  );
                })}

                {(!licenses || licenses.length === 0) && (
                  <tr>
                    <td colSpan={6} className="text-center py-16">
                      <Key className="w-12 h-12 text-slate-600 mx-auto mb-4" />
                      <h3 className="text-lg font-medium text-white mb-2">
                        No licenses yet
                      </h3>
                      <p className="text-slate-400 mb-6">
                        Create your first license to get started.
                      </p>
                      <button
                        onClick={() => setShowCreateModal(true)}
                        className="btn btn-primary"
                      >
                        <Plus className="w-4 h-4" />
                        Create License
                      </button>
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Create License Modal */}
      {showCreateModal && (
        <CreateLicenseModal
          onClose={() => setShowCreateModal(false)}
          onSubmit={(data) => createMutation.mutate(data)}
          isLoading={createMutation.isPending}
          error={createMutation.error?.message}
        />
      )}
    </div>
  );
}

function CreateLicenseModal({
  onClose,
  onSubmit,
  isLoading,
  error,
}: {
  onClose: () => void;
  onSubmit: (data: {
    customer_id: string;
    customer_name: string;
    type: string;
    products: string[];
    expires_at: string;
  }) => void;
  isLoading: boolean;
  error?: string;
}) {
  const [formData, setFormData] = useState({
    customer_id: "",
    customer_name: "",
    type: "siemcore",
    products: ["siemcore-api", "siemcore-collector", "siemcore-frontend"],
    expires_at: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString().split("T")[0],
  });

  const productOptions = [
    "siemcore-api",
    "siemcore-collector",
    "siemcore-frontend",
    "mysoc-api",
    "mysoc-frontend",
  ];

  const handleProductToggle = (product: string) => {
    setFormData((prev) => ({
      ...prev,
      products: prev.products.includes(product)
        ? prev.products.filter((p) => p !== product)
        : [...prev.products, product],
    }));
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSubmit({
      ...formData,
      expires_at: new Date(formData.expires_at).toISOString(),
    });
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-slate-800 rounded-xl p-6 w-full max-w-md border border-slate-700">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-xl font-semibold text-white">Create License</h2>
          <button
            onClick={onClose}
            className="p-1 hover:bg-slate-700 rounded transition-colors"
          >
            <X className="w-5 h-5 text-slate-400" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-slate-300 mb-1">
              Customer ID
            </label>
            <input
              type="text"
              value={formData.customer_id}
              onChange={(e) =>
                setFormData((prev) => ({ ...prev, customer_id: e.target.value }))
              }
              className="input w-full"
              placeholder="e.g., acme-corp"
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-slate-300 mb-1">
              Customer Name
            </label>
            <input
              type="text"
              value={formData.customer_name}
              onChange={(e) =>
                setFormData((prev) => ({ ...prev, customer_name: e.target.value }))
              }
              className="input w-full"
              placeholder="e.g., Acme Corporation"
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-slate-300 mb-1">
              License Type
            </label>
            <select
              value={formData.type}
              onChange={(e) =>
                setFormData((prev) => ({ ...prev, type: e.target.value }))
              }
              className="input w-full"
            >
              <option value="siemcore">SIEMCore</option>
              <option value="mysoc">MySoc</option>
              <option value="enterprise">Enterprise</option>
              <option value="trial">Trial</option>
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium text-slate-300 mb-1">
              Products
            </label>
            <div className="flex flex-wrap gap-2">
              {productOptions.map((product) => (
                <button
                  key={product}
                  type="button"
                  onClick={() => handleProductToggle(product)}
                  className={`px-3 py-1 rounded text-sm transition-colors ${
                    formData.products.includes(product)
                      ? "bg-cyan-500/20 text-cyan-400 border border-cyan-500/50"
                      : "bg-slate-700 text-slate-400 border border-slate-600"
                  }`}
                >
                  {product}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-slate-300 mb-1">
              Expires At
            </label>
            <input
              type="date"
              value={formData.expires_at}
              onChange={(e) =>
                setFormData((prev) => ({ ...prev, expires_at: e.target.value }))
              }
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
            <button
              type="button"
              onClick={onClose}
              className="btn btn-secondary flex-1"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isLoading}
              className="btn btn-primary flex-1"
            >
              {isLoading ? "Creating..." : "Create License"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

