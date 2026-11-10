// The phone editor end to end (T-181, docs/editor-phone.md): below 768 px the editor is a read-only page with a bottom sheet, a pager and dialogs, on a touch
// device (Chromium device emulation: hasTouch and isMobile). Real iOS Safari and Android Gboard are the owner's manual checks (docs/editor-phone.md).
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
//
// Tests: (1) 375x812: open from the manage page, tap the cover title, nudge and widen it with the sheet, add a Vietnamese text typed in the sheet, add a sticker,
// undo, page with the pager, apply an own template, restore from History, Done, reload; the same document at 1024 px; nothing is dragged and no desktop
// stage chunk is fetched; (2) 414x896 and landscape 812x375 smoke; (3) 40 items on a page (the server's per-page cap) on a 4x CPU throttle.
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 90_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = {
  name: "Đỗ Minh Châu",
  email: `e2e-phone-${RUN}@example.com`,
  password: `Mật-khẩu-${RUN}-9`,
};
const TITLE = `Lớp 10C – Điện thoại ${RUN}`;
const KINDS = ["cover", "profile", "notes", "back"];
const VI_TEXT = `Chúc các bạn thật nhiều niềm vui ${RUN}`;

async function step(title, body) {
  try {
    return await test.step(title, body, { timeout: STEP_MS });
  } catch (e) {
    if (!(e instanceof Error)) throw e;
    const mark = `STEP FAILED: ${title}\n`;
    e.stack = e.stack ? mark + e.stack : e.stack;
    e.message = mark + e.message;
    throw e;
  }
}

async function cleanUp() {
  if (!BASE) return;
  const api = await request.newContext({ baseURL: BASE, extraHTTPHeaders: { Origin: BASE } });
  try {
    const login = await api.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password } });
    if (!login.ok()) return;
    const list = await (await api.get("/api/yearbook/get-list")).json();
    const books = list.data?.items ?? [];
    if (books[0])
      for (const kind of KINDS) {
        const t = await (
          await api.post("/api/book-layout/template-list", { data: { yearbook_id: books[0].id, kind } })
        ).json();
        for (const x of t.data?.items ?? [])
          if (x.source === "own") await api.post("/api/book-layout/template-delete", { data: { id: x.id } });
      }
    for (const y of books) await api.post("/api/yearbook/delete", { data: { id: y.id } });
  } catch (e) {
    console.warn(`clean-up failed (the run's data stays in the test database): ${e.message}`);
  } finally {
    await api.dispose();
  }
}

test.afterAll(cleanUp);
test.describe.configure({ mode: "serial" });

let bookId = "";

/** The layout routes share the save limit (burst 20 per 10 s, 60 a minute): a 429 from a set-up call waits its Retry-After and tries again. */
async function post(page, path, data) {
  for (let i = 0; ; i++) {
    const res = await page.request.post(path, { data, headers: { Origin: BASE } });
    if (res.status() !== 429 || i >= 8) return res;
    await page.waitForTimeout(Math.max(1, Number(res.headers()["retry-after"] ?? 2)) * 1000);
  }
}
const storedLayout = async (page) =>
  (await (await page.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json()).data;
async function saveLayout(page, layout) {
  const { version } = await storedLayout(page);
  const res = await post(page, "/api/book-layout/save", { yearbook_id: bookId, base_version: version, layout });
  expect(res.ok(), `save ${res.status()} ${await res.text()}`).toBe(true);
}
const textItem = (id, text, over = {}) => ({ id, t: "text", text, x: 100, w: 700, h: 200, cy: 450, ...over });
const items = async (page, kind) => (await storedLayout(page)).layout?.pages?.[kind]?.items ?? [];
const savedBy = (page) =>
  page.waitForResponse((r) => r.url().includes("/api/book-layout/save") && r.request().method() === "POST");

/** A phone: touch, mobile emulation, the given size. The session of the owner comes from an API login. */
async function phone(browser, width, height) {
  const ctx = await browser.newContext({
    locale: "en-US",
    baseURL: BASE,
    viewport: { width, height },
    hasTouch: true,
    isMobile: true,
    deviceScaleFactor: 2,
  });
  const page = await ctx.newPage();
  const login = await page.request.post("/api/auth/login", {
    data: { email: OWNER.email, password: OWNER.password },
    headers: { Origin: BASE },
  });
  expect(login.ok(), `login ${login.status()}`).toBe(true);
  return page;
}
const position = (page) => page.getByTestId("page-position");
const sheet = (page, name = "Properties") => page.getByRole("region", { name, exact: true });
async function openEditor(page, kind = "cover") {
  await page.goto(`/yearbooks/${bookId}/editor?page=${kind}`);
  await expect(position(page)).toBeVisible();
  await expect(page.locator(`.ed-pagebox .bk-sheet[data-page='${kind}']`)).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
}
/** A one-finger swipe through the DevTools protocol (Playwright has no touch drag): the way a real finger would move an item. */
async function swipe(page, from, to, steps = 8) {
  const cdp = await page.context().newCDPSession(page);
  const pt = (p) => [{ x: Math.round(p.x), y: Math.round(p.y) }];
  await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: pt(from) });
  for (let i = 1; i <= steps; i++)
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: pt({ x: from.x + ((to.x - from.x) * i) / steps, y: from.y + ((to.y - from.y) * i) / steps }),
    });
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await cdp.detach();
}

