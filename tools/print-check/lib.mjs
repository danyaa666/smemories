// Helpers of the print check: start the web dev server, serve a fixture book through Playwright routes, print to PDF, inspect the PDF with poppler.
import { execFileSync, spawn } from "node:child_process";
import { readFileSync, rmSync, statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { BOOK_ID, CLASS_ID, DESIGN_IMAGE, photoId } from "./fixture.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));
export const ROOT = path.resolve(HERE, "../..");
const PHOTOS = process.env.PRINT_CHECK_PHOTOS ?? path.join(HERE, "fixtures"); // the sample PDFs use lighter photos (make-samples.mjs)
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
export async function serveBook(page, book, classBook = null, { mediaDelayMs = 0 } = {}) {
  const seen = { bookData: 0, media: 0, classBook: 0, inFlight: 0, maxInFlight: 0, ids: new Set(), variants: new Set() }; // a media delay makes the requests in flight visible (T-146)
  const files = new Map(Array.from({ length: 30 }, (_, i) => [photoId(i + 1), `photo-${String((i % 30) + 1).padStart(2, "0")}.jpg`]));
  files.set(DESIGN_IMAGE, "photo-05.jpg"); // the student's design image (T-154): a background or an image item; served in the display and the print version
  await page.route(/^https?:\/\/[^/]+\/api\//, async (route) => { // only the API, not the dev server's /src/api/*.ts modules
    const u = new URL(route.request().url());
    if (u.pathname === "/api/user/get-me")
      return route.fulfill(ok({ id: "01J9Z3K6V8Q4M7N2P5R8T0W1XY", email: "hong@example.com", email_verified: true, display_name: "Hồng", locale: "vi", created_at: 1759276800000 }));
    // The shell around the book screen (YearbookLayout) reads the book and the note counts for the sidebar; without them it shows "this yearbook does not exist" (T-154: found by a run of this check, paused since D-37).
    if (u.pathname === "/api/yearbook/get" && book)
      return route.fulfill(ok({ ...book.yearbook, id: BOOK_ID, created_at: 1759276800000, updated_at: 1759276800000, profile: book.profile }));
    if (u.pathname === "/api/note/get-list")
      return route.fulfill(ok({ items: [], counts: { pending: 0, approved: book?.notes.length ?? 0, hidden: 0, total: book?.notes.length ?? 0 }, next_id: "" }));
    if (u.pathname === "/api/book-data/get") {
      seen.bookData++;
      return route.fulfill(ok(book));
    }
    if (u.pathname === "/api/class/get" && classBook) // the print screen reads the class first: only an admin gets the book (T-134)
      return route.fulfill(ok({ ...classBook.class, my_role: "admin", page_linked: false, created_at: 1, updated_at: 1 }));
    if (u.pathname === "/api/class-book/get" && classBook) {
      seen.classBook++;
      return route.fulfill(ok(classBook));
    }
    if (u.pathname === "/api/media/get-content") {
      seen.media++;
      seen.ids.add(u.searchParams.get("id"));
      seen.variants.add(`${u.searchParams.get("id")}:${u.searchParams.get("variant")}`);
      seen.maxInFlight = Math.max(seen.maxInFlight, ++seen.inFlight);
      if (mediaDelayMs) await new Promise((r) => setTimeout(r, mediaDelayMs));
      seen.inFlight--;
      const f = files.get(u.searchParams.get("id") ?? "");
      const variant = u.searchParams.get("variant");
      if (!(variant === "print" || (variant === "display" && u.searchParams.get("id") === DESIGN_IMAGE)) || !f) return route.fulfill({ status: 404, contentType: "application/json", body: '{"status":"ERROR_NOT_FOUND"}' });
      return route.fulfill({ status: 200, contentType: "image/jpeg", headers: { "Cache-Control": "private, max-age=3600" }, body: readFileSync(path.join(PHOTOS, f)) });
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

/** Opens the class book screen (T-130) on one paper size and waits until it allows printing. */
export async function openClassBook(page, size) {
  const t0 = performance.now();
  await page.goto(`${BASE}/classes/${CLASS_ID}/book?size=${size}`);
  await page.waitForSelector("html[data-book-ready]", { timeout: 90000 });
  return { readyMs: Math.round(performance.now() - t0), pages: Number(await page.evaluate(() => document.documentElement.dataset.bookReady)) };
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
  // A picture drawn on more than 3 pages is the paper texture of a template (the dotted paper), not a photo; a photo is counted once.
  const pagesOf = new Map();
  for (const c of images) pagesOf.set(c[10], (pagesOf.get(c[10]) ?? 0) + 1);
  const seen = new Set();
  const photos = images.filter((c) => {
    const isPhoto = c[2] === "image" && c[5] !== "gray" && Number(c[3]) >= 400 && pagesOf.get(c[10]) <= 3 && !seen.has(c[10]);
    if (isPhoto) seen.add(c[10]);
    return isPhoto;
  });
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

/** Every word of the PDF with its box in points, per page: [{ w, h, words: [{ t, x0, y0, x1, y1 }] }] (pdftotext -bbox). */
export function words(file) {
  const xml = run("pdftotext", "-bbox", file, "-");
  return [...xml.matchAll(/<page width="([\d.]+)" height="([\d.]+)">([\s\S]*?)<\/page>/g)].map(([, w, h, body]) => ({
    w: +w,
    h: +h,
    words: [...body.matchAll(/<word xMin="([\d.]+)" yMin="([\d.]+)" xMax="([\d.]+)" yMax="([\d.]+)">([^<]*)<\/word>/g)].map((m) => ({
      x0: +m[1], y0: +m[2], x1: +m[3], y1: +m[4], t: m[5].replaceAll("&amp;", "&").normalize("NFC"),
    })),
  }));
}

/** The text boxes (.bk-fit) of every page in print layout, as fractions of their sheet: [page][{ text, x0, y0, x1, y1 }]. */
export async function fitBoxes(page) {
  await page.emulateMedia({ media: "print" });
  return page.evaluate(() =>
    [...document.querySelectorAll(".bk-sheet")].map((sheet) => {
      const s = sheet.getBoundingClientRect();
      return [...sheet.querySelectorAll(".bk-fit")].map((e) => {
        const r = e.getBoundingClientRect();
        return { text: e.textContent, x0: (r.left - s.left) / s.width, y0: (r.top - s.top) / s.height, x1: (r.right - s.left) / s.width, y1: (r.bottom - s.top) / s.height };
      });
    }),
  );
}

/** Page `n` (1-based) of a PDF as an 8-bit grey picture at `dpi`: { w, h, px } (poppler: pdftoppm -gray, a binary PGM). */
export function raster(file, n, dpi = 100) {
  const prefix = `${file}.p${n}`;
  run("pdftoppm", "-r", String(dpi), "-f", String(n), "-l", String(n), "-gray", "-singlefile", file, prefix);
  const buf = readFileSync(`${prefix}.pgm`);
  rmSync(`${prefix}.pgm`);
  const [, w, h] = /^P5\s+(\d+)\s+(\d+)\s+255\s/.exec(buf.subarray(0, 32).toString("latin1"));
  return { w: +w, h: +h, px: buf.subarray(buf.length - w * h) };
}

/** Dark pixels (grey below 128) of a picture inside a box given as fractions of the page: { x0, y0, x1, y1 }. */
export function inkIn(pic, box) {
  let n = 0;
  for (let y = Math.floor(box.y0 * pic.h); y < Math.ceil(box.y1 * pic.h); y++)
    for (let x = Math.floor(box.x0 * pic.w); x < Math.ceil(box.x1 * pic.w); x++) if (pic.px[y * pic.w + x] < 128) n++;
  return n;
}

/** The signatures (svg.bk-sig) and the text boxes (.bk-fit) of every sheet in print layout, as fractions of the sheet. */
export async function signatureBoxes(page) {
  await page.emulateMedia({ media: "print" });
  return page.evaluate(() =>
    [...document.querySelectorAll(".bk-sheet")].map((sheet) => {
      const s = sheet.getBoundingClientRect();
      const box = (e) => {
        const r = e.getBoundingClientRect();
        return { x0: (r.left - s.left) / s.width, y0: (r.top - s.top) / s.height, x1: (r.right - s.left) / s.width, y1: (r.bottom - s.top) / s.height };
      };
      return {
        sigs: [...sheet.querySelectorAll("svg.bk-sig")].map(box),
        texts: [...sheet.querySelectorAll(".bk-fit")].map((e) => ({ ...box(e), text: e.textContent })),
      };
    }),
  );
}
