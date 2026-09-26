import type { Instance, License, Release, SealStatus } from "./api";

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

// Numeric comparison of dotted version strings (MAJOR.MINOR.PATCH.BUILD).
// Returns <0 / 0 / >0 like a comparator, or null when either side is not
// purely numeric — never guess an ordering for labels like "dev".
export function compareVersions(a: string, b: string): number | null {
  const pa = a.trim().split(".").map(Number);
  const pb = b.trim().split(".").map(Number);
  if (pa.some(Number.isNaN) || pb.some(Number.isNaN)) return null;
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (d !== 0) return d;
  }
  return 0;
}

// A failed update attempt is moot once the node demonstrably runs the version
// the attempt was trying to reach (or newer) — e.g. it recovered through an
// out-of-band reinstall. Returns the current version proving supersession, or
// null when the failure still stands. The attempt record carries no product
// name, so: exact equality with anything the node reports (products or the
// updater itself) is decisive, while "newer than target" is only trusted on
// single-product nodes to avoid cross-product false positives.
export function supersededCurrentVersion(
  instance: Pick<
    Instance,
    "last_update_success" | "last_update_target_version" | "last_heartbeat_data"
  >
): string | null {
  if (instance.last_update_success !== false) return null;
  const target = instance.last_update_target_version;
  const hb = instance.last_heartbeat_data;
  if (!target || !hb) return null;

  const products = hb.products || [];
  const candidates = [
    ...products.map((p) => p.version),
    hb.updater_version,
  ].filter((v): v is string => !!v);

  const exact = candidates.find((v) => compareVersions(v, target) === 0);
  if (exact) return exact;

  if (products.length === 1 && products[0].version) {
    const cmp = compareVersions(products[0].version, target);
    if (cmp !== null && cmp > 0) return products[0].version;
  }
  return null;
}

// Returns instances ordered by most recent heartbeat first (non-mutating).
export function sortInstancesByHeartbeat(instances: Instance[] | undefined): Instance[] {
  return [...(instances || [])].sort(
    (a, b) =>
      new Date(b.last_heartbeat || 0).getTime() -
      new Date(a.last_heartbeat || 0).getTime()
  );
}

// Releases from servers before 1.16.2 carry no seal_status; they were never
// issuer-sealed.
export function sealStatusOf(release: Pick<Release, "seal_status">): SealStatus {
  return release.seal_status ?? "unsealed";
}

export const ISSUER_ORDER = ["mysoc", "siemcore", "swf", "updates", "other"] as const;

export interface IssuerSealCoverage {
  issuer: string;
  total: number;
  sealed: number;
  invalid: number;
  percentSealed: number;
}

// Per-issuer share of releases published in the last 30 days that carried a
// verified issuer seal. Issuers with no recent releases are omitted.
export function sealCoverage(
  releases: Pick<Release, "issuer" | "seal_status" | "released_at">[] | undefined,
  now = Date.now()
): IssuerSealCoverage[] {
  const byIssuer = new Map<string, IssuerSealCoverage>();
  for (const r of releases || []) {
    const at = new Date(r.released_at).getTime();
    if (Number.isNaN(at) || at < now - THIRTY_DAYS_MS || at > now) continue;
    const issuer = r.issuer || "other";
    const row = byIssuer.get(issuer) ?? { issuer, total: 0, sealed: 0, invalid: 0, percentSealed: 0 };
    row.total++;
    const status = sealStatusOf(r);
    if (status === "sealed") row.sealed++;
    if (status === "invalid") row.invalid++;
    byIssuer.set(issuer, row);
  }
  const rank = (i: string) => {
    const idx = (ISSUER_ORDER as readonly string[]).indexOf(i);
    return idx === -1 ? ISSUER_ORDER.length : idx;
  };
  return [...byIssuer.values()]
    .map((row) => ({ ...row, percentSealed: Math.round((row.sealed / row.total) * 100) }))
    .sort((a, b) => rank(a.issuer) - rank(b.issuer) || a.issuer.localeCompare(b.issuer));
}
