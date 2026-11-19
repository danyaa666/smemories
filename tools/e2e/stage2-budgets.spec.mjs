// The stage 2 performance budgets, measured on the real stack (T-183 requirement 3, docs/stage2-exit-report.md "Budgets", docs/e2e.md "The stage 2 budgets").
// It only runs when STAGE2_BUDGETS=1 (it takes about 4 minutes and its numbers depend on the machine), so `make e2e` skips it:
//   STAGE2_BUDGETS=1 scripts/e2e.sh stage2-budgets.spec.mjs --project=budgets
// and, for (d), raise the per-user layout limit so a 100-call series is not throttled by it: SMEM_RATE_LAYOUT_SAVE_PER_MINUTE=6000 (the burst of 20 per 10 s stays, so the series is paced at 2 calls a second).
// The numbers print as `T183-NUMBERS {...}` lines. The 100-item drag at 4x throttle (a) is measured by editor-gestures.spec.mjs ("T160-NUMBERS").
//   (b) print time of a 30-page book with layouts against the same book without: time to data-book-ready plus page.pdf(), median of 5 each, interleaved
//   (c) editor route: JS transferred (gzip of each script the route loads, cold cache) and time until the toolbar and the cover are drawn, 4x CPU throttle, median of 3
//   (d) book-layout/save with a ~126 KiB document, template-list and apply-template: 100 sequential calls each, p50 and p95 of the round trip on loopback
//   (e) the editor tab after 200 nudges: JS heap after a forced collection against the start, DOM nodes and event listeners (Memory.getDOMCounters)
// Credentials: the registered fixed email code (DEV-SHORTCUT(otp), SMEM_ENV=test only).
import { mkdtempSync } from "node:fs";
import { gzipSync } from "node:zlib";
import os from "node:os";
import path from "node:path";
import { expect, request, test } from "@playwright/test";
import { inspect, savePdf } from "../print-check/lib.mjs";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Võ Thị Mai Anh", email: `e2e-budget-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-7` };
const TITLE = `Lớp 11B2 – Ngân sách ${RUN}`;
const NOTES = 27; // memory-book: one friend per notes page, so cover + profile + 27 + back = 30 pages

const median = (a) => [...a].sort((x, y) => x - y)[Math.floor(a.length / 2)];
const pct = (a, p) => [...a].sort((x, y) => x - y)[Math.min(a.length - 1, Math.floor(a.length * p))];
const out = (k, v) => console.log(`T183-NUMBERS ${JSON.stringify({ [k]: v }, (_, x) => (typeof x === "number" ? Number(x.toFixed(2)) : x))}`);

/** A layout with `n` items on every page kind: half Vietnamese texts of `runes` runes, half stickers. */
function layoutOf(n, runes, textOnly = false) {
  const pages = {};
  for (const kind of ["cover", "profile", "notes", "back"]) {
    const items = [];
    for (let i = 0; i < n; i++) {
      const id = `${kind[0]}${String(i).padStart(5, "0")}`;
      const x = 50 + ((i * 97) % 700), cy = 80 + ((i * 53) % 840);
      if (textOnly || i % 2 === 0) items.push({ id, t: "text", text: "ế".repeat(runes), x, w: 120, h: 60, cy, size: 12 });
      else items.push({ id, t: "asset", asset: "sticker.doodle-squiggle", x, w: 90, h: 90, cy });
    }
    pages[kind] = { items };
  }
  return { v: 1, pages };
}

test.describe.configure({ mode: "serial" });

let bookId = "";
let owner;

