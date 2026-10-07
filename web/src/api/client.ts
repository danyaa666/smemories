import type { components, operations } from "./schema";

// All API calls go to /api/* on our own origin; the edge strips the prefix (L-07).
const BASE = "/api";

export class ApiError extends Error {
  readonly status: number; // 0 when the server could not be reached
  readonly code: string;
  readonly requestId: string | undefined;

  constructor(status: number, code: string, message: string, requestId?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

type ErrorEnvelope = components["schemas"]["Error"];

function isEnvelope(body: unknown): body is ErrorEnvelope {
  if (typeof body !== "object" || body === null || !("error" in body)) return false;
  const e = body.error;
  return typeof e === "object" && e !== null && "code" in e && typeof e.code === "string";
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(BASE + path, {
      ...init,
      headers: { Accept: "application/json", ...init?.headers },
    });
  } catch {
    throw new ApiError(0, "network_error", "Could not reach the API");
  }

  const body: unknown = await res.json().catch(() => undefined);
  if (!res.ok) {
    const headerId = res.headers.get("X-Request-Id") ?? undefined;
    if (isEnvelope(body)) {
      const { code, message, request_id } = body.error;
      throw new ApiError(res.status, code, message, request_id || headerId);
    }
    throw new ApiError(res.status, "unknown_error", `HTTP ${res.status}`, headerId);
  }
  return body as T;
}

type JsonOk<Op extends keyof operations> = operations[Op] extends {
  responses: { 200: { content: { "application/json": infer B } } };
}
  ? B
  : never;

export const getHealth = () => request<JsonOk<"getHealth">>("/healthz");
