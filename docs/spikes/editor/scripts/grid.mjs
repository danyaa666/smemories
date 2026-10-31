// Question 1 picture: the real Memory Book cover with a sticker and a text box, four unit candidates (rows) on A5, A4 and Letter (columns).
//   node scripts/grid.mjs -> results/units-grid.png
import { BASE, launch } from "./lib.mjs";
const { browser, page } = await launch({ viewport: { width: 1000, height: 2000 } });
await page.goto(`${BASE}/?view=grid`);
await page.waitForSelector("[data-cell]");
await page.evaluate(() => document.fonts.ready);
await page.waitForTimeout(800);
await page.screenshot({ path: new URL("../results/units-grid.png", import.meta.url).pathname, fullPage: true });
await browser.close();