test("setup: a Memory Book of 30 pages", async ({ browser }) => {
  test.skip(!process.env.STAGE2_BUDGETS, "the budgets run only with STAGE2_BUDGETS=1");
  test.skip(!BASE || !OTP, "run through scripts/e2e.sh");
  test.setTimeout(300_000);
  owner = await (await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } })).newPage();
  const friend = await (await browser.newContext({ locale: "en-US", baseURL: BASE })).newPage();
  await owner.goto("/register");
  await owner.getByLabel("Your name").fill(OWNER.name);
  await owner.getByLabel("Email").fill(OWNER.email);
  await owner.getByLabel("Password", { exact: true }).fill(OWNER.password);
  await owner.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
  await owner.getByRole("button", { name: "Create account" }).click();
  await owner.getByLabel("6-digit code").fill(OTP);
  await expect(owner.getByText("Your email address is verified.")).toBeVisible();
  await owner.goto("/yearbooks/new");
  await owner.getByLabel("Title").fill(TITLE);
  await owner.getByLabel("Yearbook language").selectOption("vi");
  await owner.getByLabel("Page size").selectOption("A5");
  await owner.getByRole("button", { name: "Create yearbook" }).click();
  await expect(owner.getByRole("heading", { level: 1, name: TITLE })).toBeVisible();
  bookId = new URL(owner.url()).pathname.split("/").pop();
  expect((await owner.request.post("/api/yearbook/update", { data: { id: bookId, template_id: "memory-book" }, headers: { Origin: BASE } })).ok()).toBe(true);
  await owner.goto(`/yearbooks/${bookId}/note-links`);
  await owner.getByLabel("Name (optional)").fill("Budget");
  await owner.getByRole("button", { name: "Create link" }).click();
  const link = new URL(await owner.getByLabel("Link for Budget").inputValue()).pathname;
  for (let i = 1; i <= NOTES; i++) {
    await friend.goto(link);
    await friend.getByLabel(/^Your name/).fill(`Bạn số ${i}`);
    await friend.getByLabel(/^Your message/).fill(`Chúc bạn luôn vui! (${i})`);
    const sent = friend.waitForResponse((r) => r.url().includes("/api/public-collect/submit-note") && r.ok());
    await friend.getByRole("button", { name: "Send my note" }).click();
    await sent;
  }
  await owner.goto(`/yearbooks/${bookId}/notes`);
  await expect(owner.getByText(`${NOTES} waiting for you · 0 in your book`)).toBeVisible();
  for (let i = 1; i <= NOTES; i++) await owner.getByRole("button", { name: `Approve Bạn số ${i}`, exact: true }).click();
  await owner.getByRole("button", { name: /^Approved/ }).click();
  await expect(owner.locator(".inbox-card")).toHaveCount(NOTES);
});

const post = async (url, data) => {
  for (let i = 0; ; i++) {
    const res = await owner.request.post(url, { data, headers: { Origin: BASE } });
    if (res.status() !== 429 || i >= 10) return res;
    await owner.waitForTimeout(Math.max(1, Number(res.headers()["retry-after"] ?? 2)) * 1000);
  }
};
const version = async () => (await (await owner.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json()).data.version;

for (const [name, perKind] of [["max", 30], ["typical", 8]])
test(`(b) print time of a 30-page book with layouts (${name}: ${perKind} items on each page kind) against the same book without`, async ({ browser }) => {
  test.skip(!bookId, "needs the setup test");
  test.setTimeout(600_000);
  const doc = layoutOf(perKind, 12); // "max": 30 items on each of the four page kinds (the book allows 40 a page and 150 in all)
  const measure = async () => {
    const ctx = await browser.newContext({ locale: "en-US", baseURL: BASE });
    const p = await ctx.newPage();
    const login = await p.request.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password }, headers: { Origin: BASE } });
    expect(login.ok()).toBe(true);
    const t0 = Date.now();
    await p.goto(`/yearbooks/${bookId}/book`);
    await p.waitForFunction(() => document.documentElement.dataset.bookReady, null, { timeout: 120_000 });
    const readyMs = Date.now() - t0;
    const file = path.join(mkdtempSync(path.join(os.tmpdir(), "e2e-budget-")), "book.pdf");
    const { pdfMs } = await savePdf(p, file);
    const info = inspect(file);
    const items = await p.locator("[data-layout-item]").count();
    await ctx.close();
    return { readyMs, pdfMs, pages: info.pages, items };
  };
  const withL = [], without = [];
  let warm = true;
  for (let i = 0; i < 6; i++) {
    // alternate so that a warm cache or a busy machine favours neither variant; the first pair is a warm-up and is not counted
    const first = i % 2 === 0;
    for (const useLayout of first ? [true, false] : [false, true]) {
      if (useLayout) {
        const r = await post("/api/book-layout/save", { yearbook_id: bookId, base_version: await version(), layout: doc });
        expect(r.ok(), `save ${r.status()} ${(await r.text()).slice(0, 200)}`).toBe(true);
      } else {
        const r = await post("/api/book-layout/reset", { yearbook_id: bookId });
        expect(r.ok(), `reset ${r.status()}`).toBe(true);
      }
      const m = await measure();
      expect(m.pages).toBe(30);
      expect(m.items > 0).toBe(useLayout);
      if (!warm) (useLayout ? withL : without).push(m);
    }
    warm = false;
  }
  const sum = (a) => a.map((m) => m.readyMs + m.pdfMs);
  const mw = median(sum(withL)), mo = median(sum(without));
  out(`b_print_${name}`, { runs: withL.length, withLayout: { ms: sum(withL), median: mw, readyMedian: median(withL.map((m) => m.readyMs)), pdfMedian: median(withL.map((m) => m.pdfMs)), items: withL[0].items }, without: { ms: sum(without), median: mo, readyMedian: median(without.map((m) => m.readyMs)), pdfMedian: median(without.map((m) => m.pdfMs)) }, ratio: mw / mo });
  await post("/api/book-layout/reset", { yearbook_id: bookId });
});

