// Phone note (375 px, touch): a swipe that starts on an item that is not selected scrolls the page; tap selects; a selected item is dragged without scrolling.
//   node scripts/phone.mjs -> results/phone.json
import { writeFileSync } from "node:fs";
import { BASE, centre, items, launch, ready, round } from "./lib.mjs";

const { browser, ctx, page } = await launch({ viewport: { width: 375, height: 667 }, hasTouch: true, isMobile: true });
await ready(page, `${BASE}/?view=editor&lib=inhouse&size=A5&unit=centre&n=100&scale=1.2`);
const cdp = await ctx.newCDPSession(page);
const touch = (type, x, y) => cdp.send("Input.dispatchTouchEvent", { type, touchPoints: type === "touchEnd" ? [] : [{ x, y, id: 1 }] });
const c = await centre(page, "#m22");
const res = {};
const pan = () => page.evaluate(() => Math.round(visualViewport.pageTop));
await page.evaluate(() => window.scrollTo(0, 0));
const p0 = await pan();
for (let i = 0; i <= 10; i++) { await touch(i ? "touchMove" : "touchStart", c.x, c.y - 12 * i); await page.waitForTimeout(16); }
await touch("touchEnd", c.x, c.y - 120);
await page.waitForTimeout(300);
res.swipe_on_unselected_item_pans_page_px = (await pan()) - p0;
await page.evaluate(() => window.scrollTo(0, 0)); await page.waitForTimeout(100);
const p1 = await pan();
const c2 = await centre(page, "#m22");
await touch("touchStart", c2.x, c2.y); await touch("touchEnd", c2.x, c2.y); await page.waitForTimeout(200);
res.tap_selects = await page.evaluate(() => !!document.querySelector("[data-handle=resize]"));
const b = (await items(page)).find((i) => i.id === "m22");
await touch("touchStart", c2.x, c2.y); for (let i = 1; i <= 8; i++) { await touch("touchMove", c2.x, c2.y + 8 * i); await page.waitForTimeout(16); } await touch("touchEnd", c2.x, c2.y + 64);
await page.waitForTimeout(200);
res.selected_item_dragged_design_px_y = round((await items(page)).find((i) => i.id === "m22").box.y - b.box.y);
res.page_panned_during_item_drag_px = (await pan()) - p1;
res.handle_size_css_px = round(await page.evaluate(() => document.querySelector("[data-handle=resize]").getBoundingClientRect().width));
res.note = "handles are 22 design px; at the phone's fit-to-width scale (about 0.4) they are 9 screen px, below the 44 px touch target: the phone editor needs larger handles (or move up/down buttons), as planned";
writeFileSync(new URL("../results/phone.json", import.meta.url), JSON.stringify(res, null, 1));
console.log(res);
await browser.close();
