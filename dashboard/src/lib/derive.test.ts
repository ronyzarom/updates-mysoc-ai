import { describe, it, expect } from "vitest";
import {
  hasExpiry,
  isLicenseActive,
  licenseCounts,
  isActivePath,
  effectiveUpdateGroup,
  sortInstancesByHeartbeat,
  compareVersions,
  supersededCurrentVersion,
  THIRTY_DAYS_MS,
} from "./derive";
import type { Instance, HeartbeatData } from "./api";

const NOW = new Date("2026-06-01T00:00:00Z").getTime();
const inDays = (d: number) => new Date(NOW + d * 24 * 60 * 60 * 1000).toISOString();

describe("hasExpiry", () => {
  it("treats the Go zero-value timestamp as no expiry", () => {
    expect(hasExpiry("0001-01-01T00:00:00Z")).toBe(false);
    expect(hasExpiry("")).toBe(false);
    expect(hasExpiry(undefined)).toBe(false);
    expect(hasExpiry(inDays(10))).toBe(true);
  });
});

describe("isLicenseActive", () => {
  it("is inactive when disabled", () => {
    expect(isLicenseActive(inDays(10), false, NOW)).toBe(false);
  });
  it("is active with no expiry when enabled", () => {
    expect(isLicenseActive("0001-01-01T00:00:00Z", true, NOW)).toBe(true);
  });
  it("is inactive when expired", () => {
    expect(isLicenseActive(inDays(-1), true, NOW)).toBe(false);
  });
  it("is active when enabled and not expired", () => {
    expect(isLicenseActive(inDays(1), true, NOW)).toBe(true);
  });
});

describe("licenseCounts", () => {
  it("classifies active, expiring-soon, and inactive/expired consistently", () => {
    const licenses = [
      { is_active: true, expires_at: inDays(120) }, // active, not soon
      { is_active: true, expires_at: inDays(10) }, // active, expiring soon
      { is_active: true, expires_at: inDays(-5) }, // expired -> inactive/expired
      { is_active: false, expires_at: inDays(200) }, // disabled -> inactive/expired
      { is_active: true, expires_at: "0001-01-01T00:00:00Z" }, // active, never expires
    ];
    const counts = licenseCounts(licenses, NOW);
    expect(counts.active).toBe(3);
    expect(counts.expiringSoon).toBe(1);
    expect(counts.inactiveOrExpired).toBe(2);
  });

  it("does not count expired licenses as expiring soon", () => {
    const counts = licenseCounts([{ is_active: true, expires_at: inDays(-1) }], NOW);
    expect(counts.expiringSoon).toBe(0);
    expect(counts.inactiveOrExpired).toBe(1);
  });

  it("handles undefined input", () => {
    expect(licenseCounts(undefined)).toEqual({
      active: 0,
      expiringSoon: 0,
      inactiveOrExpired: 0,
    });
  });

  it("uses a 30-day expiring window", () => {
    const justInside = licenseCounts(
      [{ is_active: true, expires_at: new Date(NOW + THIRTY_DAYS_MS - 1000).toISOString() }],
      NOW
    );
    const justOutside = licenseCounts(
      [{ is_active: true, expires_at: new Date(NOW + THIRTY_DAYS_MS + 1000).toISOString() }],
      NOW
    );
    expect(justInside.expiringSoon).toBe(1);
    expect(justOutside.expiringSoon).toBe(0);
  });
});

describe("isActivePath", () => {
  it("matches root only exactly", () => {
    expect(isActivePath("/", "/")).toBe(true);
    expect(isActivePath("/instances", "/")).toBe(false);
  });
  it("matches nested routes for non-root items", () => {
    expect(isActivePath("/instances", "/instances")).toBe(true);
    expect(isActivePath("/instances/abc", "/instances")).toBe(true);
    expect(isActivePath("/licenses/abc", "/instances")).toBe(false);
  });
});

describe("effectiveUpdateGroup", () => {
  it("never returns an empty group", () => {
    expect(effectiveUpdateGroup("", "beta")).toBe("beta");
    expect(effectiveUpdateGroup("", "")).toBe("stable");
    expect(effectiveUpdateGroup("", undefined)).toBe("stable");
  });
  it("prefers the explicit selection", () => {
    expect(effectiveUpdateGroup("alpha", "beta")).toBe("alpha");
  });
});

