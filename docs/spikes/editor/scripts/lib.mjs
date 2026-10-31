import { chromium } from "playwright";
export const BASE = process.env.SPIKE_URL ?? "http://localhost:5391";
export async function launch(opts = {}) {
  const browser = await chromium.launch();
  const ctx = await browser.newContext({ viewport: { width: 1000, height: 1000 }, ...opts });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => console.log("pageerror:", e.message.slice(0, 300)));
  return { browser, ctx, page };
}
export const items = (page) => page.evaluate(() => JSON.parse(JSON.stringify(window.__items())));
export const centre = async (page, sel) => {
  const b = await page.locator(sel).boundingBox();
  return { x: b.x + b.width / 2, y: b.y + b.height / 2, b };
};
export async function ready(page, url) {
  await page.goto(url);
  await page.waitForSelector("[data-item]");
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(300);
}
export const round = (v, d = 1) => Math.round(v * 10 ** d) / 10 ** d;
import { PNG } from "pngjs";
/** Pixel difference of two PNG buffers (cropped to the common size): pixels whose largest channel differs by more than `tol`. */
export function diffPng(a, b, tol = 48, radius = 0) {
  const A = PNG.sync.read(a), B = PNG.sync.read(b);
  const w = Math.min(A.width, B.width), h = Math.min(A.height, B.height);
  let bad = 0, max = 0;
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) {
    const i = (y * A.width + x) * 4;
    // radius > 0: the pixel only counts when no pixel of B within `radius` px is close to it (glyph edges shift by a sub-pixel between screen and PDF raster)
    let d = 1e9;
    for (let dy = -radius; dy <= radius; dy++) for (let dx = -radius; dx <= radius; dx++) {
      const yy = y + dy, xx = x + dx;
      if (yy < 0 || xx < 0 || yy >= h || xx >= w) continue;
      const j = (yy * B.width + xx) * 4;
      d = Math.min(d, Math.max(Math.abs(A.data[i] - B.data[j]), Math.abs(A.data[i + 1] - B.data[j + 1]), Math.abs(A.data[i + 2] - B.data[j + 2])));
    }
    if (d > max) max = d;
    if (d > tol) bad++;
  }
  return { w, h, size_mismatch: [A.width - B.width, A.height - B.height], diff_pixels: bad, diff_pct: Math.round((bad / (w * h)) * 10000) / 100, max_channel_delta: max };
}
