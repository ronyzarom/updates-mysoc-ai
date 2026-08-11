"use client";

import { useEffect, useId, useRef } from "react";
import { AlertTriangle, Loader2, RefreshCw, X } from "lucide-react";

// LoadingState renders a consistent inline loading indicator.
export function LoadingState({ label = "Loading..." }: { label?: string }) {
  return (
    <div
      className="flex items-center justify-center gap-3 py-12 text-slate-400"
      role="status"
      aria-live="polite"
    >
      <Loader2 className="w-5 h-5 animate-spin" />
      <span>{label}</span>
    </div>
  );
}

// ErrorState renders a retryable error panel.
export function ErrorState({
  title = "Something went wrong",
  error,
  onRetry,
}: {
  title?: string;
  error?: unknown;
  onRetry?: () => void;
}) {
  const message =
    error instanceof Error
      ? error.message
      : typeof error === "string"
        ? error
        : "An unexpected error occurred.";
  return (
    <div
      className="card border-red-500/50 bg-red-900/20"
      role="alert"
    >
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-start gap-3">
          <AlertTriangle className="w-5 h-5 text-red-400 mt-0.5 shrink-0" />
          <div>
            <p className="text-red-400 font-medium">{title}</p>
            <p className="text-sm text-slate-400 mt-1 break-words">{message}</p>
          </div>
        </div>
        {onRetry && (
          <button onClick={onRetry} className="btn btn-secondary shrink-0">
            <RefreshCw className="w-4 h-4" />
            Retry
          </button>
        )}
      </div>
    </div>
  );
}

// EmptyState renders a consistent empty placeholder.
export function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon?: React.ReactNode;
  title: string;
  description?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="text-center py-16">
      {icon && <div className="mx-auto mb-4 w-fit text-slate-600">{icon}</div>}
      <h3 className="text-lg font-medium text-white mb-2">{title}</h3>
      {description && <p className="text-slate-400 mb-6">{description}</p>}
      {action}
    </div>
  );
}

// Modal renders an accessible dialog with Escape-to-close, backdrop dismissal,
// initial focus, and proper ARIA semantics.
export function Modal({
  title,
  onClose,
  children,
  maxWidth = "max-w-md",
}: {
  title?: string;
  onClose: () => void;
  children: React.ReactNode;
  maxWidth?: string;
}) {
  const panelRef = useRef<HTMLDivElement>(null);
  const titleId = useId();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  useEffect(() => {
    panelRef.current?.focus();
  }, []);

  return (
    <div
      className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={title ? titleId : undefined}
        tabIndex={-1}
        className={`bg-slate-900 border border-slate-700 rounded-xl p-6 w-full ${maxWidth} shadow-2xl focus:outline-none`}
      >
        {title && (
          <div className="flex items-center justify-between mb-6">
            <h2 id={titleId} className="text-xl font-semibold text-white">
              {title}
            </h2>
            <button
              type="button"
              onClick={onClose}
              aria-label="Close dialog"
              className="p-1 rounded-lg hover:bg-slate-800 text-slate-400 hover:text-white transition-colors"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
        )}
        {children}
      </div>
    </div>
  );
}

// Switch is an accessible toggle with role="switch" and keyboard support.
export function Switch({
  checked,
  onChange,
  disabled,
  label,
}: {
  checked: boolean;
  onChange: (next: boolean) => void;
  disabled?: boolean;
  label: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-cyan-500/50 disabled:opacity-50 disabled:cursor-not-allowed ${
        checked ? "bg-cyan-500" : "bg-slate-600"
      }`}
    >
      <span
        className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
          checked ? "translate-x-6" : "translate-x-1"
        }`}
      />
    </button>
  );
}
