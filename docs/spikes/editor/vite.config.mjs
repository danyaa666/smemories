import path from "node:path";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The prototype renders the REAL book components of web/src (alias @web) with a layout layer on top.
// web/node_modules must be installed (cd web && npm ci); react is deduped to web's copy so there is one React.
const here = path.dirname(fileURLToPath(import.meta.url));
const web = path.resolve(here, "../../../web");
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@web": path.join(web, "src") },
    dedupe: ["react", "react-dom"],
  },
  server: { port: Number(process.env.SPIKE_PORT ?? 5391), fs: { allow: [path.resolve(here, "../../..")] } },
  preview: { port: Number(process.env.SPIKE_PORT ?? 5391) },
  build: { outDir: "dist", chunkSizeWarningLimit: 4000 },
});
