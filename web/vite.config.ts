import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The web app calls the API at /api/* on its own origin; the edge strips the prefix
// (board decision L-07). In dev, Vite plays the edge.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        rewrite: (path) => path.replace(/^\/api/, ""),
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
  },
});
