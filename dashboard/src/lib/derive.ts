import type { Instance, License } from "./api";

// Treat the Go zero-value timestamp ("0001-01-01...") as "no expiry".
export function hasExpiry(expiresAt?: string): boolean {
  return !!expiresAt && !expiresAt.startsWith("0001-01-01");
}

// A license is active only if enabled and not past its expiry.
export function isLicenseActive(expiresAt: string, isActive: boolean, now = Date.now()): boolean {
  if (!isActive) return false;
  if (!hasExpiry(expiresAt)) return true;
  return new Date(expiresAt).getTime() > now;
}

export const THIRTY_DAYS_MS = 30 * 24 * 60 * 60 * 1000;

export interface LicenseCounts {
  active: number;
  expiringSoon: number;
  inactiveOrExpired: number;
}

// Derives the license summary counters used on the licenses page. Definitions
// are mutually consistent: expiringSoon only counts still-active licenses.
export function licenseCounts(
  licenses: Pick<License, "is_active" | "expires_at">[] | undefined,
  now = Date.now()
): LicenseCounts {
  const counts: LicenseCounts = { active: 0, expiringSoon: 0, inactiveOrExpired: 0 };
  for (const l of licenses || []) {
    const expiring = hasExpiry(l.expires_at);
    const expMs = expiring ? new Date(l.expires_at).getTime() : 0;
    const expired = expiring && expMs <= now;

    if (l.is_active && !expired) {
      counts.active++;
      if (expiring && expMs > now && expMs < now + THIRTY_DAYS_MS) {
        counts.expiringSoon++;
      }
    }
    if (!l.is_active || expired) {
      counts.inactiveOrExpired++;
    }
  }
  return counts;
}

// A nav item is active for an exact match, or for any nested route under it
// (e.g. /instances is active on /instances/abc). "/" only matches exactly.
export function isActivePath(pathname: string, href: string): boolean {
  if (href === "/") return pathname === "/";
  return pathname === href || pathname.startsWith(href + "/");
}

// Resolves the update group to submit, never returning an empty value.
export function effectiveUpdateGroup(selected: string, current?: string): string {
  return selected || current || "stable";
}

// Returns instances ordered by most recent heartbeat first (non-mutating).
export function sortInstancesByHeartbeat(instances: Instance[] | undefined): Instance[] {
  return [...(instances || [])].sort(
    (a, b) =>
      new Date(b.last_heartbeat || 0).getTime() -
      new Date(a.last_heartbeat || 0).getTime()
  );
}
