import type { components, operations } from "./schema";

// All API calls go to /api/* on our own origin; the edge strips the prefix (L-07).
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
  return body as T;
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
