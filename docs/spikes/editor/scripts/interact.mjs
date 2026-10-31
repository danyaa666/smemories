// Question 2: drag / resize / rotate / snap / touch / keyboard / frame time for the in-house layer and react-moveable on the REAL cover.
//   node scripts/interact.mjs   (needs `npm run dev` on :5391) -> results/interact.json
import { writeFileSync } from "node:fs";
import { BASE, centre, items, launch, ready, round } from "./lib.mjs";

const out = {};
for (const lib of ["inhouse", "moveable"]) {
  const r = (out[lib] = {});
  const { browser, page } = await launch();
  await ready(page, `${BASE}/?view=editor&lib=${lib}&size=A5&unit=centre&n=2`);
  const k = (await page.locator("[data-layer]").boundingBox()).width / 816; // screen px per design px
  r.screen_px_per_design_px = round(k, 4);

  // 1. drag by +60,+40 screen px: expect +60/k, +40/k design px
  const before = (await items(page)).find((i) => i.id === "t1");
  let c = await centre(page, "#t1");
  await page.mouse.click(c.x, c.y);
  await page.mouse.move(c.x, c.y);
  await page.mouse.down();
  await page.mouse.move(c.x + 30, c.y + 20, { steps: 5 });
  await page.mouse.move(c.x + 60, c.y + 40, { steps: 5 });
  await page.mouse.up();
  await page.waitForTimeout(100);
  const after = (await items(page)).find((i) => i.id === "t1");
  r.drag = { expected_dx: round(60 / k), expected_dy: round(40 / k), got_dx: round(after.box.x - before.box.x), got_dy: round(after.box.y - before.box.y) };

  // 2. resize with the corner handle by +40,+30 screen px
  c = await centre(page, "#s1");
  await page.mouse.click(c.x, c.y);
  const hs = lib === "inhouse" ? "[data-handle=resize]" : ".moveable-control.moveable-se";
  const rs = await page.locator(hs).first().boundingBox();
  const w0 = (await items(page)).find((i) => i.id === "s1").box.w;
  await page.mouse.move(rs.x + rs.width / 2, rs.y + rs.height / 2);
  await page.mouse.down();
  await page.mouse.move(rs.x + rs.width / 2 + 40, rs.y + rs.height / 2 + 30, { steps: 8 });
  await page.mouse.up();
  await page.waitForTimeout(100);
  const rz = (await items(page)).find((i) => i.id === "s1");
  r.resize_rotated_12deg_by_40x30_screen_px = { got_dw_design_px: round(rz.box.w - w0), note: "40 px along x is 58.4 design px; a rotation-aware resize projects the pointer on the item axes" };

  // 3. rotate: drag the rotate handle to the right of the item centre -> about 90 degrees (snap to 15)
  c = await centre(page, "#t1");
  await page.mouse.click(c.x, c.y);
  const rh = lib === "inhouse" ? "[data-handle=rotate]" : ".moveable-rotation .moveable-control, .moveable-rotation-control, .moveable-control.moveable-rotation-control";
  const rb = await page.locator(rh).first().boundingBox().catch(() => null);
  if (rb) {
    const cc = await centre(page, "#t1");
    await page.mouse.move(rb.x + rb.width / 2, rb.y + rb.height / 2);
    await page.mouse.down();
    await page.mouse.move(cc.x + 150, cc.y + 2, { steps: 10 });
    await page.mouse.up();
    await page.waitForTimeout(100);
    r.rotate = { got_deg: round((await items(page)).find((i) => i.id === "t1").r) };
  } else r.rotate = "handle not found";

  // 4. snap to the page centre: drag the sticker so its centre is about 3 design px from the page middle
  const s0 = (await items(page)).find((i) => i.id === "s1");
  c = await centre(page, "#s1");
  await page.mouse.click(c.x, c.y);
  const targetX = 816 / 2 - (s0.box.x + s0.box.w / 2) - 3; // design px to move
  await page.mouse.move(c.x, c.y);
  await page.mouse.down();
  await page.mouse.move(c.x + (targetX * k) / 2, c.y, { steps: 5 });
  await page.mouse.move(c.x + targetX * k, c.y, { steps: 5 });
  await page.mouse.up();
  await page.waitForTimeout(100);
  const s1 = (await items(page)).find((i) => i.id === "s1");
  r.snap_centre = { distance_from_page_middle_after: round(s1.box.x + s1.box.w / 2 - 408, 2), started_at: -3 };
  await browser.close();

  // 5. touch drag on a 768 px wide viewport (CDP touch events)
  const t = await launch({ viewport: { width: 768, height: 1024 }, hasTouch: true, isMobile: false });
  await ready(t.page, `${BASE}/?view=editor&lib=${lib}&size=A5&unit=centre&n=2`);
  const kt = (await t.page.locator("[data-layer]").boundingBox()).width / 816;
  const cdp = await t.ctx.newCDPSession(t.page);
  const touch = (type, x, y) => cdp.send("Input.dispatchTouchEvent", { type, touchPoints: type === "touchEnd" ? [] : [{ x, y, id: 1 }] });
  const tb = (await items(t.page)).find((i) => i.id === "s1");
  const tc = await centre(t.page, "#s1");
  await touch("touchStart", tc.x, tc.y); await touch("touchEnd", tc.x, tc.y); // tap = select
  await t.page.waitForTimeout(150);
  await touch("touchStart", tc.x, tc.y);
  for (let i = 1; i <= 10; i++) await touch("touchMove", tc.x - 5 * i, tc.y - 4 * i);
  await touch("touchEnd", tc.x - 50, tc.y - 40);
  await t.page.waitForTimeout(150);
  const ta = (await items(t.page)).find((i) => i.id === "s1");
  r.touch_768 = { expected_dx: round(-50 / kt), expected_dy: round(-40 / kt), got_dx: round(ta.box.x - tb.box.x), got_dy: round(ta.box.y - tb.box.y),
    page_scrolled: await t.page.evaluate(() => window.scrollY) };
  await t.browser.close();

  // 5b. the stage is drawn at a zoom other than 1 (the book fits its pages to the window with transform: scale): drag by +50 screen px at scale 0.6
  const z = await launch();
  await ready(z.page, `${BASE}/?view=editor&lib=${lib}&size=A5&unit=centre&n=2&scale=0.6`);
  const kz = (await z.page.locator("[data-layer]").boundingBox()).width / 816;
  const zb = (await items(z.page)).find((i) => i.id === "s1");
  const zc = await centre(z.page, "#s1");
  await z.page.mouse.click(zc.x, zc.y);
  await z.page.mouse.move(zc.x, zc.y); await z.page.mouse.down(); await z.page.mouse.move(zc.x + 25, zc.y, { steps: 4 }); await z.page.mouse.move(zc.x + 50, zc.y, { steps: 4 }); await z.page.mouse.up();
  await z.page.waitForTimeout(100);
  r.stage_scale_0_6 = { expected_dx: round(50 / kz), got_dx: round((await items(z.page)).find((i) => i.id === "s1").box.x - zb.box.x) };
  await z.browser.close();

  // 6. keyboard (in-house only has it built in)
  const kb = await launch();
  await ready(kb.page, `${BASE}/?view=editor&lib=${lib}&size=A5&unit=centre&n=2`);
  const k0 = (await items(kb.page)).find((i) => i.id === "s1");
  await kb.page.focus("#s1").catch(() => {});
  await kb.page.keyboard.press("ArrowRight"); await kb.page.keyboard.press("Shift+ArrowDown");
  const k1 = (await items(kb.page)).find((i) => i.id === "s1");
  r.keyboard = { focusable: await kb.page.evaluate(() => document.activeElement?.id === "s1"), dx: round(k1.box.x - k0.box.x), dy: round(k1.box.y - k0.box.y) };
  await kb.browser.close();

  // 7. frame rate while dragging one item among 100, Chrome 4x CPU throttle
  for (const n of [100, 300]) {
    const f = await launch();
    await ready(f.page, `${BASE}/?view=editor&lib=${lib}&size=A5&unit=centre&n=${n}`);
    const cdp2 = await f.ctx.newCDPSession(f.page);
    await cdp2.send("Emulation.setCPUThrottlingRate", { rate: 4 });
    const cc = await centre(f.page, "#m22");
    await f.page.evaluate(() => window.scrollTo(0, 0));
    await f.page.mouse.click(cc.x, cc.y);
    await f.page.evaluate(() => {
      window.__frames = []; window.__lat = [];
      addEventListener("mousemove", () => { const s0 = performance.now(); requestAnimationFrame(() => requestAnimationFrame(() => window.__lat.push(performance.now() - s0))); }, true);
      let last = performance.now();
      const tick = (t) => { window.__frames.push(t - last); last = t; if (!window.__stop) requestAnimationFrame(tick); };
      requestAnimationFrame(tick);
    });
    await f.page.mouse.move(cc.x, cc.y);
    await f.page.mouse.down();
    const t0 = Date.now();
    for (let i = 0; i < 240; i++) await f.page.mouse.move(cc.x + 60 * Math.sin(i / 20), cc.y + 40 * Math.cos(i / 15));
    const ms = Date.now() - t0;
    await f.page.mouse.up();
    await f.page.waitForTimeout(500);
    const lat = (await f.page.evaluate(() => { window.__stop = true; return window.__lat; })).sort((a, b) => a - b);
    const fr = await f.page.evaluate(() => window.__frames.slice(2));
    fr.sort((a, b) => a - b);
    const total = fr.reduce((a, b) => a + b, 0);
    r[`frames_${n}_items_4x_throttle`] = { drag_ms: ms, frames: fr.length, fps: round((fr.length / total) * 1000), median_ms: round(fr[fr.length >> 1]), p95_ms: round(fr[Math.floor(fr.length * 0.95)]), max_ms: round(fr.at(-1)), move_to_second_frame_median_ms: round(lat[lat.length >> 1]), move_to_second_frame_p95_ms: round(lat[Math.floor(lat.length * 0.95)]) };
    await f.browser.close();
  }
  console.log(lib, JSON.stringify(r));
}
writeFileSync(new URL("../results/interact.json", import.meta.url), JSON.stringify(out, null, 1));