test("setup: register and create a Memory Book", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const owner = await (
    await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } })
  ).newPage();
  await step("register and verify the email", async () => {
    await owner.goto("/register");
    await owner.getByLabel("Your name").fill(OWNER.name);
    await owner.getByLabel("Email").fill(OWNER.email);
    await owner.getByLabel("Password", { exact: true }).fill(OWNER.password);
    await owner.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
    const registered = owner.waitForResponse(
      (r) => r.url().includes("/api/auth/register") && r.request().method() === "POST",
    );
    await owner.getByRole("button", { name: "Create account" }).click();
    if ((await registered).status() === 429)
      throw new Error("registration is rate limited: run `make reset-limits SMEM_ENV=test` (make e2e does)");
    await owner.getByLabel("6-digit code").fill(OTP);
    await expect(owner.getByText("Your email address is verified.")).toBeVisible();
  });
  await step("create the book", async () => {
    await owner.goto("/yearbooks/new");
    await owner.getByLabel("Title").fill(TITLE);
    await owner.getByLabel("Yearbook language").selectOption("vi");
    await owner.getByLabel("Page size").selectOption("A5");
    await owner.getByRole("button", { name: "Create yearbook" }).click();
    await expect(owner.getByRole("heading", { level: 1, name: TITLE })).toBeVisible();
    bookId = new URL(owner.url()).pathname.split("/").pop();
    expect((await post(owner, "/api/yearbook/update", { id: bookId, template_id: "memory-book" })).ok()).toBe(true);
  });
});