test("(c) editor route: JS transferred and time to a drawn editor at 4x CPU throttle", async ({ browser }) => {
  test.skip(!bookId, "needs the setup test");
  test.setTimeout(300_000);
  const runs = [];
  for (let i = 0; i < 3; i++) {
    const ctx = await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } });
    const p = await ctx.newPage();
    const login = await p.request.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password }, headers: { Origin: BASE } });
    expect(login.ok()).toBe(true);
    const cdp = await ctx.newCDPSession(p);
    await cdp.send("Emulation.setCPUThrottlingRate", { rate: 4 });
    const scripts = new Map();
    p.on("response", (r) => { if (/\.js(\?|$)/.test(r.url()) && r.status() === 200) scripts.set(r.url(), r); });
    const t0 = Date.now();
    await p.goto(`/yearbooks/${bookId}/editor`);
    await expect(p.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible({ timeout: 60_000 });
    await expect(p.locator(".ed-pagebox .bk-sheet[data-page='cover']")).toBeVisible({ timeout: 60_000 });
    await p.evaluate(() => document.fonts.ready);
    const drawnMs = Date.now() - t0;
    let raw = 0, gz = 0;
    for (const r of scripts.values()) {
      const body = await r.body();
      raw += body.length;
      gz += gzipSync(body, { level: 9 }).length;
    }
    runs.push({ drawnMs, scripts: scripts.size, rawKB: raw / 1024, gzipKB: gz / 1024 });
    await ctx.close();
  }
  out("c_editor_load", { runs: runs.map((r) => r.drawnMs), medianMs: median(runs.map((r) => r.drawnMs)), scripts: runs[0].scripts, rawKB: runs[0].rawKB, gzipKB: runs[0].gzipKB });
});