describe("sortInstancesByHeartbeat", () => {
  it("orders most-recent heartbeat first and is non-mutating", () => {
    const input = [
      { id: "a", last_heartbeat: inDays(-3) },
      { id: "b", last_heartbeat: inDays(-1) },
      { id: "c" },
    ] as unknown as Instance[];
    const sorted = sortInstancesByHeartbeat(input);
    expect(sorted.map((i) => i.id)).toEqual(["b", "a", "c"]);
    // original array is untouched
    expect(input.map((i) => i.id)).toEqual(["a", "b", "c"]);
  });
});

describe("compareVersions", () => {
  it("compares four-part versions numerically, not lexically", () => {
    expect(compareVersions("2.2.0.31", "2.2.0.31")).toBe(0);
    expect(compareVersions("2.2.0.31", "2.2.0.4")!).toBeGreaterThan(0);
    expect(compareVersions("1.9.0.1", "1.10.0.1")!).toBeLessThan(0);
  });
  it("pads missing parts with zero", () => {
    expect(compareVersions("2.2.0", "2.2.0.0")).toBe(0);
    expect(compareVersions("2.2.1", "2.2.0.9")!).toBeGreaterThan(0);
  });
  it("refuses non-numeric versions instead of guessing", () => {
    expect(compareVersions("dev", "2.2.0.31")).toBeNull();
    expect(compareVersions("2.2.0.31", "2.2.0-rc1")).toBeNull();
  });
});

describe("supersededCurrentVersion", () => {
  const failedTo = (
    target: string,
    hb: Partial<HeartbeatData> | undefined
  ): Pick<
    Instance,
    "last_update_success" | "last_update_target_version" | "last_heartbeat_data"
  > => ({
    last_update_success: false,
    last_update_target_version: target,
    last_heartbeat_data: hb as HeartbeatData | undefined,
  });

  it("resolves the SWF lifecycle case: failed attempt, node reinstalled to the target", () => {
    // swf-e2e-ronyassus: failed 0.0.0.0 -> 2.2.0.31, now reports swf 2.2.0.31
    const inst = failedTo("2.2.0.31", {
      updater_version: "2.1.0.1",
      products: [{ name: "swf", version: "2.2.0.31", status: "installed" }],
    });
    expect(supersededCurrentVersion(inst)).toBe("2.2.0.31");
  });

  it("accepts a single-product node that moved past the failed target", () => {
    const inst = failedTo("2.2.0.31", {
      updater_version: "2.1.0.1",
      products: [{ name: "swf", version: "2.2.0.32", status: "installed" }],
    });
    expect(supersededCurrentVersion(inst)).toBe("2.2.0.32");
  });

  it("matches a failed updater self-update against the updater's own version", () => {
    const inst = failedTo("2.1.0.2", {
      updater_version: "2.1.0.2",
      products: [{ name: "swf", version: "2.2.0.31", status: "installed" }],
    });
    expect(supersededCurrentVersion(inst)).toBe("2.1.0.2");
  });

  it("does not cross-compare products on multi-product nodes", () => {
    // siemcore 3.3.133.1 > target 2.2.0.31 numerically, but it is a different
    // product — only an exact match may supersede on multi-product nodes.
    const inst = failedTo("2.2.0.31", {
      updater_version: "1.10.1.1",
      products: [
        { name: "siemcore", version: "3.3.133.1", status: "installed" },
        { name: "other", version: "9.0.0.0", status: "installed" },
      ],
    });
    expect(supersededCurrentVersion(inst)).toBeNull();
  });

  it("keeps a genuine failure red", () => {
    const inst = failedTo("2.2.0.31", {
      updater_version: "2.1.0.1",
      products: [{ name: "swf", version: "2.2.0.30", status: "installed" }],
    });
    expect(supersededCurrentVersion(inst)).toBeNull();
  });

  it("never supersedes successful or heartbeat-less attempts", () => {
    expect(
      supersededCurrentVersion({
        last_update_success: true,
        last_update_target_version: "2.2.0.31",
        last_heartbeat_data: undefined,
      })
    ).toBeNull();
    expect(supersededCurrentVersion(failedTo("2.2.0.31", undefined))).toBeNull();
  });
});
