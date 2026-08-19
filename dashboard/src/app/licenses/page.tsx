"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, License } from "@/lib/api";
import { Key, Plus, RefreshCw, Calendar, CheckCircle, XCircle } from "lucide-react";
import { formatDistanceToNow, format } from "date-fns";
import Link from "next/link";
import { useState } from "react";
import { LoadingState, ErrorState, Modal } from "@/components/ui";
import { RequireRole } from "@/lib/auth-context";
import { hasExpiry, licenseCounts, THIRTY_DAYS_MS } from "@/lib/derive";

export default function LicensesPage() {
  const queryClient = useQueryClient();
  const { data: licenses, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["licenses"],
    queryFn: () => api.getLicenses(),
    retry: false,
  });

  const [showCreateModal, setShowCreateModal] = useState(false);

  const now = Date.now();
  const {
    active: activeCount,
    expiringSoon: expiringSoonCount,
    inactiveOrExpired: inactiveExpiredCount,
  } = licenseCounts(licenses, now);

  const createMutation = useMutation({
    mutationFn: (data: {
      customer_id: string;
      customer_name: string;
      type: string;
      operator_id: string;
      reseller_id: string;
      reseller_name: string;
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
          <h1 className="text-3xl font-bold text-white">Legacy Licenses</h1>
          <p className="text-slate-400 mt-1">
            Pre-1.8.0 per-customer keys. New operators get a single platform
            key on the{" "}
            <Link href="/operators" className="text-cyan-400 hover:underline">
              Operators
            </Link>{" "}
            page; their fleet updates through the cascade instead.
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
              Create License
            </button>
          </RequireRole>
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
              <p className="text-2xl font-bold text-white">{activeCount}</p>
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
              <p className="text-2xl font-bold text-white">{expiringSoonCount}</p>
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
                {inactiveExpiredCount}
              </p>
              <p className="text-sm text-slate-400">Inactive/Expired</p>
            </div>
          </div>
        </div>
      </div>

      {/* Licenses Table */}
      {isLoading ? (
        <LoadingState label="Loading licenses..." />
      ) : isError ? (
        <ErrorState
          title="Failed to load licenses"
          error={error}
          onRetry={() => refetch()}
        />
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
                  const licenseHasExpiry = hasExpiry(license.expires_at);
                  const expMs = licenseHasExpiry
                    ? new Date(license.expires_at).getTime()
                    : 0;
                  const isExpired = licenseHasExpiry && expMs <= now;
                  const isExpiringSoon =
                    licenseHasExpiry &&
                    expMs > now &&
                    expMs < now + THIRTY_DAYS_MS;

                  return (
                    <tr key={license.id}>
                      <td>
                        <Link href={`/licenses/${license.id}`} className="block group">
                          <p className="font-medium text-white group-hover:text-cyan-400">
                            {license.customer_name}
                          </p>
                          <p className="text-xs text-slate-500">
                            {license.customer_id}
                          </p>
                        </Link>
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
                          {licenseHasExpiry ? (
                            <>
                              <p className="text-sm">
                                {format(new Date(license.expires_at), "MMM d, yyyy")}
                              </p>
                              <p className="text-xs">
                                {formatDistanceToNow(new Date(license.expires_at), {
                                  addSuffix: true,
                                })}
                              </p>
                            </>
                          ) : (
                            <p className="text-sm text-slate-400">Never</p>
                          )}
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
                      <RequireRole roles={["admin"]}>
                        <button
                          onClick={() => setShowCreateModal(true)}
                          className="btn btn-primary"
                        >
                          <Plus className="w-4 h-4" />
                          Create License
                        </button>
                      </RequireRole>
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
          licenses={licenses || []}
          onClose={() => setShowCreateModal(false)}
          onSubmit={(data) => createMutation.mutate(data)}
          isLoading={createMutation.isPending}
          error={createMutation.error?.message}
        />
      )}
    </div>
  );
}

// A platform license marks a SOC operator (as opposed to a customer license).
function isPlatformLicense(l: License): boolean {
  return l.type === "mysoc-cloud" || (!!l.operator_id && l.operator_id === l.customer_id);
}

const CUSTOMER_PRODUCTS = ["siemcore-api", "siemcore-collector", "siemcore-frontend", "swf"];
const PLATFORM_PRODUCTS = ["mysoc-api", "mysoc-frontend"];

function CreateLicenseModal({
  licenses,
  onClose,
  onSubmit,
  isLoading,
  error,
}: {
  licenses: License[];
  onClose: () => void;
  onSubmit: (data: {
    customer_id: string;
    customer_name: string;
    type: string;
    operator_id: string;
    reseller_id: string;
    reseller_name: string;
    products: string[];
    expires_at: string;
  }) => void;
  isLoading: boolean;
  error?: string;
}) {
  // Known SOC operators, derived from existing platform licenses.
  const operatorOptions = licenses
    .filter(isPlatformLicense)
    .map((l) => ({
      id: l.operator_id || l.customer_id,
      name: l.customer_name || l.customer_id,
    }))
    .filter((op, i, all) => all.findIndex((o) => o.id === op.id) === i);

  const [scope, setScope] = useState<"customer" | "operator">("customer");
  const [formData, setFormData] = useState({
    customer_id: "",
    customer_name: "",
    type: "siemcore",
    operator_id: operatorOptions[0]?.id || "",
    reseller_id: "",
    reseller_name: "",
    products: ["siemcore-api", "siemcore-collector", "siemcore-frontend", "swf"],
    expires_at: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString().split("T")[0],
  });

  const productOptions = scope === "customer" ? CUSTOMER_PRODUCTS : PLATFORM_PRODUCTS;

  const handleScopeChange = (next: "customer" | "operator") => {
    setScope(next);
    setFormData((prev) => ({
      ...prev,
      type: next === "customer" ? "siemcore" : "mysoc-cloud",
      products: next === "customer" ? [...CUSTOMER_PRODUCTS] : [...PLATFORM_PRODUCTS],
    }));
  };

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
      // An operator's platform license belongs to itself; the server also
      // enforces this default.
      operator_id: scope === "operator" ? formData.customer_id : formData.operator_id,
      reseller_id: scope === "operator" ? "" : formData.reseller_id,
      reseller_name: scope === "operator" ? "" : formData.reseller_name,
      expires_at: new Date(formData.expires_at).toISOString(),
    });
  };

  return (
    <Modal title="Create License" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <span className="block text-sm font-medium text-slate-300 mb-1">
            License Scope
          </span>
          <div className="grid grid-cols-2 gap-2">
            <button
              type="button"
              aria-pressed={scope === "customer"}
              onClick={() => handleScopeChange("customer")}
              className={`px-3 py-2 rounded text-sm text-left transition-colors ${
                scope === "customer"
                  ? "bg-cyan-500/20 text-cyan-300 border border-cyan-500/50"
                  : "bg-slate-700 text-slate-400 border border-slate-600"
              }`}
            >
              <span className="font-medium block">Customer</span>
              <span className="text-xs opacity-80">siemcore + swf</span>
            </button>
            <button
              type="button"
              aria-pressed={scope === "operator"}
              onClick={() => handleScopeChange("operator")}
              className={`px-3 py-2 rounded text-sm text-left transition-colors ${
                scope === "operator"
                  ? "bg-violet-500/20 text-violet-300 border border-violet-500/50"
                  : "bg-slate-700 text-slate-400 border border-slate-600"
              }`}
            >
              <span className="font-medium block">SOC Operator</span>
              <span className="text-xs opacity-80">mysoc platform</span>
            </button>
          </div>
        </div>

        <div>
          <label htmlFor="lic-customer-id" className="block text-sm font-medium text-slate-300 mb-1">
            {scope === "customer" ? "Customer ID" : "Operator ID"}
          </label>
          <input
            id="lic-customer-id"
            type="text"
            value={formData.customer_id}
            onChange={(e) =>
              setFormData((prev) => ({ ...prev, customer_id: e.target.value }))
            }
            className="input w-full"
            placeholder={scope === "customer" ? "e.g., acme-corp" : "e.g., cyfox-soc"}
            required
          />
        </div>

        <div>
          <label htmlFor="lic-customer-name" className="block text-sm font-medium text-slate-300 mb-1">
            {scope === "customer" ? "Customer Name" : "Operator Name"}
          </label>
          <input
            id="lic-customer-name"
            type="text"
            value={formData.customer_name}
            onChange={(e) =>
              setFormData((prev) => ({ ...prev, customer_name: e.target.value }))
            }
            className="input w-full"
            placeholder={scope === "customer" ? "e.g., Acme Corporation" : "e.g., Cyfox SOC"}
            required
          />
        </div>

        {scope === "customer" && (
          <>
            <div>
              <label htmlFor="lic-operator" className="block text-sm font-medium text-slate-300 mb-1">
                SOC Operator
              </label>
              {operatorOptions.length > 0 ? (
                <select
                  id="lic-operator"
                  value={formData.operator_id}
                  onChange={(e) =>
                    setFormData((prev) => ({ ...prev, operator_id: e.target.value }))
                  }
                  className="input w-full"
                >
                  {operatorOptions.map((op) => (
                    <option key={op.id} value={op.id}>
                      {op.name} ({op.id})
                    </option>
                  ))}
                  <option value="">Unassigned (set later)</option>
                </select>
              ) : (
                <input
                  id="lic-operator"
                  type="text"
                  value={formData.operator_id}
                  onChange={(e) =>
                    setFormData((prev) => ({ ...prev, operator_id: e.target.value }))
                  }
                  className="input w-full"
                  placeholder="Operator ID (no operator licenses yet)"
                />
              )}
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <label htmlFor="lic-reseller-id" className="block text-sm font-medium text-slate-300 mb-1">
                  Reseller ID <span className="text-slate-500">(optional)</span>
                </label>
                <input
                  id="lic-reseller-id"
                  type="text"
                  value={formData.reseller_id}
                  onChange={(e) =>
                    setFormData((prev) => ({ ...prev, reseller_id: e.target.value }))
                  }
                  className="input w-full"
                  placeholder="direct sale if empty"
                />
              </div>
              <div>
                <label htmlFor="lic-reseller-name" className="block text-sm font-medium text-slate-300 mb-1">
                  Reseller Name
                </label>
                <input
                  id="lic-reseller-name"
                  type="text"
                  value={formData.reseller_name}
                  onChange={(e) =>
                    setFormData((prev) => ({ ...prev, reseller_name: e.target.value }))
                  }
                  className="input w-full"
                  placeholder="e.g., Acme Channel Ltd"
                />
              </div>
            </div>

            <div>
              <label htmlFor="lic-type" className="block text-sm font-medium text-slate-300 mb-1">
                License Type
              </label>
              <select
                id="lic-type"
                value={formData.type}
                onChange={(e) =>
                  setFormData((prev) => ({ ...prev, type: e.target.value }))
                }
                className="input w-full"
              >
                <option value="siemcore">siemcore</option>
                <option value="siemcore-lite">siemcore-lite</option>
              </select>
            </div>
          </>
        )}

        <div>
          <span className="block text-sm font-medium text-slate-300 mb-1">
            Products
          </span>
          <div className="flex flex-wrap gap-2">
            {productOptions.map((product) => (
              <button
                key={product}
                type="button"
                aria-pressed={formData.products.includes(product)}
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
          <label htmlFor="lic-expires" className="block text-sm font-medium text-slate-300 mb-1">
            Expires At
          </label>
          <input
            id="lic-expires"
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
    </Modal>
  );
}

