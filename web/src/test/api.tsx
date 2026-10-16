import { QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import { StrictMode, type ReactNode } from "react";
import { BrowserRouter, MemoryRouter, useLocation } from "react-router-dom";
import { vi } from "vitest";
import { App } from "../App";
import { makeQueryClient } from "../queryClient";

export type Call = { method: string; path: string; body: unknown; search: string };
type Handler = Response | ((call: Call) => Response);

// v2 spelling of the codes whose locale key is not just the lower-cased name (see normaliseCode in api/client.ts).
const V2_NAMES: Record<string, string> = {
  unauthenticated: "ERROR_UNAUTHORIZED",
  internal_error: "ERROR_INTERNAL",
  invalid_body: "ERROR_PARAM",
  payload_too_large: "ERROR_TOO_LARGE",
};

/** A v2 error response; `code` is the lower-case locale key (`rate_limited`), spelled ERROR_RATE_LIMITED on the wire. */
export const err = (status: number, code: string, headers?: Record<string, string>) =>
  Response.json(
    {
      status: V2_NAMES[code] ?? `ERROR_${code.toUpperCase()}`,
      error_message: code,
      request_id: "r1",
    },
    { status, headers },
  );

/** A v2 success response: HTTP 200 {status: "OK", data}. */
export const ok = (data: object = {}) => Response.json({ status: "OK", data });

export const user = (over: object = {}) => ({
  id: "01J9Z3K6V8Q4M7N2P5R8T0W1XY",
  email: "lan@example.com",
  email_verified: true,
  display_name: "Lan Nguyễn",
  locale: "en",
  created_at: 1759276800000,
  ...over,
});

/** Stubs fetch with "METHOD /path" routes; unlisted routes fail the test. Returns the call log. */
export function mockApi(routes: Record<string, Handler>): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      const call: Call = {
        method: init?.method ?? "GET",
        path: url.replace(/^\/api/, ""),
        body: typeof init?.body === "string" ? JSON.parse(init.body) : undefined,
        search: window.location.search,
      };
      calls.push(call);
      const h = routes[`${call.method} ${call.path}`];
      if (!h) throw new Error(`unexpected request ${call.method} ${call.path}`);
      return typeof h === "function" ? h(call) : h.clone();
    }),
  );
  return calls;
}

function Where() {
  const l = useLocation();
  return <div data-testid="where">{l.pathname + l.search}</div>;
}

export function renderApp(
  path: string,
  opts: { strict?: boolean; browser?: boolean; retry?: number } = {},
) {
  // browser: a real window.history, so a test can see what the address bar holds when fetch runs
  if (opts.browser) {
    window.history.replaceState(null, "", path);
  }
  const Router = opts.browser ? BrowserRouter : MemoryRouter;
  const tree: ReactNode = (
    <QueryClientProvider client={makeQueryClient(opts.retry)}>
      <Router {...(opts.browser ? {} : { initialEntries: [path] })}>
        <App />
        <Where />
      </Router>
    </QueryClientProvider>
  );
  return render(opts.strict ? <StrictMode>{tree}</StrictMode> : tree);
}
