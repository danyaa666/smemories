import { describe, expect, it, vi } from "vitest";
import { ApiError, getHealth } from "./client";

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
});
