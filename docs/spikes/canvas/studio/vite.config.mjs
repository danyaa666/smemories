import path from "node:path";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Two page entries like the product's two routes: the book/editor shell (index.html) and the print route (print.html). The studio is a dynamic import.
const here = path.dirname(fileURLToPath(import.meta.url));
export default defineConfig({
  root: here,
  plugins: [react()],
  build: { outDir: path.join(here, "dist"), emptyOutDir: true, manifest: true, rollupOptions: { input: { main: path.join(here, "index.html"), print: path.join(here, "print.html") } } },
  server: { host: "127.0.0.1" },
});
