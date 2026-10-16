// Helpers of the print check: start the web dev server, serve a fixture book through Playwright routes, print to PDF, inspect the PDF with poppler.
import { execFileSync, spawn } from "node:child_process";
import { readFileSync, statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { BOOK_ID, photoId } from "./fixture.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));
export const ROOT = path.resolve(HERE, "../..");
export const PORT = Number(process.env.PRINT_CHECK_PORT ?? 5199);
export const BASE = `http://localhost:${PORT}`;

/** Starts `vite` in web/ and resolves with a stop function. */
export async function startServer() {
  const child = spawn("npx", ["vite", "--port", String(PORT), "--strictPort"], {
    cwd: path.join(ROOT, "web"),
    stdio: "ignore",
  });
  for (let i = 0; i < 150; i++) {
    try {
      if ((await fetch(BASE)).ok) return () => child.kill();
    } catch {
      /* not up yet */
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  child.kill();
  throw new Error("vite did not start");
}

const ok = (data) => ({ status: 200, contentType: "application/json", body: JSON.stringify({ status: "OK", data }) });

/** Serves the signed-in owner, the book and its photos; every other API call is a 404. Counts book-data and media requests. */
export async function serveBook(page, book) {
  const seen = { bookData: 0, media: 0 };
  const files = new Map(Array.from({ length: 30 }, (_, i) => [photoId(i + 1), `photo-${String((i % 30) + 1).padStart(2, "0")}.jpg`]));
  await page.route(/^https?:\/\/[^/]+\/api\//, (route) => { // only the API, not the dev server's /src/api/*.ts modules
    const u = new URL(route.request().url());
    if (u.pathname === "/api/user/get-me")
      return route.fulfill(ok({ id: "01J9Z3K6V8Q4M7N2P5R8T0W1XY", email: "hong@example.com", email_verified: true, display_name: "Hồng", locale: "vi", created_at: 1759276800000 }));
    if (u.pathname === "/api/book-data/get") {
      seen.bookData++;
      return route.fulfill(ok(book));
    }
    if (u.pathname === "/api/media/get-content") {
      seen.media++;
      const f = files.get(u.searchParams.get("id") ?? "");
      if (u.searchParams.get("variant") !== "print" || !f) return route.fulfill({ status: 404, contentType: "application/json", body: '{"status":"ERROR_NOT_FOUND"}' });
      return route.fulfill({ status: 200, contentType: "image/jpeg", headers: { "Cache-Control": "private, max-age=3600" }, body: readFileSync(path.join(HERE, "fixtures", f)) });
    }
    return route.fulfill({ status: 404, contentType: "application/json", body: '{"status":"ERROR_NOT_FOUND"}' });
  });
  return seen;
}

/** Opens the book screen and waits until it allows printing (html[data-book-ready] holds the page count). */
export async function open(page) {
  const t0 = performance.now();
  await page.goto(`${BASE}/yearbooks/${BOOK_ID}/book`);
  await page.waitForSelector("html[data-book-ready]", { timeout: 60000 });
  return { readyMs: Math.round(performance.now() - t0) };
}

/** page.pdf() with what "Save as PDF" gives with margins none and backgrounds on: CSS @page decides the size. */
export async function savePdf(page, file) {
  const t0 = performance.now();
  await page.pdf({ path: file, preferCSSPageSize: true, printBackground: true });
  return { pdfMs: Math.round(performance.now() - t0), bytes: statSync(file).size };
}

const run = (cmd, ...args) => execFileSync(cmd, args, { encoding: "utf8", maxBuffer: 64 << 20 });

/** Facts about a PDF from poppler (pdfinfo, pdffonts, pdfimages). */
export function inspect(file) {
  const info = run("pdfinfo", file);
  const get = (re) => info.match(re)?.[1];
  const [wpt, hpt] = (get(/Page size:\s+(.+) pts/) ?? "0 x 0").split(" x ").map(Number);
  const fonts = run("pdffonts", file)
    .split("\n")
    .slice(2)
    .filter(Boolean)
    .map((l) => ({ name: l.split(/\s+/)[0], emb: /\s(yes|no)\s+(yes|no)\s+(yes|no)\s+\d+\s+\d+\s*$/.exec(l)?.[1] }));
  const images = run("pdfimages", "-list", file).split("\n").slice(2).filter(Boolean).map((l) => l.trim().split(/\s+/));
  // columns: page num type width height color comp bpc enc interp object ID x-ppi y-ppi size ratio
  const photos = images.filter((c) => c[2] === "image" && c[5] !== "gray" && Number(c[3]) >= 400);
  const ppis = photos.map((c) => Number(c[12]));
  return {
    pages: Number(get(/Pages:\s+(\d+)/)),
    sizePt: [wpt, hpt],
    fonts,
    allFontsEmbedded: fonts.length > 0 && fonts.every((f) => f.emb === "yes"),
    photos: photos.length,
    ppiMin: ppis.length ? Math.min(...ppis) : null,
  };
}

export const text = (file) => run("pdftotext", "-layout", file, "-").normalize("NFC");
