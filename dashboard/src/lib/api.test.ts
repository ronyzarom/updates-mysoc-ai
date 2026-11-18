import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { api, ApiError } from "./api";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  api.clearTokens();
  api.setAuthLostHandler(null);
});

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const authOf = (init?: RequestInit) =>
  (init?.headers as Record<string, string> | undefined)?.["Authorization"];

// fakeStorage stands in for the localStorage two tabs share. The client
// checks `window` at call time, so stubbing it per test exercises the
// browser-only multi-tab paths under the node test environment.
function shareStorage(): Map<string, string> {
  const m = new Map<string, string>();
  vi.stubGlobal("window", { addEventListener: () => {} });
  vi.stubGlobal("localStorage", {
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, v),
    removeItem: (k: string) => void m.delete(k),
  });
  return m;
}

describe("ApiError", () => {
  it("carries the HTTP status and server message", async () => {
    api.setTokens("tok", "ref");
    vi.stubGlobal("fetch", vi.fn(async () => json({ error: "promotion refused" }, 409)));
    const err = await api.getFleetStats().catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(409);
    expect(err.message).toBe("promotion refused");
  });
});

describe("session loss", () => {
  it("signs out when the refresh token is refused", async () => {
    api.setTokens("expired", "revoked");
    const lost = vi.fn();
    api.setAuthLostHandler(lost);
    vi.stubGlobal("fetch", vi.fn(async (url: string | URL) =>
      String(url).endsWith("/api/v1/auth/refresh") ? json({ error: "invalid" }, 401) : json({ error: "unauthorized" }, 401)
    ));
    await expect(api.getFleetStats()).rejects.toMatchObject({ status: 401 });
    expect(lost).toHaveBeenCalledTimes(1);
    expect(api.isAuthenticated()).toBe(false);
  });

  it.each([
    ["a 5xx from refresh", async () => json({ error: "db down" }, 503)],
    ["a network failure", async () => { throw new TypeError("network"); }],
  ])("keeps the session on %s", async (_label, refreshReply) => {
    api.setTokens("expired", "still-good");
    const lost = vi.fn();
    api.setAuthLostHandler(lost);
    vi.stubGlobal("fetch", vi.fn(async (url: string | URL) =>
      String(url).endsWith("/api/v1/auth/refresh") ? refreshReply() : json({ error: "unauthorized" }, 401)
    ));
    await expect(api.getFleetStats()).rejects.toBeInstanceOf(ApiError);
    expect(lost).not.toHaveBeenCalled();
    expect(api.isAuthenticated()).toBe(true);
  });

  it("logout never throws and always clears local tokens", async () => {
    api.setTokens("tok", "ref");
    vi.stubGlobal("fetch", vi.fn(async () => { throw new TypeError("offline"); }));
    await expect(api.logout()).resolves.toBeUndefined();
    expect(api.isAuthenticated()).toBe(false);
  });

  it("logoutAll reports failure and keeps the session", async () => {
    api.setTokens("tok", "ref");
    vi.stubGlobal("fetch", vi.fn(async () => json({ error: "boom" }, 500)));
    await expect(api.logoutAll()).rejects.toMatchObject({ status: 500 });
    expect(api.isAuthenticated()).toBe(true);
  });
});

describe("multi-tab tokens", () => {
  it("adopts a pair another tab already rotated instead of spending a revoked token", async () => {
    const shared = shareStorage();
    api.setTokens("old-access", "old-refresh");
    // Another tab refreshes first and writes the new pair.
    shared.set("access_token", "access-B");
    shared.set("refresh_token", "refresh-B");
    let refreshCalls = 0;
    vi.stubGlobal("fetch", vi.fn(async (url: string | URL, init?: RequestInit) => {
      if (String(url).endsWith("/api/v1/auth/refresh")) {
        refreshCalls++;
        return json({ error: "revoked" }, 401);
      }
      return authOf(init) === "Bearer access-B" ? json({ ok: true }) : json({ error: "unauthorized" }, 401);
    }));
    await expect(api.getFleetStats()).resolves.toEqual({ ok: true });
    expect(refreshCalls).toBe(0);
  });

  it("does not wipe the shared session when it loses a concurrent rotation race", async () => {
    const shared = shareStorage();
    api.setTokens("old-access", "old-refresh");
    const lost = vi.fn();
    api.setAuthLostHandler(lost);
    vi.stubGlobal("fetch", vi.fn(async (url: string | URL, init?: RequestInit) => {
      if (String(url).endsWith("/api/v1/auth/refresh")) {
        // The other tab won with the same token while this request was in flight.
        shared.set("access_token", "access-B");
        shared.set("refresh_token", "refresh-B");
        return json({ error: "revoked" }, 401);
      }
      return authOf(init) === "Bearer access-B" ? json({ ok: true }) : json({ error: "unauthorized" }, 401);
    }));
    await expect(api.getFleetStats()).resolves.toEqual({ ok: true });
    expect(lost).not.toHaveBeenCalled();
    expect(shared.get("refresh_token")).toBe("refresh-B");
  });
});

