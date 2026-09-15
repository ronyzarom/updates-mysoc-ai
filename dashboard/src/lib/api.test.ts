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


describe("paired artifact upload", () => {
  it("sends both files in one release request", async () => {
    api.setTokens("fixture-token", "fixture-refresh");
    let body: FormData | undefined;
    vi.stubGlobal("fetch", vi.fn(async (_url: unknown, init?: RequestInit) => {
      body = init?.body as FormData;
      return new Response(JSON.stringify({ id: "paired" }), { status: 201 });
    }));
    await api.uploadRelease({ product: "siemcore", version: "1.0.1", channel: "stable", target_groups: ["alpha"], artifact: new File(["boot"], "bootstrap.tgz"), update_artifact: new File(["thin"], "update.tgz"), artifact_variants: "[]" });
    expect(body?.has("artifact")).toBe(false);
    expect((body?.get("bootstrap") as File).name).toBe("bootstrap.tgz");
    expect((body?.get("update") as File).name).toBe("update.tgz");
    expect(body?.get("artifact_variants")).toBe("[]");
  });
});


describe("independent artifact publication", () => {
  it.each(["bootstrap", "update"])("uploads %s without a paired file", async (kind) => {
    api.setTokens("fixture-token", "fixture-refresh");
    let body: FormData | undefined;
    vi.stubGlobal("fetch", vi.fn(async (_url: unknown, init?: RequestInit) => {
      body = init?.body as FormData;
      return new Response(JSON.stringify({ id: kind }), { status: 201 });
    }));
    const metadata = JSON.stringify({ kind, product: "mysoc", version: "2.0.0" });
    await api.uploadRelease({ product: "mysoc", version: "2.0.0", channel: "dual-alpha-mysoc", target_groups: ["alpha"], artifact_kind: kind, artifact: new File([kind], `${kind}.tgz`), artifact_metadata: metadata });
    expect((body?.get("artifact") as File).name).toBe(`${kind}.tgz`);
    expect(body?.get("artifact_kind")).toBe(kind);
    expect(body?.get("artifact_metadata")).toBe(metadata);
    expect(body?.has("bootstrap")).toBe(false);
    expect(body?.has("update")).toBe(false);
    expect(body?.has("artifact_variants")).toBe(false);
  });
});
