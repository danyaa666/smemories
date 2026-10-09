import { describe, expect, it, vi } from "vitest";
import { ApiError, apiErrorFrom, getHealth, postJson, request, unwrapSuccess } from "./client";

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

describe("v2 envelope", () => {
  it("returns data of {status:OK,data}", async () => {
    stubFetch(async () => json({ status: "OK", data: { id: "01J", created_at: 1760000000000 } }));
    await expect(request("/yearbook/get")).resolves.toEqual({
      id: "01J",
      created_at: 1760000000000,
    });
  });

  it("still returns old bodies as they are, including a lower-case status", async () => {
    stubFetch(async () => json({ status: "ok" }));
    await expect(request("/healthz")).resolves.toEqual({ status: "ok" });
    expect(unwrapSuccess({ status: "OK" })).toEqual({ status: "OK" }); // no data key: not a v2 body
    expect(unwrapSuccess(undefined)).toBeUndefined();
    expect(unwrapSuccess("text")).toBe("text");
  });

  it.each([
    ["ERROR_INVALID_TITLE", "invalid_title"],
    ["ERROR_INTERNAL", "internal_error"],
    ["ERROR_UNAUTHORIZED", "unauthenticated"],
    ["ERROR_PARAM", "invalid_body"],
    ["ERROR_TOO_LARGE", "payload_too_large"],
    ["ERROR_NOT_FOUND", "not_found"],
    ["ERROR_RATE_LIMITED", "rate_limited"],
  ])("maps %s to the legacy code %s", async (v2, legacy) => {
    stubFetch(async () =>
      json({ status: v2, error_message: "english text", request_id: "req-7" }, { status: 400 }),
    );
    const err = await request("/x").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({
      status: 400,
      code: legacy,
      message: "english text",
      requestId: "req-7",
    });
  });

  it("keeps Retry-After on a v2 429 and falls back to the header request id", () => {
    const headers: Record<string, string> = { "Retry-After": "30", "X-Request-Id": "hdr-1" };
    const err = apiErrorFrom(
      429,
      { status: "ERROR_RATE_LIMITED", error_message: "slow down" },
      (n) => headers[n] ?? null,
    );
    expect(err).toMatchObject({
      status: 429,
      code: "rate_limited",
      retryAfter: 30,
      requestId: "hdr-1",
    });
  });

  it("maps a v2 401 to unauthenticated", () => {
    const err = apiErrorFrom(401, { status: "ERROR_UNAUTHORIZED", error_message: "x" }, () => null);
    expect(err).toMatchObject({ status: 401, code: "unauthenticated" });
  });

  it("handles a v2 error without a message and a body that is not JSON", async () => {
    expect(apiErrorFrom(500, { status: "ERROR_INTERNAL" }, () => null)).toMatchObject({
      code: "internal_error",
      message: "HTTP 500",
    });
    stubFetch(async () => new Response("<html>bad gateway</html>", { status: 502 }));
    await expect(request("/x")).rejects.toMatchObject({ status: 502, code: "unknown_error" });
  });

  it("does not mistake an ok status string for an error", () => {
    expect(apiErrorFrom(500, { status: "OK" }, () => null)).toMatchObject({
      code: "unknown_error",
    });
  });
});
