import { describe, expect, it, vi } from "vitest";
import { ApiError, getHealth, postJson, request } from "./client";

function stubFetch(impl: (...args: unknown[]) => Promise<Response>) {
  const fn = vi.fn(impl);
  vi.stubGlobal("fetch", fn);
  return fn;
}

const json = (body: unknown, init?: ResponseInit) =>
  new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
    ...init,
  });

describe("api client", () => {
  it("calls /api/healthz and returns the JSON body", async () => {
    const fetchMock = stubFetch(async () => json({ status: "ok" }));
    await expect(getHealth()).resolves.toEqual({ status: "ok" });
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/healthz");
  });

  it("maps the error envelope to ApiError", async () => {
    stubFetch(async () =>
      json(
        { error: { code: "not_ready", message: "db down", request_id: "req-1" } },
        { status: 503 },
      ),
    );
    const err = await getHealth().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({
      status: 503,
      code: "not_ready",
      message: "db down",
      requestId: "req-1",
    });
  });

  it("falls back to unknown_error and the X-Request-Id header for a non-envelope body", async () => {
    stubFetch(
      async () =>
        new Response("<html>boom</html>", { status: 502, headers: { "X-Request-Id": "r9" } }),
    );
    await expect(getHealth()).rejects.toMatchObject({
      status: 502,
      code: "unknown_error",
      requestId: "r9",
    });
  });

  it("maps a network failure to status 0", async () => {
    stubFetch(async () => {
      throw new TypeError("Failed to fetch");
    });
    await expect(getHealth()).rejects.toMatchObject({ status: 0, code: "network_error" });
  });

  it("keeps the entries of a Headers instance and adds Accept", async () => {
    const fetchMock = stubFetch(async () => json({}));
    await request("/x", { headers: new Headers({ "X-Test": "1" }) });
    const sent = new Headers((fetchMock.mock.calls[0]?.[1] as RequestInit).headers);
    expect(sent.get("X-Test")).toBe("1");
    expect(sent.get("Accept")).toBe("application/json");
  });

  it("reads Retry-After into the error", async () => {
    stubFetch(async () => new Response("{}", { status: 429, headers: { "Retry-After": "120" } }));
    await expect(getHealth()).rejects.toMatchObject({ status: 429, retryAfter: 120 });
  });

  it("postJson sends a JSON body, none without one", async () => {
    const fetchMock = stubFetch(async () => new Response(null, { status: 204 }));
    await postJson("/a", { k: 1 });
    await postJson("/b");
    const [a, b] = fetchMock.mock.calls.map((c) => c[1] as RequestInit);
    expect(a).toMatchObject({ method: "POST", body: '{"k":1}' });
    expect(new Headers(a?.headers).get("Content-Type")).toBe("application/json");
    expect(b?.body).toBeUndefined();
  });
});
