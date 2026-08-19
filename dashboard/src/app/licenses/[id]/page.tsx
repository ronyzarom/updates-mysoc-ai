"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, License } from "@/lib/api";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { useState } from "react";
import { format, formatDistanceToNow } from "date-fns";
import {
  ArrowLeft,
  Key,
  CheckCircle,
  XCircle,
  Calendar,
  Package,
  Building2,
  Network,
} from "lucide-react";
import { LoadingState, ErrorState, EmptyState } from "@/components/ui";
import { RequireRole } from "@/lib/auth-context";

export default function LicenseDetailPage() {
  const params = useParams();
  const router = useRouter();
  const id = params.id as string;

  const {
    data: license,
    isLoading,
    isError,
    error,
    refetch,
  } = useQuery({
    queryKey: ["license", id],
    queryFn: () => api.getLicense(id),
    retry: false,
  });

  const hasExpiry =
    !!license?.expires_at && !license.expires_at.startsWith("0001-01-01");
  const isExpired = hasExpiry && new Date(license!.expires_at) < new Date();

  return (
    <div className="space-y-8">
      <div className="flex items-center gap-4">
        <button
          onClick={() => router.back()}
          aria-label="Go back"
          className="p-2 rounded-lg hover:bg-slate-800 transition-colors"
        >
          <ArrowLeft className="w-5 h-5 text-slate-400" />
        </button>
        <div>
          <h1 className="text-3xl font-bold text-white">License Details</h1>
          <p className="text-slate-400 mt-1">
            <Link href="/licenses" className="hover:text-cyan-400">
              Licenses
            </Link>{" "}
            / {id}
          </p>
        </div>
      </div>

      {isLoading ? (
        <LoadingState label="Loading license..." />
      ) : isError ? (
        <ErrorState
          title="Failed to load license"
          error={error}
          onRetry={() => refetch()}
        />
      ) : !license ? (
        <EmptyState
          icon={<Key className="w-12 h-12" />}
          title="License not found"
          description="This license may have been deleted."
          action={
            <Link href="/licenses" className="btn btn-secondary">
              <ArrowLeft className="w-4 h-4" />
              Back to Licenses
            </Link>
          }
        />
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="card lg:col-span-2">
            <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
              <Building2 className="w-5 h-5 text-cyan-400" />
              Customer
            </h2>
            <div className="grid grid-cols-2 gap-4">
              <Detail label="Customer Name" value={license.customer_name} />
              <Detail label="Customer ID" value={license.customer_id} />
              <Detail label="License Type" value={license.type} />
              <Detail
                label="Bound To"
                value={license.bound_to || "Not bound"}
              />
            </div>

            <div className="mt-6">
              <p className="text-sm text-slate-400 mb-1">License Key</p>
              <code className="block px-3 py-2 rounded bg-slate-800 text-cyan-400 font-mono text-sm break-all">
                {license.license_key}
              </code>
            </div>
          </div>

          <div className="card">
            <h2 className="text-lg font-semibold text-white mb-4">Status</h2>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <span className="text-slate-400 text-sm">Active</span>
                {license.is_active && !isExpired ? (
                  <span className="inline-flex items-center gap-1.5 text-emerald-400 text-sm">
                    <CheckCircle className="w-4 h-4" /> Active
                  </span>
                ) : isExpired ? (
                  <span className="inline-flex items-center gap-1.5 text-red-400 text-sm">
                    <XCircle className="w-4 h-4" /> Expired
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1.5 text-slate-400 text-sm">
                    <XCircle className="w-4 h-4" /> Inactive
                  </span>
                )}
              </div>

              <div className="flex items-center justify-between">
                <span className="text-slate-400 text-sm flex items-center gap-1">
                  <Calendar className="w-4 h-4" /> Expires
                </span>
                <span
                  className={`text-sm ${
                    isExpired ? "text-red-400" : "text-white"
                  }`}
                >
                  {hasExpiry
                    ? `${format(new Date(license.expires_at), "MMM d, yyyy")}`
                    : "Never"}
                </span>
              </div>

              {hasExpiry && (
                <p className="text-xs text-slate-500 text-right">
                  {formatDistanceToNow(new Date(license.expires_at), {
                    addSuffix: true,
                  })}
                </p>
              )}

              {license.issued_at &&
                !license.issued_at.startsWith("0001-01-01") && (
                  <div className="flex items-center justify-between">
                    <span className="text-slate-400 text-sm">Issued</span>
                    <span className="text-white text-sm">
                      {format(new Date(license.issued_at), "MMM d, yyyy")}
                    </span>
                  </div>
                )}
            </div>
          </div>

          <OwnershipCard license={license} />

          <div className="card lg:col-span-3">
            <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
              <Package className="w-5 h-5 text-cyan-400" />
              Products & Features
            </h2>
            <div className="space-y-4">
              <div>
                <p className="text-sm text-slate-400 mb-2">Products</p>
                <div className="flex flex-wrap gap-2">
                  {license.products && license.products.length > 0 ? (
                    license.products.map((product) => (
                      <span
                        key={product}
                        className="px-2 py-1 rounded bg-slate-700 text-xs text-slate-200"
                      >
                        {product}
                      </span>
                    ))
                  ) : (
                    <span className="text-slate-500 text-sm">None</span>
                  )}
                </div>
              </div>
              {license.features && license.features.length > 0 && (
                <div>
                  <p className="text-sm text-slate-400 mb-2">Features</p>
                  <div className="flex flex-wrap gap-2">
                    {license.features.map((feature) => (
                      <span
                        key={feature}
                        className="px-2 py-1 rounded bg-cyan-500/10 text-xs text-cyan-300"
                      >
                        {feature}
                      </span>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-sm text-slate-400">{label}</p>
      <p className="text-white font-medium break-words">{value}</p>
    </div>
  );
}

// Ownership: which SOC operator this license belongs to and (for customer
// licenses) which reseller sold it. Admins can edit; legacy licenses start
// unassigned.
function OwnershipCard({ license }: { license: License }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({
    operator_id: license.operator_id || "",
    reseller_id: license.reseller_id || "",
    reseller_name: license.reseller_name || "",
  });

  const saveMutation = useMutation({
    mutationFn: () => api.updateLicense(license.id, form),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["license", license.id] });
      queryClient.invalidateQueries({ queryKey: ["licenses"] });
      queryClient.invalidateQueries({ queryKey: ["instance-tree"] });
      setEditing(false);
    },
  });

  return (
    <div className="card lg:col-span-3">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-white flex items-center gap-2">
          <Network className="w-5 h-5 text-violet-400" />
          Ownership
        </h2>
        <RequireRole roles={["admin"]}>
          {!editing && (
            <button onClick={() => setEditing(true)} className="btn btn-secondary">
              Edit
            </button>
          )}
        </RequireRole>
      </div>

      {!editing ? (
        <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
          <Detail label="SOC Operator" value={license.operator_id || "Unassigned"} />
          <Detail
            label="Sold Via"
            value={
              license.reseller_id
                ? `${license.reseller_name || license.reseller_id} (${license.reseller_id})`
                : "Direct"
            }
          />
        </div>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            saveMutation.mutate();
          }}
          className="space-y-4"
        >
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div>
              <label htmlFor="own-operator" className="block text-sm font-medium text-slate-300 mb-1">
                Operator ID
              </label>
              <input
                id="own-operator"
                type="text"
                value={form.operator_id}
                onChange={(e) => setForm((p) => ({ ...p, operator_id: e.target.value }))}
                className="input w-full"
                placeholder="e.g., cyfox-soc"
              />
            </div>
            <div>
              <label htmlFor="own-reseller-id" className="block text-sm font-medium text-slate-300 mb-1">
                Reseller ID <span className="text-slate-500">(optional)</span>
              </label>
              <input
                id="own-reseller-id"
                type="text"
                value={form.reseller_id}
                onChange={(e) => setForm((p) => ({ ...p, reseller_id: e.target.value }))}
                className="input w-full"
                placeholder="direct sale if empty"
              />
            </div>
            <div>
              <label htmlFor="own-reseller-name" className="block text-sm font-medium text-slate-300 mb-1">
                Reseller Name
              </label>
              <input
                id="own-reseller-name"
                type="text"
                value={form.reseller_name}
                onChange={(e) => setForm((p) => ({ ...p, reseller_name: e.target.value }))}
                className="input w-full"
                placeholder="e.g., Acme Channel Ltd"
              />
            </div>
          </div>

          {saveMutation.isError && (
            <div className="p-3 rounded bg-red-500/20 border border-red-500/50 text-red-400 text-sm">
              {saveMutation.error instanceof Error
                ? saveMutation.error.message
                : "Failed to save ownership"}
            </div>
          )}

          <div className="flex gap-3">
            <button
              type="button"
              onClick={() => setEditing(false)}
              className="btn btn-secondary"
            >
              Cancel
            </button>
            <button type="submit" disabled={saveMutation.isPending} className="btn btn-primary">
              {saveMutation.isPending ? "Saving..." : "Save"}
            </button>
          </div>
        </form>
      )}
    </div>
  );
}
