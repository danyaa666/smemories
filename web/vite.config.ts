import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The web app calls the API at /api/* on its own origin. Business endpoints (/api/<namespace>/<action>,
// docs/api-contract.md) are forwarded unchanged; only the /healthz and /readyz probes lose the "/api" prefix at the edge.
// In dev, Vite plays the edge.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        rewrite: (path) => path.replace(/^\/api(?=\/(?:healthz|readyz)(?:\/|\?|$))/, ""),
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
  },
});
