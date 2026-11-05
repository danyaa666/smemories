// Question 4: the studio's failure modes against the built prototype (studio/dist, `npm run studio:bundle` builds it). Every row of the ADR table that says "proven" is a case here.
//   node scripts/failures.mjs -> results/failures.json
import { readFileSync, writeFileSync } from "node:fs";
import { chromium } from "playwright";
import { flattenSize } from "../studio/src/flatten.mjs";
import { serve } from "../src/serve.mjs";

const dist = new URL("../studio/dist/", import.meta.url).pathname.replace(/\/$/, "");
const srv = await serve(dist), browser = await chromium.launch();
const results = [];
const record = (name, pass, detail) => { results.push({ name, pass, detail }); console.log(pass ? "PASS" : "FAIL", name, JSON.stringify(detail)); };
const json = (route, status, body, headers = {}) => route.fulfill({ status, contentType: "application/json", headers, body: JSON.stringify(body) });
async function fresh({ mock, query = "", block } = {}) {
  const ctx = await browser.newContext({ viewport: { width: 900, height: 900 } }), page = await ctx.newPage();
  const uploads = []; page.on("pageerror", () => {});
  await page.route("**/api/media/create", (r) => { uploads.push(Date.now()); return mock ? mock(r) : json(r, 200, { status: "OK", data: { id: "01HZ" } }); });
  if (block) await page.route(block, (r) => r.abort());
  await page.goto(`${srv.base}/${query}`);
  return { ctx, page, uploads };
}
const typeInBook = async (page, s) => { await page.fill("#book-text", s); await page.waitForFunction((t) => window.__book?.text === t, s, { timeout: 3000 }); return page.evaluate(() => window.__book.saves); };
const bookOk = async (page, label) => { const before = await typeInBook(page, label + " A"); const after = await typeInBook(page, label + " B"); return after > before && (await page.inputValue("#book-text")) === label + " B"; };

// 1. chunk fails to load
{ const { ctx, page } = await fresh({ block: "**/assets/studio-*.js" });
  await typeInBook(page, "keep me"); await page.click("#open"); await page.waitForSelector("#studio-error");
  record("studio chunk fails to load -> one message, book untouched, autosave alive", (await page.inputValue("#book-text")) === "keep me" && (await bookOk(page, "after")), { message: await page.textContent("#studio-error") });
  await ctx.close(); }
// 2. crash mid-edit
{ const { ctx, page } = await fresh();
  await page.click("#open"); await page.waitForSelector("#save"); await page.evaluate(() => window.__studio.addRect()); await page.evaluate(() => window.__studio.crash());
  await page.waitForSelector("#studio-error");
  const draft = await page.evaluate(() => localStorage.getItem("studio-draft:BOOK"));
  record("studio crashes mid-edit -> boundary message, book untouched, draft kept in the browser", (await bookOk(page, "x")) && !!draft && /rect/i.test(draft), { draft_bytes: draft?.length });
  await ctx.close(); }
// 3. export failure (toBlob answers null, what a phone does when it cannot allocate)
{ const { ctx, page, uploads } = await fresh();
  await page.click("#open"); await page.waitForSelector("#save"); await page.evaluate(() => { window.__studio.addRect(); HTMLCanvasElement.prototype.toBlob = function (cb) { cb(null); }; });
  await page.click("#save"); await page.waitForFunction(() => document.querySelector("#msg").textContent.includes("export failed"));
  record("export cannot allocate (toBlob null) -> one message, no upload, draft kept", uploads.length === 0 && !!(await page.evaluate(() => localStorage.getItem("studio-draft:BOOK"))) && (await bookOk(page, "e")), { msg: await page.textContent("#msg") });
  await ctx.close(); }
