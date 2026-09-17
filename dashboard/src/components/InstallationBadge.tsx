import React from "react";
import { LockKeyhole } from "lucide-react";
import type { InstallationIdentity } from "@/lib/api";

export function InstallationBadge({ installation }: { installation?: InstallationIdentity }) {
  const label = installation?.kind === "normal" ? "Normal" : installation?.kind === "pod" ? "POD" : undefined;
  if (!label) return <span className="text-slate-400">Not reported</span>;
  return (
    <span className="inline-flex items-center gap-1.5 rounded border border-slate-600 px-2 py-1 text-white"
      aria-label={`Installation type: ${label} (read-only)`}
      title="Read-only installation identity. Active/standby changes do not change Normal/POD.">
      <LockKeyhole className="h-3 w-3" aria-hidden="true" />
      {label}
      <span className="text-xs text-slate-400">Read-only</span>
    </span>
  );
}
