// Contact sheet of the shipped page templates of Memory Book (T-177): one PNG per paper size, a column per page kind and a row per look, each page drawn by the
// book's own print route with the sample book (photos, a long Vietnamese name, notes). Usage: node contact-sheet.mjs [--sizes A5,A4,Letter] [--out DIR] [--width PX]
// Needs the dependencies of print-check (npm ci); starts its own vite on a free port. A5 is committed as docs/images/page-templates-memory-book.png.
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { chromium } from "playwright";
import { makeBook } from "./fixture.mjs";
import { open, ROOT, serveBook, startServer } from "./lib.mjs";
import { KINDS, layoutOf, sampleBook, shippedTemplates } from "./templates.mjs";

const arg = (name, def) => {
  const i = process.argv.indexOf(`--${name}`);
  return i > 0 ? process.argv[i + 1] : def;
};
const sizes = arg("sizes", "A5,A4,Letter").split(",");
const out = arg("out", path.join(ROOT, "docs/images"));
const cardWidth = Number(arg("width", 200)); // the width of one page in the sheet
const all = shippedTemplates();
const rows = Math.max(...KINDS.map((k) => all.filter((t) => t.kind === k).length));

const stop = await startServer();
const browser = await chromium.launch();
try {
  mkdirSync(out, { recursive: true });
  for (const size of sizes) {
    const cells = []; // { kind, row, id, png }
    for (let row = 0; row < rows; row++) {
      const pages = {};
      const ids = {};
      for (const k of KINDS) {
        const t = all.filter((x) => x.kind === k)[row];
        if (t) { pages[k] = t.page; ids[k] = t.id; }
      }
      const book = sampleBook(makeBook, size);
      book.layout = layoutOf(pages);
      const page = await browser.newPage({ viewport: { width: 1400, height: 1900 }, deviceScaleFactor: 1 });
      await serveBook(page, book);
      await open(page);
      await page.addStyleTag({ content: ".bk-ui { display: none !important } .bk-stage { --s: 1 !important; padding: 0 !important }" });
      for (const k of Object.keys(pages)) {
        const sheet = page.locator(`.bk-sheet[data-page="${k}"]`).first();
        await sheet.evaluate((e) => (e.style.cssText += ";position: fixed; left: 0; top: 0; z-index: 99999; box-shadow: none"));
        const box = await sheet.evaluate((e) => { const r = e.getBoundingClientRect(); return { width: r.width, height: r.height }; });
        cells.push({ kind: k, row, id: ids[k], png: (await page.screenshot({ clip: { x: 0, y: 0, ...box } })).toString("base64"), box });
        await sheet.evaluate((e) => (e.style.position = e.style.left = e.style.top = e.style.zIndex = ""));
      }
      await page.close();
    }
    const html = `<style>body{margin:0;background:#fff;font:11px sans-serif;color:#333}table{border-collapse:separate;border-spacing:12px}td{vertical-align:top;text-align:center}
      img{width:${cardWidth}px;display:block;border:1px solid #bbb}</style><table><tr>${KINDS.map((k) => `<th>${k}</th>`).join("")}</tr>${Array.from({ length: rows }, (_, r) =>
        `<tr>${KINDS.map((k) => { const c = cells.find((x) => x.kind === k && x.row === r); return `<td>${c ? `<img src="data:image/png;base64,${c.png}"><div>${c.id}</div>` : ""}</td>`; }).join("")}</tr>`).join("")}</table>`;
    const sheet = await browser.newPage({ viewport: { width: 400, height: 400 }, deviceScaleFactor: 1 });
    await sheet.setContent(html);
    await sheet.waitForFunction(() => [...document.images].every((i) => i.complete));
    const png = await sheet.locator("table").screenshot({ type: "png" });
    const file = path.join(out, `page-templates-memory-book${size === "A5" ? "" : `-${size}`}.png`);
    writeFileSync(file, png);
    console.log(`${file} ${(png.length / 1024).toFixed(0)} KB`);
    await sheet.close();
  }
} finally {
  await browser.close();
  stop();
}
