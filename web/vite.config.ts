import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The web app calls the API at /api/* on its own origin. The legacy endpoints are /api/v1/*
// and the edge strips "/api" for them (board decision L-07); v2 endpoints (/api/<namespace>/<action>,
// docs/api-contract.md) are forwarded unchanged. In dev, Vite plays the edge.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        rewrite: (path) => path.replace(/^\/api\/v1(?=\/|\?|$)/, "/v1"),
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
  },
});
