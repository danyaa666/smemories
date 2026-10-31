// Question 2 follow-up: do the in-house gestures move an item by the DISPLAYED distance on every paper size and in every unit?
// Gestures edit the box as drawn on the edited page and write back the stored form (unplace). This drags on A5, A4 and Letter in all four units
// (snap off, so the distance is the pointer's; snapping is measured in interact.json) and compares the item's movement (layout px of the layer / design px) with the pointer's. Includes a drag that ends in the clamp zone.
//   node scripts/gestures.mjs (needs `npm run dev`) -> results/gestures.json
import { writeFileSync } from "node:fs";
import { BASE, launch, ready, round } from "./lib.mjs";

const H = { A5: 1158, A4: 1154, Letter: 1056 };
const box = (page, sel) => page.evaluate((s) => { const e = document.querySelector(s), c = getComputedStyle(e); return { left: parseFloat(c.left), top: parseFloat(c.top), w: parseFloat(c.width), h: parseFloat(c.height) }; }, sel);
const centreOf = async (page, sel) => { const b = await page.locator(sel).boundingBox(); return { x: b.x + b.width / 2, y: b.y + b.height / 2 }; };
async function drag(page, from, dx, dy) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + dx / 2, from.y + dy / 2, { steps: 6 });
  await page.mouse.move(from.x + dx, from.y + dy, { steps: 6 });
  await page.mouse.up();
  await page.waitForTimeout(80);
}
const out = {};
let worst = 0;
for (const size of ["A5", "A4", "Letter"]) for (const unit of ["pct", "mm", "centre", "anchor"]) {
  const { browser, page } = await launch({ viewport: { width: 1000, height: 1500 } });
  await ready(page, `${BASE}/?view=editor&lib=inhouse&size=${size}&unit=${unit}&n=2&snap=off`);
  const k = (await page.locator("[data-layer]").boundingBox()).width / 816; // screen px per design px
  const r = (out[`${unit}@${size}`] = {});
  // 1. text box: +50 right, 100 up, in screen px; the item must move the same displayed distance (error in screen px)
  const c1 = await centreOf(page, "#t1");
  const a = await box(page, "#t1");
  await page.mouse.click(c1.x, c1.y);
  await drag(page, c1, 50, -100);
  const b = await box(page, "#t1");
  r.drag_50_right_100_up = { item_dx_screen_px: round((b.left - a.left) * k, 2), item_dy_screen_px: round((b.top - a.top) * k, 2), error_screen_px: round(Math.max(Math.abs((b.left - a.left) * k - 50), Math.abs((b.top - a.top) * k + 100)), 2) };
  // 2. resize the sticker by the corner handle +40,+30 screen px: width grows by 40 screen px
  const c2 = await centreOf(page, "#s1");
  await page.mouse.click(c2.x, c2.y);
  const s0 = await box(page, "#s1");
  const hb = await page.locator("[data-handle=resize]").first().boundingBox();
  const hx = hb.x + hb.width / 2, hy = hb.y + hb.height / 2;
  // the handle sits on the item's corner; an item flush to the page edge puts it outside .bk-inner, which clips it (finding, ADR risk 8)
  const hit = await page.evaluate(([x, y]) => document.elementFromPoint(x, y)?.dataset?.handle === "resize", [hx, hy]);
  if (hit) {
    await drag(page, { x: hx, y: hy }, 40, 30);
    const s1 = await box(page, "#s1");
    r.resize_sticker_40_screen_px = { width_growth_screen_px: round((s1.w - s0.w) * k, 2), aspect_kept: round(s1.w / s1.h - s0.w / s0.h, 4) };
  } else r.resize_sticker_40_screen_px = { handle_clipped_by_page: true };
  // 3. clamp: drag the sticker 400 screen px down (far past the bottom edge); it must stop flush at the bottom edge, then 100 px up must move it exactly 100 screen px
  const c3 = await centreOf(page, "#s1");
  await drag(page, c3, 0, 400);
  const d = await box(page, "#s1");
  const c4 = await centreOf(page, "#s1");
  await drag(page, c4, 0, -100);
  const e = await box(page, "#s1");
  r.clamp_zone = { bottom_after_drag_past_edge_design_px: d.top + d.h, page_height: H[size], flush: Math.abs(d.top + d.h - H[size]) < 0.05, then_100_screen_px_up_moved: round((d.top - e.top) * k, 2) };
  // 4. keyboard: ArrowRight = 1 design px, Shift+ArrowDown = 10 design px
  const k0 = await box(page, "#t1");
  await page.focus("#t1");
  await page.keyboard.press("ArrowRight"); await page.keyboard.press("Shift+ArrowDown");
  const k1 = await box(page, "#t1");
  r.keyboard = { dx_design_px: round(k1.left - k0.left, 2), dy_design_px: round(k1.top - k0.top, 2) };
  // 5. a second pointer (another finger, pointerId 7) during a gesture is ignored: no move from its events, no new gesture, no deselect
  if (size === "Letter" && unit === "centre") {
    const c5 = await centreOf(page, "#t1");
    await page.mouse.click(c5.x, c5.y);
    const p0 = await box(page, "#t1");
    await page.mouse.move(c5.x, c5.y); await page.mouse.down();
    await page.mouse.move(c5.x + 10, c5.y, { steps: 2 });
    await page.evaluate(([x, y]) => {
      window.dispatchEvent(new PointerEvent("pointermove", { pointerId: 7, clientX: x + 300, clientY: y + 300, bubbles: true }));
      const f = (t, el) => el.dispatchEvent(new PointerEvent(t, { pointerId: 7, isPrimary: false, pointerType: "touch", clientX: x, clientY: y, bubbles: true }));
      f("pointerdown", document.querySelector("#s1")); f("pointerdown", document.querySelector("[data-layer]")); f("pointerup", document.body);
      window.dispatchEvent(new PointerEvent("pointerup", { pointerId: 7, bubbles: true }));
    }, [c5.x, c5.y]);
    const p1 = await box(page, "#t1");
    await page.mouse.move(c5.x + 20, c5.y, { steps: 2 });
    const p2 = await box(page, "#t1");
    await page.mouse.up();
    r.second_pointer = { moved_by_other_pointer_design_px: round(p1.left - p0.left - 10 / k, 2), still_selected: await page.locator("#t1 [data-handle=resize]").count() === 1, main_pointer_still_drags_screen_px: round((p2.left - p0.left) * k, 2) };
  }
  worst = Math.max(worst, r.drag_50_right_100_up.error_screen_px, Math.abs(r.clamp_zone.then_100_screen_px_up_moved - 100));
  await browser.close();
}
out.worst_pointer_error_screen_px = worst;
console.log(JSON.stringify(out));
writeFileSync(new URL("../results/gestures.json", import.meta.url), JSON.stringify(out, null, 1));