describe("token refresh single-flight", () => {
  beforeEach(() => {
    api.setTokens("expired-access", "refresh-1");
  });

  it("refreshes exactly once for concurrent 401s and retries with the new token", async () => {
    let refreshCalls = 0;
    const fetchMock = vi.fn(async (url: string | URL, init?: RequestInit) => {
      const u = String(url);
      if (u.endsWith("/api/v1/auth/refresh")) {
        refreshCalls++;
        return new Response(
          JSON.stringify({ access_token: "new-access", refresh_token: "new-refresh" }),
          { status: 200 }
        );
      }
      const auth = (init?.headers as Record<string, string> | undefined)?.["Authorization"];
      if (auth === "Bearer new-access") {
        return new Response(JSON.stringify([{ id: "1" }]), { status: 200 });
      }
      return new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 });
    });
    vi.stubGlobal("fetch", fetchMock);

    const [a, b] = await Promise.all([api.getFleetStats(), api.getFleetStats()]);

    expect(refreshCalls).toBe(1);
    expect(a).toEqual([{ id: "1" }]);
    expect(b).toEqual([{ id: "1" }]);
  });
});

describe("multipart upload", () => {
  beforeEach(() => {
    api.setTokens("token-A", "refresh-A");
  });

  it("sends FormData without a JSON content-type and with auth", async () => {
    let captured: RequestInit | undefined;
    const fetchMock = vi.fn(async (url: string | URL, init?: RequestInit) => {
      if (String(url).endsWith("/api/v1/releases")) {
        captured = init;
        return new Response(
          JSON.stringify({ id: "r1", product_name: "siemcore" }),
          { status: 200 }
        );
      }
      return new Response("{}", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);

    const file = new File(["binary-data"], "artifact.bin");
    const res = await api.uploadRelease({
      product: "siemcore",
      version: "1.0.0",
      artifact: file,
    });

    expect(res.id).toBe("r1");
    expect(captured?.body).toBeInstanceOf(FormData);
    const headers = captured?.headers as Record<string, string>;
    expect(headers["Content-Type"]).toBeUndefined();
    expect(headers["Authorization"]).toBe("Bearer token-A");
  });

  it("retries the upload with FormData intact after a token refresh", async () => {
    const bodies: unknown[] = [];
    let refreshCalls = 0;
    let uploadAttempts = 0;
    const fetchMock = vi.fn(async (url: string | URL, init?: RequestInit) => {
      const u = String(url);
      if (u.endsWith("/api/v1/auth/refresh")) {
        refreshCalls++;
        return new Response(
          JSON.stringify({ access_token: "token-B", refresh_token: "refresh-B" }),
          { status: 200 }
        );
      }
      if (u.endsWith("/api/v1/releases")) {
        uploadAttempts++;
        bodies.push(init?.body);
        const auth = (init?.headers as Record<string, string> | undefined)?.["Authorization"];
        if (auth === "Bearer token-B") {
          return new Response(JSON.stringify({ id: "r2" }), { status: 200 });
        }
        return new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 });
      }
      return new Response("{}", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);

    const file = new File(["binary-data"], "artifact.bin");
    const res = await api.uploadRelease({
      product: "siemcore",
      version: "1.0.0",
      artifact: file,
    });

    expect(res.id).toBe("r2");
    expect(refreshCalls).toBe(1);
    expect(uploadAttempts).toBe(2);
    expect(bodies.every((b) => b instanceof FormData)).toBe(true);
  });
});


describe("paired artifact upload", () => {
  it("sends both files in one release request", async () => {
    api.setTokens("fixture-token", "fixture-refresh");
    let body: FormData | undefined;
    vi.stubGlobal("fetch", vi.fn(async (_url: unknown, init?: RequestInit) => {
      body = init?.body as FormData;
      return new Response(JSON.stringify({ id: "paired" }), { status: 201 });
    }));
    await api.uploadRelease({ product: "siemcore", version: "1.0.1", target_groups: ["alpha"], artifact: new File(["boot"], "bootstrap.tgz"), update_artifact: new File(["thin"], "update.tgz"), artifact_variants: "[]" });
    expect(body?.has("channel")).toBe(false);
    expect(body?.get("target_groups")).toBe("alpha");
    expect(body?.has("artifact")).toBe(false);
    expect((body?.get("bootstrap") as File).name).toBe("bootstrap.tgz");
    expect((body?.get("update") as File).name).toBe("update.tgz");
    expect(body?.get("artifact_variants")).toBe("[]");
  });
});


describe("independent artifact publication", () => {
  it.each(["bootstrap", "update"] as const)("uploads %s without a paired file", async (kind) => {
    api.setTokens("fixture-token", "fixture-refresh");
    let body: FormData | undefined;
    vi.stubGlobal("fetch", vi.fn(async (_url: unknown, init?: RequestInit) => {
      body = init?.body as FormData;
      return new Response(JSON.stringify({ id: kind }), { status: 201 });
    }));
    const metadata = JSON.stringify({ kind, product: "mysoc", version: "2.0.0" });
    await api.uploadRelease({ product: "mysoc", version: "2.0.0", channel: "dual-alpha-mysoc", target_groups: ["alpha"], artifact_kind: kind, artifact: new File([kind], `${kind}.tgz`), artifact_metadata: metadata });
    expect((body?.get("artifact") as File).name).toBe(`${kind}.tgz`);
    expect(body?.get("channel")).toBe("dual-alpha-mysoc");
    expect(body?.get("artifact_kind")).toBe(kind);
    expect(body?.get("artifact_metadata")).toBe(metadata);
    expect(body?.has("bootstrap")).toBe(false);
    expect(body?.has("update")).toBe(false);
    expect(body?.has("artifact_variants")).toBe(false);
  });
});