// 4. upload outcomes
for (const [name, mock, expect, keepsDraft] of [
  ["server 500", (r) => json(r, 500, { status: "ERROR_INTERNAL" }), "upload failed", true],
  ["network drop", (r) => r.abort(), "upload failed", true],
  ["429 with Retry-After 7", (r) => json(r, 429, { status: "ERROR_RATE_LIMITED" }, { "Retry-After": "7" }), "retry in 7s", true],
  ["30-image cap (ERROR_DESIGN_IMAGE_LIMIT)", (r) => json(r, 409, { status: "ERROR_DESIGN_IMAGE_LIMIT", data: { limit: 30 } }), "limit (30)", true],
  ["quota (ERROR_QUOTA_EXCEEDED)", (r) => json(r, 409, { status: "ERROR_QUOTA_EXCEEDED" }), "quota", true],
  ["success", (r) => json(r, 200, { status: "OK", data: { id: "01HZ" } }), "saved", false],
]) {
  const { ctx, page, uploads } = await fresh({ mock });
  await page.click("#open"); await page.waitForSelector("#save"); await page.evaluate(() => window.__studio.addRect()); await page.click("#save");
  await page.waitForFunction((e) => document.querySelector("#msg").textContent.includes(e), expect, { timeout: 5000 });
  const draft = !!(await page.evaluate(() => localStorage.getItem("studio-draft:BOOK")));
  const disabled = await page.isDisabled("#save");
  record(`upload: ${name} -> "${await page.textContent("#msg")}"`, draft === keepsDraft && uploads.length === 1 && (name.startsWith("429") ? disabled : !disabled || name === "success"), { requests: uploads.length, draft_kept: draft, save_disabled: disabled });
  await ctx.close();
}
// 5. double click sends one upload
{ const { ctx, page, uploads } = await fresh({ mock: async (r) => { await new Promise((ok) => setTimeout(ok, 600)); return json(r, 200, { status: "OK", data: { id: "01HZ" } }); } });
  await page.click("#open"); await page.waitForSelector("#save"); await page.evaluate(() => window.__studio.addRect());
  await page.dblclick("#save"); await page.waitForFunction(() => document.querySelector("#msg").textContent === "saved");
  record("double click on save -> exactly one upload", uploads.length === 1, { requests: uploads.length }); await ctx.close(); }
// 6. leaving with unsaved work; the draft comes back
{ let { ctx, page } = await fresh();
  await page.click("#open"); await page.waitForSelector("#save"); await page.evaluate(() => window.__studio.addRect()); await page.mouse.click(5, 5);
  let dialog = null; page.on("dialog", async (d) => { dialog = d.type(); await d.accept(); });
  await page.click("#save", { trial: true }); // user activation: Chromium only shows the beforeunload prompt after a gesture on the page
  await page.close({ runBeforeUnload: true }); await new Promise((ok) => setTimeout(ok, 300));
  const asked = dialog === "beforeunload";
  page = await ctx.newPage(); await page.route("**/api/media/create", (r) => json(r, 200, { status: "OK", data: { id: "x" } })); await page.goto(`${srv.base}/`);
  await page.click("#open"); await page.waitForSelector("#save"); await page.waitForFunction(() => document.querySelector("#msg").textContent.includes("draft restored"));
  const n = await page.evaluate(() => window.__studio.canvas.getObjects().length);
  record("leave with unsaved studio work -> the browser asks (beforeunload), the draft is restored on reopening", asked && n === 1, { beforeunload_dialog: asked, objects_restored: n }); await ctx.close(); }
// 7. feature flag off: no button, no chunk request
{ const reqs = []; const ctx = await browser.newContext(), page = await ctx.newPage(); page.on("request", (r) => reqs.push(r.url()));
  await page.goto(`${srv.base}/?studio=off`); await page.waitForTimeout(300);
  record("flag off -> no studio button and the studio chunk is never requested", (await page.locator("#open").count()) === 0 && !reqs.some((u) => /studio-/.test(u)), { requests: reqs.length }); await ctx.close(); }
{ const reqs = []; const ctx = await browser.newContext(), page = await ctx.newPage(); page.on("request", (r) => reqs.push(r.url()));
  await page.goto(`${srv.base}/`); await page.waitForTimeout(300); const before = reqs.some((u) => /studio-/.test(u)); await page.click("#open"); await page.waitForSelector("#save");
  record("flag on, studio closed -> chunk not fetched until the button is pressed", !before && reqs.some((u) => /studio-/.test(u)), { fetched_before_click: before }); await ctx.close(); }
