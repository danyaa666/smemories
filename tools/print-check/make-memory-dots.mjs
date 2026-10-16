// Makes web/src/book/templates/memory-dots.png: the dotted paper of the memory-book template (design/canvas/temp1.html:
// radial dots every 22 px, ink at 13 %), as one transparent picture 816 x 1158 design px at 2x (192 dpi on Letter; each extra pixel costs PDF time per page).
// Why a picture: Chromium prints a tiled gradient as a 72 dpi raster and Firefox as black (ADR 0003, finding 5), and one dot per
// vector path makes the PDF 4x bigger (8 MB for 24 pages); one image is stored once in the PDF.   Run: node make-memory-dots.mjs
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const H = 1158; // the tallest design box: A5 (816 x 1158); A4 and Letter show the top of it
const out = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../web/src/book/templates/memory-dots.png");
const rows = Array.from({ length: Math.ceil(H / 22) }, (_, j) => `M11 ${11 + 22 * j}H816`).join("");
const svg = `<svg width="816" height="${H}" style="display:block"><path d="${rows}" stroke="#3B2F4A" stroke-opacity=".13" stroke-width="2.9" stroke-linecap="round" stroke-dasharray="0 22" fill="none"/></svg>`;
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 816, height: H }, deviceScaleFactor: 2 });
await page.setContent(`<body style="margin:0;background:transparent">${svg}`);
await page.screenshot({ path: out, omitBackground: true });
await browser.close();
console.log("wrote", out);
