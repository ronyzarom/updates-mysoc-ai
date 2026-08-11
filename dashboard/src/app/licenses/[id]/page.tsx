"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { format, formatDistanceToNow } from "date-fns";
import {
  ArrowLeft,
  Key,
  CheckCircle,
  XCircle,
  Calendar,
  Package,
  Building2,
} from "lucide-react";
import { LoadingState, ErrorState, EmptyState } from "@/components/ui";

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
