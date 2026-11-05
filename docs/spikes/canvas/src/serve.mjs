// A static file server on a free port (port 0) rooted at the repository checkout; the caller closes it. Serves only GET, only under the root.
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import path from "node:path";
const root = path.resolve(new URL("../../../../", import.meta.url).pathname);
const types = { ".html": "text/html", ".css": "text/css", ".woff2": "font/woff2", ".svg": "image/svg+xml", ".png": "image/png", ".js": "text/javascript", ".mjs": "text/javascript", ".json": "application/json", ".webp": "image/webp" };
export function serve(dir = root) {
  const srv = createServer(async (req, res) => {
    const p = path.resolve(dir, "." + decodeURIComponent(new URL(req.url, "http://x").pathname).replace(/\/$/, "/index.html"));
    if (!p.startsWith(dir)) { res.writeHead(403).end(); return; }
    try { const b = await readFile(p); res.writeHead(200, { "content-type": types[path.extname(p)] ?? "application/octet-stream" }).end(b); } catch { res.writeHead(404).end(); }
  });
  return new Promise((ok) => srv.listen(0, "127.0.0.1", () => ok({ base: `http://127.0.0.1:${srv.address().port}`, root: dir, close: () => srv.close() })));
}