test("(d) book-layout/save (about 126 KiB), template-list and apply-template: 100 sequential calls each", async () => {
  test.skip(!bookId, "needs the setup test");
  test.setTimeout(600_000);
  const big = layoutOf(30, 320, true); // 120 text items of 320 runes of 3 bytes
  const size = Buffer.byteLength(JSON.stringify(big));
  expect(size, `document size ${size}`).toBeGreaterThan(120 * 1024);
  expect(size).toBeLessThan(128 * 1024);
  const pace = () => owner.waitForTimeout(560); // the burst limit is 20 per 10 s: 2 calls a second at most
  const timed = async (fn) => {
    const t0 = performance.now();
    const r = await fn();
    return { ms: performance.now() - t0, status: r.status() };
  };
  const stats = (rows) => {
    const bad = rows.filter((r) => r.status !== 200);
    const ms = rows.filter((r) => r.status === 200).map((r) => r.ms);
    return { calls: rows.length, notOk: bad.length, p50: pct(ms, 0.5), p95: pct(ms, 0.95), max: Math.max(...ms) };
  };
  let v = await version();
  const saves = [];
  for (let i = 0; i < 100; i++) {
    const t = await timed(async () => {
      const r = await owner.request.post("/api/book-layout/save", { data: { yearbook_id: bookId, base_version: v, layout: i % 2 ? big : layoutOf(30, 319, true) }, headers: { Origin: BASE } });
      if (r.status() === 200) v = (await r.json()).data.version;
      return r;
    });
    saves.push(t);
    await pace();
  }
  out("d_save_128KiB", { bytes: size, ...stats(saves) });
  const lists = [];
  for (let i = 0; i < 100; i++) {
    lists.push(await timed(() => owner.request.post("/api/book-layout/template-list", { data: { yearbook_id: bookId, kind: "cover" }, headers: { Origin: BASE } })));
    await pace();
  }
  const listed = (await (await owner.request.post("/api/book-layout/template-list", { data: { yearbook_id: bookId, kind: "cover" }, headers: { Origin: BASE } })).json()).data;
  out("d_template_list", { shipped: (listed.items ?? []).filter((x) => x.source === "shipped").length, ...stats(lists) });
  const shipped = (listed.items ?? []).find((x) => x.source === "shipped");
  expect(shipped, "the book has a shipped cover template").toBeTruthy();
  const applies = [];
  v = await version();
  for (let i = 0; i < 100; i++) {
    applies.push(await timed(async () => {
      const r = await owner.request.post("/api/book-layout/apply-template", { data: { yearbook_id: bookId, base_version: v, kind: "cover", source: "shipped", template_id: shipped.id }, headers: { Origin: BASE } });
      if (r.status() === 200) v = (await r.json()).data.version;
      return r;
    }));
    await pace();
  }
  out("d_apply_template", stats(applies));
  await post("/api/book-layout/reset", { yearbook_id: bookId });
});

test("(e) the editor tab after 200 nudges: heap, DOM nodes, event listeners", async ({ browser }) => {
  test.skip(!bookId, "needs the setup test");
  test.setTimeout(300_000);
  const ctx = await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } });
  const p = await ctx.newPage();
  const login = await p.request.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password }, headers: { Origin: BASE } });
  expect(login.ok()).toBe(true);
  await p.goto(`/yearbooks/${bookId}/editor`);
  await expect(p.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
  await expect(p.locator(".ed-pagebox .bk-sheet[data-page='cover']")).toBeVisible();
  await p.evaluate(() => document.fonts.ready);
  const cdp = await ctx.newCDPSession(p);
  await cdp.send("HeapProfiler.enable");
  await cdp.send("Memory.getDOMCounters");
  const snap = async () => {
    await cdp.send("HeapProfiler.collectGarbage");
    await cdp.send("HeapProfiler.collectGarbage");
    const heap = await cdp.send("Runtime.getHeapUsage");
    const dom = await cdp.send("Memory.getDOMCounters");
    return { heapMB: heap.usedSize / 1048576, nodes: dom.nodes, listeners: dom.jsEventListeners };
  };
  await p.getByRole("button", { name: "Cover title", exact: true }).click();
  const warmUp = async (n) => { for (let i = 0; i < n; i++) await p.keyboard.press(i % 2 ? "ArrowLeft" : "ArrowRight"); };
  await warmUp(20); // the first moves load the lazy parts of the editor; the baseline is taken after them
  await p.waitForTimeout(2500);
  const before = await snap();
  await warmUp(200);
  await p.waitForTimeout(2500); // the autosave settles
  const after = await snap();
  out("e_200_nudges", { before, after, heapGrowthPct: ((after.heapMB - before.heapMB) / before.heapMB) * 100, nodesGrowth: after.nodes - before.nodes, listenersGrowth: after.listeners - before.listeners });
  await ctx.close();
});

test.afterAll(async () => {
  if (!BASE || !bookId) return;
  const api = await request.newContext({ baseURL: BASE, extraHTTPHeaders: { Origin: BASE } });
  try {
    const login = await api.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password } });
    if (!login.ok()) return;
    const list = await (await api.get("/api/yearbook/get-list")).json();
    for (const y of list.data?.items ?? []) await api.post("/api/yearbook/delete", { data: { id: y.id } });
  } catch (e) {
    console.warn(`clean-up failed (the run's data stays in the test database): ${e.message}`);
  } finally {
    await api.dispose();
  }
});