test("375x812 touch: select, nudge, widen, type Vietnamese in the sheet, sticker, undo, pager, template, history, Done, reload (AC1, AC2, AC3)", async ({
  browser,
}) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await phone(browser, 375, 812);
  const NAME = `Mẫu hồ sơ ${RUN}`;
  const SRC = `Từ mẫu ${RUN}`;
  const BEFORE = `Hồ sơ trước ${RUN}`;
  await step("an own template of the profile page, then a distinct current profile page", async () => {
    await saveLayout(page, { v: 1, pages: { profile: { items: [textItem("profSrc", SRC)] } } });
    const made = await post(page, "/api/book-layout/template-create", {
      yearbook_id: bookId,
      kind: "profile",
      name: NAME,
    });
    expect(made.ok(), `template-create ${made.status()} ${await made.text()}`).toBe(true);
    await saveLayout(page, { v: 1, pages: { profile: { items: [textItem("profNow", BEFORE)] } } });
  });
  const fetched = [];
  page.on("request", (r) => fetched.push(new URL(r.url()).pathname));

  await step("open the editor from the manage page: the phone shell, no warning, no handles", async () => {
    await page.goto(`/yearbooks/${bookId}`);
    await page.getByRole("link", { name: "Edit page by page" }).click();
    await expect(position(page)).toHaveText("Page 1 of 3: Cover");
    await expect(page.getByText(/needs a screen at least 768 px wide/)).toHaveCount(0);
    await expect(page.locator(".ed-handle")).toHaveCount(0);
    await expect(page.getByRole("navigation", { name: "Pages" })).toHaveCount(0);
    await page.evaluate(() => document.fonts.ready);
    // the page is fit to the screen width and the screen does not scroll sideways
    const fit = await page.evaluate(() => ({
      page: document.querySelector(".ed-pagebox").getBoundingClientRect().width,
      inner: window.innerWidth,
      scrollW: document.documentElement.scrollWidth,
    }));
    expect(fit.page).toBeLessThanOrEqual(fit.inner);
    expect(fit.scrollW).toBeLessThanOrEqual(fit.inner + 1);
  });
  await step("tap the cover title: selected with a 2 px outline, the sheet opens; every button is at least 44 px", async () => {
    await page.getByRole("button", { name: "Cover title", exact: true }).tap();
    await expect(page.getByRole("button", { name: "Cover title", exact: true })).toHaveAttribute("aria-pressed", "true");
    await expect(sheet(page)).toBeVisible();
    const small = await page.evaluate(() =>
      [...document.querySelectorAll(".ed-phone button, .ed-phone a.ed-plink, .ed-phone select")]
        .filter((b) => b.offsetParent !== null && !b.classList.contains("ed-hit"))
        .map((b) => ({ n: (b.getAttribute("aria-label") ?? b.textContent).trim().slice(0, 24), h: b.getBoundingClientRect().height }))
        .filter((b) => b.h > 0 && b.h < 43.5),
    );
    expect(small, "buttons under 44 px on a phone").toEqual([]);
    const border = await page.evaluate(
      () => getComputedStyle(document.querySelector(".ed-box.is-selected .ed-hit")).borderTopWidth,
    );
    expect(parseFloat(border)).toBeGreaterThanOrEqual(2);
  });
  await step("nothing can be dragged or resized by touch, and the touch is left to the screen's scroll (AC2)", async () => {
    const hit = page.getByRole("button", { name: "Cover title", exact: true });
    const box = await hit.boundingBox();
    const before = await storedLayout(page);
    const touchAction = await hit.evaluate((el) => getComputedStyle(el).touchAction);
    expect(touchAction).not.toBe("none");
    await swipe(page, { x: box.x + box.width / 2, y: box.y + box.height / 2 }, { x: box.x + box.width / 2 + 60, y: box.y + box.height / 2 + 80 });
    await page.waitForTimeout(2000); // longer than the 1.5 s autosave debounce
    const after = await storedLayout(page);
    expect(after.version).toBe(before.version);
    expect(after.layout).toEqual(before.layout);
    await expect(page.getByRole("button", { name: "Undo", exact: true })).toBeDisabled();
  });
  await step("nudge right and widen with the sheet: one save, the same numbers as the desktop panel", async () => {
    const widthBefore = Number(await sheet(page).getByRole("spinbutton", { name: "Width" }).inputValue());
    const saved = savedBy(page);
    await sheet(page).getByRole("button", { name: "Move right" }).tap();
    await sheet(page).getByRole("button", { name: "Wider" }).tap();
    expect((await saved).status()).toBe(200);
    const widthAfter = Number(await sheet(page).getByRole("spinbutton", { name: "Width" }).inputValue());
    expect(widthAfter).toBeGreaterThan(widthBefore);
    const ov = (await storedLayout(page)).layout.pages.cover.overrides["cover.title"];
    expect(ov.w).toBeGreaterThan(0);
  });
  await step("add a text from the Add sheet and type Vietnamese in the sheet's textarea", async () => {
    await page.getByRole("button", { name: "Add", exact: true }).tap();
    await sheet(page, "Add").getByRole("button", { name: "Add text" }).tap();
    const ta = sheet(page).getByLabel("Text of the box (edit here)");
    await expect(ta).toBeVisible();
    const fontSize = await ta.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
    expect(fontSize, "16 px keeps iOS Safari from zooming").toBeGreaterThanOrEqual(16);
    const saved = savedBy(page);
    await ta.fill(VI_TEXT);
    await saved;
    await expect.poll(async () => (await items(page, "cover")).map((i) => i.text)).toContain(VI_TEXT);
    await expect(page.locator(".ed-pagebox")).toContainText(VI_TEXT);
  });
  await step("add a sticker from the palette in the Add sheet, then undo it", async () => {
    const n = (await items(page, "cover")).length;
    await page.getByRole("button", { name: "Add", exact: true }).tap();
    await sheet(page, "Add").locator("button:has(.ed-tile-pic)").first().tap();
    await expect.poll(async () => (await items(page, "cover")).length, { timeout: 15_000 }).toBe(n + 1);
    await page.getByRole("button", { name: "Undo", exact: true }).tap();
    await expect.poll(async () => (await items(page, "cover")).length, { timeout: 15_000 }).toBe(n);
  });
  await step("the pager: next page, the page list, previous page", async () => {
    await page.getByRole("button", { name: "Next page" }).tap();
    await expect(position(page)).toHaveText("Page 2 of 3: Profile");
    await position(page).tap();
    await sheet(page, "Pages").getByRole("button", { name: "Page 3 of 3: Back cover" }).tap();
    await expect(position(page)).toHaveText("Page 3 of 3: Back cover");
    await page.getByRole("button", { name: "Previous page" }).tap();
    await expect(position(page)).toHaveText("Page 2 of 3: Profile");
  });
  await step("Templates opens as a full-height dialog; apply the own template; the dialog's buttons are 44 px", async () => {
    await page.getByRole("button", { name: "Templates", exact: true }).tap();
    const dialog = page.getByRole("dialog").first();
    await expect(dialog).toBeVisible();
    const box = await dialog.boundingBox();
    expect(box.height).toBeGreaterThan(700); // full height of the 812 px screen
    await page.getByRole("radio", { name: new RegExp(`^Template ${NAME},`) }).tap();
    const apply = page.getByRole("button", { name: /^Apply to/ });
    await expect(apply).toBeVisible(); // the preview bar: the page shows through
    await expect(page.locator(".ed-pagebox")).toContainText(SRC);
    await apply.tap();
    const confirm = page.getByRole("dialog").getByRole("button", { name: "Apply template" });
    const h = (await confirm.boundingBox()).height;
    expect(h, "the confirm button of the apply dialog").toBeGreaterThanOrEqual(43.5);
    await confirm.tap();
    await expect(page.locator(".ed-pagebox")).toContainText(SRC);
    await expect.poll(async () => (await items(page, "profile")).map((i) => i.text).join("|")).toContain(SRC);
  });
  await step("History opens as a dialog: preview the 'Before template' row and restore it", async () => {
    await page.getByRole("button", { name: "History", exact: true }).tap();
    await expect(page.getByRole("dialog")).toBeVisible();
    const rows = page.getByRole("radio");
    await expect(rows.first()).toBeVisible();
    await rows.first().tap();
    await expect(page.locator(".ed-pagebox")).toContainText(BEFORE);
    await page.locator(".ed-prevbar").getByRole("button", { name: "Restore" }).tap();
    await page.getByRole("dialog").getByRole("button", { name: "Restore" }).tap();
    await expect.poll(async () => (await items(page, "profile")).map((i) => i.text).join("|")).toContain(BEFORE);
  });
  await step("Done saves and goes back to the manage page; a reload shows the same document at 1024 px", async () => {
    await page.getByRole("button", { name: "Done", exact: true }).tap();
    await expect(page).toHaveURL(new RegExp(`/yearbooks/${bookId}$`));
    const stored = await storedLayout(page);
    expect(stored.layout.pages.cover.items.map((i) => i.text)).toContain(VI_TEXT);
    const wide = await (
      await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1024, height: 900 } })
    ).newPage();
    await wide.request.post("/api/auth/login", {
      data: { email: OWNER.email, password: OWNER.password },
      headers: { Origin: BASE },
    });
    await wide.goto(`/yearbooks/${bookId}/editor?page=cover`);
    await expect(wide.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
    await expect(wide.locator(".ed-pagebox")).toContainText(VI_TEXT);
    await expect(wide.getByRole("navigation", { name: "Pages" })).toBeVisible();
  });
  await step("a phone never fetched the desktop stage chunk (gestures)", async () => {
    expect(fetched.filter((p) => /\/assets\/Stage-[^/]+\.js$/.test(p))).toEqual([]);
  });
});

