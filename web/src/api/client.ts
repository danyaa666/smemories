import type { components, operations } from "./schema";

// All API calls go to /api/* on our own origin. Legacy paths are /api/v1/... (the edge strips
// /api); v2 paths are /api/<namespace>/<action> and reach the server unchanged (docs/api-contract.md).
const BASE = "/api";

export class ApiError extends Error {
  readonly status: number; // 0 when the server could not be reached
  readonly code: string;
  readonly requestId: string | undefined;
  readonly retryAfter: number | undefined; // seconds, from the Retry-After header (429)

  constructor(
    status: number,
    code: string,
    message: string,
    requestId?: string,
    retryAfter?: number,
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
    this.retryAfter = retryAfter;
  }
}

type ErrorEnvelope = components["schemas"]["Error"];

/** v2 error body: {status: "ERROR_...", error_message, request_id}. */
interface V2Error {
  status: string;
  error_message?: string;
  request_id?: string;
}

// v2 codes whose legacy spelling is not just the lower-cased name; the locale files and every
// screen still key on the legacy spelling.
const LEGACY_CODES: Record<string, string> = {
  ERROR_INTERNAL: "internal_error",
  ERROR_UNAUTHORIZED: "unauthenticated",
  ERROR_PARAM: "invalid_body",
  ERROR_TOO_LARGE: "payload_too_large",
};

const legacyCode = (v2: string) => LEGACY_CODES[v2] ?? v2.replace(/^ERROR_/, "").toLowerCase();

function isV2Error(body: unknown): body is V2Error {
  return (
    typeof body === "object" &&
    body !== null &&
    "status" in body &&
    typeof body.status === "string" &&
    body.status.startsWith("ERROR_")
  );
}

/** The payload of a success body: `data` of a v2 {status:"OK",data}, anything else as it is. */
export function unwrapSuccess(body: unknown): unknown {
  if (
    typeof body === "object" &&
    body !== null &&
    "status" in body &&
    body.status === "OK" &&
    "data" in body
  ) {
    return body.data;
  }
  return body;
}

function isEnvelope(body: unknown): body is ErrorEnvelope {
  if (typeof body !== "object" || body === null || !("error" in body)) return false;
  const e = body.error;
  return typeof e === "object" && e !== null && "code" in e && typeof e.code === "string";
}

/** The ApiError for a failed response (shared by fetch and the XHR upload). */
export function apiErrorFrom(
  status: number,
  body: unknown,
  header: (name: string) => string | null,
): ApiError {
  const headerId = header("X-Request-Id") ?? undefined;
  const retry = Number(header("Retry-After"));
  const retryAfter = Number.isFinite(retry) && retry > 0 ? retry : undefined;
  if (isV2Error(body)) {
    return new ApiError(
      status,
      legacyCode(body.status),
      body.error_message ?? `HTTP ${status}`,
      body.request_id || headerId,
      retryAfter,
    );
  }
  if (isEnvelope(body)) {
    const { code, message, request_id } = body.error;
    return new ApiError(status, code, message, request_id || headerId, retryAfter);
  }
  return new ApiError(status, "unknown_error", `HTTP ${status}`, headerId, retryAfter);
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    const headers = new Headers(init?.headers); // also accepts Headers instances and tuples
    if (!headers.has("Accept")) headers.set("Accept", "application/json");
    res = await fetch(BASE + path, { ...init, headers });
  } catch {
    throw new ApiError(0, "network_error", "Could not reach the API");
  }

  const body: unknown = await res.json().catch(() => undefined);
  if (!res.ok) throw apiErrorFrom(res.status, body, (n) => res.headers.get(n));
  return unwrapSuccess(body) as T;
}

type JsonOk<Op extends keyof operations> = operations[Op] extends {
  responses: { 200: { content: { "application/json": infer B } } };
}
  ? B
  : never;

export const getHealth = () => request<JsonOk<"getHealth">>("/healthz");

/** POST a JSON body (or none); the session cookie travels automatically (same origin). */
export const postJson = <T>(path: string, body?: unknown) =>
  request<T>(path, {
    method: "POST",
    ...(body === undefined
      ? {}
      : { headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }),
  });

/** Any method with an optional JSON body (PATCH, PUT, DELETE). */
export const sendJson = <T>(method: string, path: string, body?: unknown) =>
  request<T>(path, {
    method,
    ...(body === undefined
      ? {}
      : { headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }),
  });
