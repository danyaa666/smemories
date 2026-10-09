import { describe, expect, it } from "vitest";
import config from "../../vite.config";

// Only /api/v1 and the /healthz and /readyz probes lose the /api prefix; v2 paths reach the server unchanged.
describe("dev proxy", () => {
  const proxy = config.server?.proxy?.["/api"];
  const rewrite = typeof proxy === "object" ? proxy.rewrite : undefined;

  it.each([
    ["/api/v1/me", "/v1/me"],
    ["/api/v1/yearbooks?limit=5", "/v1/yearbooks?limit=5"],
    ["/api/v1", "/v1"],
    ["/api/healthz", "/healthz"],
    ["/api/readyz", "/readyz"],
    ["/api/readyz?x=1", "/readyz?x=1"],
    ["/api/healthzz", "/api/healthzz"],
    ["/api/yearbook/get?id=1", "/api/yearbook/get?id=1"],
    ["/api/v10/x", "/api/v10/x"],
    ["/api/auth/login", "/api/auth/login"],
  ])("%s -> %s", (from, to) => {
    expect(rewrite?.(from)).toBe(to);
  });
});
