import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { api } from "./api";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  api.clearTokens();
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

    const [a, b] = await Promise.all([api.getInstances(), api.getInstances()]);

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
      channel: "stable",
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
      channel: "stable",
      artifact: file,
    });

    expect(res.id).toBe("r2");
    expect(refreshCalls).toBe(1);
    expect(uploadAttempts).toBe(2);
    expect(bodies.every((b) => b instanceof FormData)).toBe(true);
  });
});