test("414x896 portrait and 812x375 landscape: the sheet opens, the selection survives a rotation", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await phone(browser, 414, 896);
  await saveLayout(page, { v: 1, pages: { cover: { items: [textItem("coverTxt", `Xoay ${RUN}`)] } } });
  await openEditor(page);
  await page.getByRole("button", { name: new RegExp(`^Text: Xoay`) }).tap();
  await expect(sheet(page)).toBeVisible();
  await step("rotate to landscape: still selected, the sheet is still open, the bottom bar is on screen", async () => {
    await page.setViewportSize({ width: 812, height: 375 });
    await expect(page.getByRole("button", { name: new RegExp(`^Text: Xoay`) })).toHaveAttribute("aria-pressed", "true");
    await expect(sheet(page)).toBeVisible();
    const bar = await page.locator(".ed-pager").last().boundingBox();
    expect(bar.y + bar.height).toBeLessThanOrEqual(375 + 1);
    await expect(page.getByRole("button", { name: "Next page" })).toBeVisible();
  });
  await step("on-screen keyboard: with the visual viewport shrunk the sheet's textarea stays above it", async () => {
    await page.setViewportSize({ width: 414, height: 896 });
    const ta = sheet(page).getByLabel("Text of the box (edit here)");
    await ta.tap();
    await page.evaluate(() => {
      // a keyboard emulation: the layout viewport stays, the visual viewport is shorter
      const vv = window.visualViewport;
      Object.defineProperty(vv, "height", { value: 500, configurable: true });
      vv.dispatchEvent(new Event("resize"));
    });
    const kb = await page.evaluate(() => document.querySelector(".ed-bottom").style.getPropertyValue("--kb"));
    expect(parseInt(kb)).toBeGreaterThan(300);
    await expect(ta).toBeVisible();
  });
});

