// Question 2: print parity of three converted templates, ADR 0004 question 5 method. The sheet on screen at paper size (device scale 2 = 192 dpi) against the
// browser's PDF of the same HTML rasterised at 192 dpi: a pixel difference (1-px tolerant) and the ink box of every unrotated shape within 2 px.
//   node scripts/parity.mjs (needs poppler: pdftoppm, pdfinfo, pdffonts, pdftotext) -> results/parity.json
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { chromium } from "playwright";
import { PNG } from "pngjs";
import { PAPERS, TOKENS, convertPage } from "../src/convert.mjs";
import { place, sheetHtml } from "../src/render.mjs";
import { serve } from "../src/serve.mjs";

const sh = (cmd, ...a) => execFileSync(cmd, a, { encoding: "utf8" });
const tmp = mkdtempSync(path.join(os.tmpdir(), "canvas-parity-"));
const slugs = JSON.parse(readFileSync(new URL("../results/selection.json", import.meta.url), "utf8")).best.map((x) => x.slug);
const rgb = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16));

function diffPng(a, b, tol = 48, radius = 1) { // pixels of A that no pixel of B within `radius` px is close to (glyph edges move by a sub-pixel between screen and PDF raster)
  const A = PNG.sync.read(a), B = PNG.sync.read(b), w = Math.min(A.width, B.width), h = Math.min(A.height, B.height);
  let bad = 0;
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) {
    const i = (y * A.width + x) * 4; let d = 1e9;
    for (let dy = -radius; dy <= radius; dy++) for (let dx = -radius; dx <= radius; dx++) {
      const yy = y + dy, xx = x + dx; if (yy < 0 || xx < 0 || yy >= h || xx >= w) continue;
      const j = (yy * B.width + xx) * 4; d = Math.min(d, Math.max(Math.abs(A.data[i] - B.data[j]), Math.abs(A.data[i + 1] - B.data[j + 1]), Math.abs(A.data[i + 2] - B.data[j + 2])));
    }
    if (d > tol) bad++;
  }
  return { size_mismatch: [A.width - B.width, A.height - B.height], diff_pct: Math.round((bad / (w * h)) * 10000) / 100 };
}
function inkBox(png, [x0, y0, x1, y1], colour) { // bounding box of the pixels within 3/255 of `colour` inside a search window
  const P = PNG.sync.read(png); let a = 1e9, b = 1e9, c = -1, d = -1;
  for (let y = Math.max(0, y0); y < Math.min(P.height, y1); y++) for (let x = Math.max(0, x0); x < Math.min(P.width, x1); x++) {
    const i = (y * P.width + x) * 4;
    if (colour.every((v, k) => Math.abs(P.data[i + k] - v) < 3)) { a = Math.min(a, x); b = Math.min(b, y); c = Math.max(c, x); d = Math.max(d, y); }
  }
  return c < 0 ? null : [a, b, c, d];
}

const srv = await serve(), browser = await chromium.launch(), out = {};
for (const slug of slugs) {
  const design = JSON.parse(readFileSync(new URL(`../doory/backend/data/templates/${slug}.json`, import.meta.url), "utf8"));
  const { page: layout } = convertPage(design, 0, { slug, kind: "cover", stretch: true }); // the proposed shapes: edge-to-edge boxes measure geometry best
  for (const paper of Object.keys(PAPERS)) {
    const ctx = await browser.newContext({ deviceScaleFactor: 2, viewport: { width: 1000, height: 1300 } }), page = await ctx.newPage();
    const file = `docs/spikes/canvas/out/renders/parity-${paper}.html`;
    writeFileSync(new URL(`../out/renders/parity-${paper}.html`, import.meta.url), sheetHtml(layout, paper));
    await page.goto(`${srv.base}/${file}`); await page.waitForSelector("body[data-ready]");
    const screen = await page.locator("#sheet").screenshot();
    const pdf = path.join(tmp, `${slug}-${paper}.pdf`);
    writeFileSync(pdf, await page.pdf({ preferCSSPageSize: true, printBackground: true }));
    await ctx.close();
    sh("pdftoppm", "-r", "192", "-png", "-singlefile", pdf, path.join(tmp, `${slug}-${paper}`));
    const raster = readFileSync(path.join(tmp, `${slug}-${paper}.png`));
    const shapes = layout.items.filter((i) => i.t === "asset" && !i.r && i.tint && TOKENS[i.tint] && rgb(TOKENS[i.tint]).some((v, k) => Math.abs(v - rgb(TOKENS[layout.bg.color])[k]) > 8)) // a shape the colour of the page cannot be found in a picture;
    let maxOff = 0, maxVsGeom = 0, checked = 0;
    for (const it of shapes) {
      const r = place(it, paper), k = (PAPERS[paper][0] * 96) / 25.4 / 816 * 2; // design px -> device px
      const win = [Math.floor(r.left * k) - 12, Math.floor(r.top * k) - 12, Math.ceil((r.left + r.width) * k) + 12, Math.ceil((r.top + r.height) * k) + 12];
      const be = inkBox(screen, win, rgb(TOKENS[it.tint])), bp = inkBox(raster, win, rgb(TOKENS[it.tint]));
      if (!be || !bp || r.width * k < 8 || r.height * k < 8) continue; // hidden under later items, or too thin to find
      checked++; if (process.env.DEBUG_PARITY) console.log(' ', it.asset, it.tint, 'screen', be, 'pdf', bp, 'geom', [r.left*k, r.top*k, (r.left+r.width)*k, (r.top+r.height)*k].map(Math.round));
      maxOff = Math.max(maxOff, ...be.map((v, i) => Math.abs(v - bp[i])));
      maxVsGeom = Math.max(maxVsGeom, Math.abs(be[0] - r.left * k), Math.abs(be[1] - r.top * k)); // screen against the geometry rule itself
    }
    const fonts = sh("pdffonts", pdf).split("\n").slice(2).filter(Boolean);
    const text = sh("pdftotext", "-raw", pdf, "-").replace(/\s+/g, " ");
    const firstText = layout.items.find((i) => i.t === "text").text.replace(/\s+/g, " ").slice(0, 12);
    out[`${slug}/${paper}`] = { ...diffPng(screen, raster), pages: Number(/Pages:\s+(\d+)/.exec(sh("pdfinfo", pdf))[1]), fonts: fonts.length, fonts_all_embedded: fonts.every((f) => /\syes\s+\w+\s+\w+\s+\d+\s+\d+$/.test(f)), text_in_pdf: text.includes(firstText), shapes_checked: checked, max_shape_edge_offset_px_192dpi: maxOff, max_screen_vs_geometry_px: Math.round(maxVsGeom * 10) / 10 };
    console.log(slug, paper, JSON.stringify(out[`${slug}/${paper}`]));
  }
}
await browser.close(); srv.close();
writeFileSync(new URL("../results/parity.json", import.meta.url), JSON.stringify(out, null, 1) + "\n");
