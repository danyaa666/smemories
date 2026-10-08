import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import { StrictMode, type ReactNode } from "react";
import { BrowserRouter, MemoryRouter, useLocation } from "react-router-dom";
import { vi } from "vitest";
import { App } from "../App";

export type Call = { method: string; path: string; body: unknown; search: string };
type Handler = Response | ((call: Call) => Response);

export const err = (status: number, code: string, headers?: Record<string, string>) =>
  Response.json({ error: { code, message: code, request_id: "r1" } }, { status, headers });

export const user = (over: object = {}) => ({
  id: "01J9Z3K6V8Q4M7N2P5R8T0W1XY",
  email: "lan@example.com",
  email_verified: true,
  display_name: "Lan Nguyễn",
  locale: "en",
  created_at: "2026-10-01T00:00:00Z",
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

export function renderApp(path: string, opts: { strict?: boolean; browser?: boolean } = {}) {
  // browser: a real window.history, so a test can see what the address bar holds when fetch runs
  if (opts.browser) window.history.replaceState(null, "", path);
  const Router = opts.browser ? BrowserRouter : MemoryRouter;
  const tree: ReactNode = (
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <Router {...(opts.browser ? {} : { initialEntries: [path] })}>
        <App />
        <Where />
      </Router>
    </QueryClientProvider>
  );
  return render(opts.strict ? <StrictMode>{tree}</StrictMode> : tree);
}
