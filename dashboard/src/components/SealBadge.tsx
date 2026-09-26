import { BadgeCheck, CircleDashed, ShieldX, KeyRound } from "lucide-react";
import type { Release, SealStatus } from "@/lib/api";
import { sealStatusOf } from "@/lib/derive";

export const SEAL_LABELS: Record<SealStatus, string> = {
  sealed: "Sealed",
  unsealed: "Unsealed",
  invalid: "Invalid",
  unknown_key: "Unknown key",
};

const STYLES: Record<SealStatus, string> = {
  sealed: "bg-emerald-500/20 text-emerald-400",
  unsealed: "bg-slate-700 text-slate-300",
  invalid: "bg-red-500/20 text-red-400",
  unknown_key: "bg-amber-500/20 text-amber-400",
};

const ICONS = {
  sealed: BadgeCheck,
  unsealed: CircleDashed,
  invalid: ShieldX,
  unknown_key: KeyRound,
} as const;

function describe(status: SealStatus, issuer?: string, keyId?: string): string {
  const who = issuer ? ` by ${issuer}` : "";
  const key = keyId ? ` (key ${keyId})` : "";
  switch (status) {
    case "sealed":
      return `Sealed${who}${key}: the issuer's seal verified against a trusted key.`;
    case "invalid":
      return `The uploaded seal${key} did not verify. Accepted and server-signed during the transition; investigate this upload.`;
    case "unknown_key":
      return `Sealed with a key that is not an active trusted key${key}. Accepted and server-signed during the transition.`;
    default:
      return "Uploaded without an issuer seal; signed by the server.";
  }
}

export function SealBadge({ release }: { release: Pick<Release, "seal_status" | "issuer" | "issuer_key_id"> }) {
  const status = sealStatusOf(release);
  const Icon = ICONS[status];
  return (
    <div className="flex flex-col gap-0.5">
      <span
        className={`inline-flex w-fit items-center gap-1 px-2 py-0.5 rounded text-xs ${STYLES[status]}`}
        title={describe(status, release.issuer, release.issuer_key_id)}
        data-seal-status={status}
      >
        <Icon className="w-3.5 h-3.5" />
        {SEAL_LABELS[status]}
      </span>
      {status === "sealed" && (
        <span className="text-[11px] text-slate-500">
          {release.issuer} · <code>{release.issuer_key_id}</code>
        </span>
      )}
    </div>
  );
}
