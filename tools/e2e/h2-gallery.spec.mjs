// T-107: a gallery page must load completely when the browser multiplexes its requests (HTTP/2, what CloudFront and the ALB give browsers).
// Real Chromium loads a page of 30 and of 100 thumbnails through a small TLS proxy, over HTTP/2 and over HTTP/1.1. The proxy sends one HTTP/1.1
// request per stream to the API, as the edge does. With a per-account cap of 8 downloads 22 of 30 images answered 429 and stayed broken.
//   make e2e        (scripts/e2e.sh sets E2E_API_URL; see docs/e2e.md)
import { expect, request, test, chromium } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { readFileSync, mkdtempSync } from "node:fs";
import http from "node:http";
import http2 from "node:http2";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const PHOTO = path.join(HERE, "../print-check/fixtures/photo-01.jpg");
const BASE = process.env.E2E_BASE_URL;
const API = new URL(process.env.E2E_API_URL ?? BASE ?? "http://127.0.0.1");
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
// The terms version the API requires at sign-up: the Go constant is the single source (internal/legal/legal.go).
const TERMS_VERSION = readFileSync(path.join(HERE, "../../internal/legal/legal.go"), "utf8").match(/const CurrentVersion = "([^"]+)"/)[1];
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;

let server;
let origin = "";
let session = "";
let photoId = "";
let bookId = "";

test.beforeAll(async () => {
  if (!BASE || !OTP) return;
  // An account with one photo, made through the API (the Origin header is what the CSRF guard wants on a write).
  const api = await request.newContext({ baseURL: API.origin, extraHTTPHeaders: { Origin: BASE } });
  const reg = await api.post("/api/auth/register", { data: { email: `h2-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9`, display_name: "H2 gallery", accept_terms_version: TERMS_VERSION } });
  expect(reg.status(), "register (429: run `make reset-limits SMEM_ENV=test`)").toBe(200);
  session = /smem_session=([^;]+)/.exec(reg.headers()["set-cookie"] ?? "")?.[1] ?? "";
  expect(session).not.toBe("");
  const auth = { Cookie: `smem_session=${session}` };
  expect((await api.post("/api/auth/verify-email", { headers: auth, data: { code: OTP } })).status()).toBe(200);
  const book = await (await api.post("/api/yearbook/create", { headers: auth, data: { title: `h2 ${RUN}`, language: "en" } })).json();
  bookId = book.data.id;
  const up = await api.post("/api/media/create", {
    headers: auth,
    multipart: { yearbook_id: book.data.id, file: { name: "photo.jpg", mimeType: "image/jpeg", buffer: readFileSync(PHOTO) } },
  });
  expect(up.status()).toBe(200);
  photoId = (await up.json()).data.id;
  await api.dispose();

  // The edge: TLS with a throwaway certificate, HTTP/2 and HTTP/1.1 (ALPN), one HTTP/1.1 request to the API per request.
  const dir = mkdtempSync(path.join(os.tmpdir(), "h2-gallery-"));
  execFileSync("openssl", ["req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=127.0.0.1", "-addext", "subjectAltName=IP:127.0.0.1",
    "-keyout", path.join(dir, "key.pem"), "-out", path.join(dir, "cert.pem")], { stdio: "ignore" });
  server = http2.createSecureServer(
    { key: readFileSync(path.join(dir, "key.pem")), cert: readFileSync(path.join(dir, "cert.pem")), allowHTTP1: true, settings: { maxConcurrentStreams: 1000 } },
    (req, res) => {
      const u = new URL(req.url, "https://x");
      if (u.pathname === "/gallery") {
        const n = Number(u.searchParams.get("n"));
        const imgs = Array.from({ length: n }, (_, i) => `<img width="40" height="40" src="/api/media/get-content?id=${photoId}&variant=thumb&n=${i}">`);
        res.writeHead(200, { "content-type": "text/html" }).end(`<!doctype html><title>gallery</title>${imgs.join("")}`);
        return;
      }
      const { host: _h, connection: _c, "keep-alive": _k, "transfer-encoding": _t, ...rest } = req.headers;
      const headers = Object.fromEntries(Object.entries(rest).filter(([k]) => !k.startsWith(":"))); // HTTP/2 pseudo-headers
      const up = http.request({ host: API.hostname, port: API.port, path: req.url, method: req.method, headers, agent: false }, (r) => {
        const { connection: _c2, "keep-alive": _k2, "transfer-encoding": _t2, ...out } = r.headers;
        res.writeHead(r.statusCode, out);
        r.pipe(res);
      });
      up.on("error", () => res.writeHead(502).end());
      req.pipe(up);
    },
  );
  await new Promise((ok) => server.listen(0, "127.0.0.1", ok));
  origin = `https://127.0.0.1:${server.address().port}`;
});

test.afterAll(async () => {
  if (bookId) {
    // The yearbook and its photo go; the account stays (there is no endpoint to delete it). Never fails the run.
    const api = await request.newContext({ baseURL: API.origin, extraHTTPHeaders: { Origin: BASE, Cookie: `smem_session=${session}` } });
    await api.post("/api/yearbook/delete", { data: { id: bookId } }).catch(() => {});
    await api.dispose();
  }
  await new Promise((done) => (server ? server.close(done) : done()));
  server?.closeAllConnections?.();
});

for (const [proto, args] of [["h2", []], ["http/1.1", ["--disable-http2"]]]) {
  for (const n of [30, 100]) {
    test(`${n} thumbnails load completely over ${proto}`, async () => {
      test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
      const browser = await chromium.launch({ args });
      try {
        const context = await browser.newContext({ ignoreHTTPSErrors: true });
        await context.addCookies([{ name: "smem_session", value: session, url: origin, secure: true, httpOnly: true }]);
        const page = await context.newPage();
        const statuses = [];
        page.on("response", (r) => r.url().includes("/api/media/get-content") && statuses.push(r.status()));
        await page.goto(`${origin}/gallery?n=${n}`, { waitUntil: "load" });
        const loaded = await page.evaluate(() => [...document.images].filter((i) => i.complete && i.naturalWidth > 0).length);
        const used = await page.evaluate(() => [...new Set(performance.getEntriesByType("resource").map((e) => e.nextHopProtocol))]);
        console.log(`${proto}: ${loaded}/${n} thumbnails rendered, statuses ${JSON.stringify(Object.fromEntries([...new Set(statuses)].map((s) => [s, statuses.filter((x) => x === s).length])))}, protocol ${used}`);
        expect(used).toEqual([proto === "h2" ? "h2" : "http/1.1"]);
        expect(statuses.filter((s) => s !== 200)).toEqual([]);
        expect(loaded).toBe(n);
      } finally {
        await browser.close();
      }
    });
  }
}