// 8. Fabric version bump: the working document survives toJSON -> loadFromJSON -> toJSON unchanged, and the version is pinned exactly
{ const { ctx, page } = await fresh();
  await page.click("#open"); await page.waitForSelector("#save");
  const r = await page.evaluate(async () => { const s = window.__studio; s.addRect(); s.addEllipse(); const a = JSON.stringify(s.canvas.toJSON()); await s.canvas.loadFromJSON(JSON.parse(a)); const b = JSON.stringify(s.canvas.toJSON()); return { same: a === b, version: JSON.parse(a).version, bytes: a.length }; });
  const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  record("Fabric document round-trips unchanged; version pinned exactly in package.json", r.same && /^\d+\.\d+\.\d+$/.test(pkg.dependencies.fabric) && r.version === pkg.dependencies.fabric, { fabric_json_version: r.version, pinned: pkg.dependencies.fabric, json_bytes: r.bytes }); await ctx.close(); }

// 9. sizes, time and bytes of a real flatten (A5 / A4 / Letter at the chosen size), plain and with the CPU 4x slower (a mid phone)
const sizes = {};
for (const [paper, mm] of [["A5", 148], ["A4", 210], ["Letter", 215.9]]) sizes[paper] = { flatten: flattenSize(paper, mm), uncapped_300dpi: (() => { const w = Math.round((mm / 25.4) * 300); return { w, mp: Math.round((w * Math.round(w * { A5: 210 / 148, A4: 297 / 210, Letter: 279.4 / 215.9 }[paper])) / 1e5) / 10 }; })() };
const timing = {};
for (const throttle of [1, 4]) {
  const ctx = await browser.newContext(), page = await ctx.newPage(); await page.goto(`${srv.base}/`); await page.click("#open"); await page.waitForSelector("#save");
  const cdp = await ctx.newCDPSession(page); await cdp.send("Emulation.setCPUThrottlingRate", { rate: throttle });
  for (const paper of ["A5", "A4", "Letter"]) {
    const { w, h } = sizes[paper].flatten;
    timing[`${paper}@cpu${throttle}x`] = await page.evaluate(async ({ w, h }) => {
      // a photo-like worst case: noise (PNG hates it) under a gradient, plus vector shapes on top
      const c = document.createElement("canvas"); c.width = w; c.height = h; const g = c.getContext("2d");
      let seed = 1; const rnd = () => (seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0) / 4294967296; const id = g.createImageData(w, h); for (let i = 0; i < id.data.length; i += 4) { const v = (rnd() * 40) | 0; id.data[i] = 120 + v + ((i / 4) % w) / w * 80; id.data[i + 1] = 100 + v; id.data[i + 2] = 140 + v; id.data[i + 3] = 255; } g.putImageData(id, 0, 0);
      const out = {};
      for (const [k, type, q] of [["png", "image/png"], ["jpeg85", "image/jpeg", 0.85], ["webp85", "image/webp", 0.85]]) { const t0 = performance.now(); const b = await new Promise((ok) => c.toBlob(ok, type, q)); out[k] = { ms: Math.round(performance.now() - t0), kib: Math.round(b.size / 1024), type: b.type }; }
      return out;
    }, { w, h });
  }
  await ctx.close();
}
record("flatten sizes, timings and bytes measured (see details)", true, { sizes, timing });
await browser.close(); srv.close();
const failed = results.filter((r) => !r.pass);
writeFileSync(new URL("../results/failures.json", import.meta.url), JSON.stringify({ chromium: browser.version?.() ?? "", passed: results.length - failed.length, failed: failed.length, results }, null, 1) + "\n");
console.log(`${results.length - failed.length} passed, ${failed.length} failed`);
if (failed.length) process.exit(1);