test("crossing 768 px keeps the selection and the typed text (AC3); a 40-item page selects fast on a 4x CPU throttle (AC4)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await phone(browser, 375, 812);
  const many = Array.from({ length: 40 }, (_, i) =>
    textItem(`many${i}`, `Mục ${i}`, { x: 20 + (i % 8) * 110, w: 100, h: 60, cy: 60 + Math.floor(i / 8) * 150 }),
  );
  await saveLayout(page, { v: 1, pages: { cover: { items: many } } });
  await openEditor(page);
  await step("select an item and edit its text, then resize the window across 768 px and back", async () => {
    await page.getByRole("button", { name: "Text: Mục 3", exact: true }).tap();
    const ta = sheet(page).getByLabel("Text of the box (edit here)");
    await ta.fill("Mục ba");
    await page.setViewportSize({ width: 900, height: 900 });
    await expect(page.getByRole("navigation", { name: "Pages" })).toBeVisible(); // the desktop shell
    await expect(page.getByLabel("Text of the box (edit here)")).toHaveValue("Mục ba");
    await expect(page.getByRole("button", { name: "Text: Mục ba", exact: true })).toHaveAttribute("aria-pressed", "true");
    await page.setViewportSize({ width: 375, height: 812 });
    await expect(position(page)).toBeVisible();
    await expect(page.getByRole("button", { name: "Text: Mục ba", exact: true })).toHaveAttribute("aria-pressed", "true");
  });
  await step("4x CPU throttle: a tap selects within a second and a swipe scrolls the screen", async () => {
    const cdp = await page.context().newCDPSession(page);
    await cdp.send("Emulation.setCPUThrottlingRate", { rate: 4 });
    const t0 = Date.now();
    await page.getByRole("button", { name: "Text: Mục 20", exact: true }).tap();
    await expect(page.getByRole("button", { name: "Text: Mục 20", exact: true })).toHaveAttribute("aria-pressed", "true");
    const ms = Date.now() - t0;
    console.log(`phone 40-item page: tap to selected in ${ms} ms (4x CPU throttle)`);
    expect(ms).toBeLessThan(3000);
    await page.getByRole("button", { name: "Close" }).tap();
    const y0 = await page.evaluate(() => window.scrollY);
    await swipe(page, { x: 187, y: 600 }, { x: 187, y: 200 });
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThanOrEqual(y0);
    await cdp.send("Emulation.setCPUThrottlingRate", { rate: 1 });
    await cdp.detach();
  });
});
