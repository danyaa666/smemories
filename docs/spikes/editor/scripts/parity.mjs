// Question 5: editor view against the browser's PDF of the same page, A5 / A4 / Letter.
//   node scripts/parity.mjs   (needs `npm run dev` and poppler: pdftoppm pdfinfo pdffonts pdftotext) -> results/parity.json (+ results/parity-*.png)
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { PNG } from "pngjs";
import { BASE, diffPng, launch, ready } from "./lib.mjs";

const tmp = mkdtempSync(path.join(os.tmpdir(), "editor-parity-"));
const sh = (cmd, ...a) => execFileSync(cmd, a, { encoding: "utf8" });
const pinkBox = (png) => { // bounding box of the sticker's pink fill (#d9668c), to compare positions in px
  const P = PNG.sync.read(png); let x0 = 1e9, y0 = 1e9, x1 = -1, y1 = -1;
  for (let y = Math.floor(P.height * 0.6); y < P.height; y++) for (let x = 0; x < P.width; x++) { const i = (y * P.width + x) * 4;
    if (Math.abs(P.data[i] - 217) < 6 && Math.abs(P.data[i + 1] - 102) < 6 && Math.abs(P.data[i + 2] - 140) < 6) { x0 = Math.min(x0, x); y0 = Math.min(y0, y); x1 = Math.max(x1, x); y1 = Math.max(y1, y); } }
  return [x0, y0, x1, y1];
};
const out = {};
for (const unit of ["centre"]) for (const size of ["A5", "A4", "Letter"]) {
  const { browser, page } = await launch({ deviceScaleFactor: 2, viewport: { width: 1000, height: 1300 } });
  // editor view (screen media, stage scale 1 = 96 dpi, 2x device pixels = 192 dpi)
  await ready(page, `${BASE}/?view=editor&lib=inhouse&size=${size}&unit=${unit}&n=2`);
  const editor = await page.locator(".bk-sheet").screenshot();
  // the print route of the prototype -> PDF
  await page.goto(`${BASE}/?view=print&size=${size}&unit=${unit}`); await page.waitForSelector("[data-item]"); await page.evaluate(() => document.fonts.ready); await page.waitForTimeout(500);
  const pdf = path.join(tmp, `${size}.pdf`);
  writeFileSync(pdf, await page.pdf({ preferCSSPageSize: true, printBackground: true }));
  await browser.close();
  sh("pdftoppm", "-r", "192", "-png", "-singlefile", pdf, path.join(tmp, size));
  const raster = readFileSync(path.join(tmp, `${size}.png`));
  const d = diffPng(editor, raster);
  const d1 = diffPng(editor, raster, 48, 1); // 1 px tolerant (sub-pixel glyph edges)
  const info = sh("pdfinfo", pdf);
  const fonts = sh("pdffonts", pdf).split("\n").slice(2).filter(Boolean).map((l) => l.trim());
  const text = sh("pdftotext", "-raw", pdf, "-").replace(/\s+/g, " ").trim();
  const be = pinkBox(editor), bp = pinkBox(raster);
  out[`${unit}/${size}`] = {
    exact: d, tolerant_1px: d1, pages: Number(/Pages:\s+(\d+)/.exec(info)[1]), page_size_pt: /Page size:\s+([\d. x]+) pts/.exec(info)[1].trim(),
    fonts: fonts.length, fonts_all_embedded: fonts.every((f) => /\syes\s+\w+\s+\w+\s+\d+\s+\d+$/.test(f)),
    text_has_item: text.includes("Việt Nam — đường về nhà"),
    sticker_bbox_editor: be, sticker_bbox_pdf: bp, sticker_bbox_max_offset_px_at_192dpi: Math.max(...be.map((v, i) => Math.abs(v - bp[i]))),
  };
  writeFileSync(new URL(`../results/parity-${size}-editor.png`, import.meta.url), editor);
  writeFileSync(new URL(`../results/parity-${size}-pdf.png`, import.meta.url), raster);
  console.log(size, JSON.stringify(out[`${unit}/${size}`]));
}
writeFileSync(new URL("../results/parity.json", import.meta.url), JSON.stringify(out, null, 1));
