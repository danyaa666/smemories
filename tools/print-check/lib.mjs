// Shared helpers for spike T-054: start the web dev server, render the prototype to PDF with Chromium, inspect the PDF with poppler.
import { execFileSync, spawn } from "node:child_process";
import { mkdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const ROOT = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../..",
);
export const SIZES_MM = {
  a5: { w: 148, h: 210 },
  a4: { w: 210, h: 297 },
  letter: { w: 215.9, h: 279.4 },
};
export const PORT = 5199;
export const BASE = `http://localhost:${PORT}`;
export const url = (design, size, lang, extra = "") =>
  `${BASE}/spike/print?design=${design}&size=${size}&lang=${lang}${extra}`;

/** Starts `vite` (dev server: the route only exists there) and resolves with a stop function. */
export async function startServer() {
  const child = spawn("npx", ["vite", "--port", String(PORT), "--strictPort"], {
    cwd: path.join(ROOT, "web"),
    stdio: "ignore",
  });
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(`${BASE}/spike/print`)).ok) return () => child.kill();
    } catch {
      /* not up yet */
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  child.kill();
  throw new Error("vite did not start");
}

/** Opens the prototype and waits until fonts, photos and text fitting are done. Returns timings and the fit statistics. */
export async function open(page, design, size, lang, extra = "") {
  const t0 = performance.now();
  await page.goto(url(design, size, lang, extra));
  await page.waitForSelector("html[data-ready]", { timeout: 60000 });
  const ready = performance.now() - t0;
  const stats = await page.evaluate(() => ({
    pages: document.querySelectorAll(".sp-sheet").length,
    fit: Object.fromEntries(
      [...document.querySelectorAll("[data-fit]")].reduce(
        (m, e) => m.set(e.dataset.fit, (m.get(e.dataset.fit) ?? 0) + 1),
        new Map(),
      ),
    ),
  }));
  return { readyMs: Math.round(ready), ...stats };
}

/** page.pdf() with exactly what a user gets from "Save as PDF" with margins none and backgrounds on: CSS @page decides the size. */
export async function savePdf(page, file) {
  mkdirSync(path.dirname(file), { recursive: true });
  const t0 = performance.now();
  await page.pdf({
    path: file,
    preferCSSPageSize: true,
    printBackground: true,
  });
  return {
    pdfMs: Math.round(performance.now() - t0),
    bytes: statSync(file).size,
  };
}

const run = (cmd, ...args) =>
  execFileSync(cmd, args, { encoding: "utf8", maxBuffer: 64 << 20 });

/** Facts about a PDF from poppler (pdfinfo, pdffonts, pdfimages, pdftotext). */
export function inspect(file) {
  const info = run("pdfinfo", "-box", file);
  const get = (re) => info.match(re)?.[1];
  const [wpt, hpt] = (get(/Page size:\s+(.+) pts/) ?? "0 x 0")
    .split(" x ")
    .map(Number);
  const fonts = run("pdffonts", file)
    .split("\n")
    .slice(2)
    .filter(Boolean)
    .map((l) => ({
      name: l.split(/\s+/)[0],
      emb: /\s(yes|no)\s+(yes|no)\s+(yes|no)\s+\d+\s+\d+\s*$/.exec(l)?.[1],
    }));
  const images = run("pdfimages", "-list", file)
    .split("\n")
    .slice(2)
    .filter(Boolean)
    .map((l) => l.trim().split(/\s+/));
  // columns: page num type width height color comp bpc enc interp object ID x-ppi y-ppi size ratio
  const photos = images.filter(
    (c) => c[2] === "image" && c[5] !== "gray" && Number(c[3]) >= 400,
  );
  const ppis = photos.map((c) => Number(c[12]));
  return {
    pages: Number(get(/Pages:\s+(\d+)/)),
    sizePt: [wpt, hpt],
    sizeMm: [+((wpt * 25.4) / 72).toFixed(1), +((hpt * 25.4) / 72).toFixed(1)],
    pdfVersion: get(/PDF version:\s+(\S+)/),
    fonts,
    allFontsEmbedded: fonts.length > 0 && fonts.every((f) => f.emb === "yes"),
    images: images.length,
    photos: photos.length,
    photoPx: photos.length ? `${photos[0][3]}x${photos[0][4]}` : null,
    photoEncodings: [...new Set(photos.map((c) => c[8]))],
    ppiMin: ppis.length ? Math.min(...ppis) : null,
    ppiMax: ppis.length ? Math.max(...ppis) : null,
  };
}

export const text = (file) =>
  run("pdftotext", "-layout", file, "-").normalize("NFC");
export const rasterize = (file, pageNo, outPrefix, dpi = 100) =>
  run(
    "pdftoppm",
    "-png",
    "-r",
    String(dpi),
    "-f",
    String(pageNo),
    "-l",
    String(pageNo),
    file,
    outPrefix,
  );

/** Sum of resident memory (MB) of a process and all its descendants. */
export function treeRssMb(rootPid) {
  const rows = run("ps", "-A", "-o", "pid=,ppid=,rss=")
    .trim()
    .split("\n")
    .map((l) => l.trim().split(/\s+/).map(Number));
  const kids = new Map();
  for (const [pid, ppid] of rows)
    kids.set(ppid, [...(kids.get(ppid) ?? []), pid]);
  const rss = new Map(rows.map(([pid, , r]) => [pid, r]));
  let sum = 0;
  const walk = (p) => {
    sum += rss.get(p) ?? 0;
    (kids.get(p) ?? []).forEach(walk);
  };
  walk(rootPid);
  return Math.round(sum / 1024);
}

export const readText = (f) => readFileSync(f, "utf8");
