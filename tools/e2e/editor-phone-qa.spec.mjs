// QA probes of the phone editor (T-181), beyond editor-phone.spec.mjs: the 44 px rule of the Templates dialogs on a touch screen at 768 px (the shell the
// leader's note on T-176 is about), 320 px wide and 200% text, a vertical swipe over an item scrolls the screen and moves nothing, the delete confirm,
// and a save refused as a conflict (another tab) pauses the phone editor. Run through `make e2e` (scripts/e2e.sh), same setup as editor-phone.spec.mjs.
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Lê Quang Huy", email: `e2e-phoneqa-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9` };
const KINDS = ["cover", "profile", "notes", "back"];

test.describe.configure({ mode: "serial" });
let bookId = "";

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
        const t = await (await api.post("/api/book-layout/template-list", { data: { yearbook_id: books[0].id, kind } })).json();
        for (const x of t.data?.items ?? [])
          if (x.source === "own") await api.post("/api/book-layout/template-delete", { data: { id: x.id } });
      }
    for (const y of books) await api.post("/api/yearbook/delete", { data: { id: y.id } });
  } finally {
    await api.dispose();
  }
}
test.afterAll(cleanUp);

async function post(page, path, data) {
  for (let i = 0; ; i++) {
    const res = await page.request.post(path, { data, headers: { Origin: BASE } });
    if (res.status() !== 429 || i >= 8) return res;
    await page.waitForTimeout(Math.max(1, Number(res.headers()["retry-after"] ?? 2)) * 1000);
  }
}
const storedLayout = async (page) => (await (await page.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json()).data;
async function saveLayout(page, layout) {
  const { version } = await storedLayout(page);
  const res = await post(page, "/api/book-layout/save", { yearbook_id: bookId, base_version: version, layout });
  expect(res.ok(), `save ${res.status()} ${await res.text()}`).toBe(true);
}
const textItem = (id, text, over = {}) => ({ id, t: "text", text, x: 100, w: 700, h: 200, cy: 450, ...over });

async function touchPage(browser, width, height) {
  const ctx = await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width, height }, hasTouch: true, isMobile: true, deviceScaleFactor: 2 });
  const page = await ctx.newPage();
  const login = await page.request.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password }, headers: { Origin: BASE } });
  expect(login.ok(), `login ${login.status()}`).toBe(true);
  return page;
}
async function openEditor(page, kind = "cover") {
  await page.goto(`/yearbooks/${bookId}/editor?page=${kind}`);
  await expect(page.locator(`.ed-pagebox .bk-sheet[data-page='${kind}']`)).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
}
async function swipe(page, from, to, steps = 10) {
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
const small = (page, sel) =>
  page.evaluate(
    (s) =>
      [...document.querySelectorAll(s)]
        .filter((b) => b.offsetParent !== null && !b.classList.contains("ed-hit"))
        .map((b) => ({ n: (b.getAttribute("aria-label") ?? b.textContent).trim().slice(0, 30), h: Math.round(b.getBoundingClientRect().height * 10) / 10 }))
        .filter((b) => b.h > 0 && b.h < 43.5),
    sel,
  );

test("setup: register and create a Memory Book", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e`");
  const owner = await (await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } })).newPage();
  await owner.goto("/register");
  await owner.getByLabel("Your name").fill(OWNER.name);
  await owner.getByLabel("Email").fill(OWNER.email);
  await owner.getByLabel("Password", { exact: true }).fill(OWNER.password);
  await owner.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
  await owner.getByRole("button", { name: "Create account" }).click();
  await owner.getByLabel("6-digit code").fill(OTP);
  await expect(owner.getByText("Your email address is verified.")).toBeVisible();
  await owner.goto("/yearbooks/new");
  await owner.getByLabel("Title").fill(`Lớp QA ${RUN}`);
  await owner.getByLabel("Yearbook language").selectOption("vi");
  await owner.getByLabel("Page size").selectOption("A5");
  await owner.getByRole("button", { name: "Create yearbook" }).click();
  await expect(owner.getByRole("heading", { level: 1, name: `Lớp QA ${RUN}` })).toBeVisible();
  bookId = new URL(owner.url()).pathname.split("/").pop();
  expect((await post(owner, "/api/yearbook/update", { id: bookId, template_id: "memory-book" })).ok()).toBe(true);
});

test("768 px touch (desktop shell, coarse pointer): the Templates dialogs have 44 px buttons (QA of T-176)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await touchPage(browser, 768, 1024);
  const NAME = `Mẫu QA ${RUN}`;
  await saveLayout(page, { v: 1, pages: { back: { items: [textItem("backSrc", `Bìa sau ${RUN}`)] } } });
  const made = await post(page, "/api/book-layout/template-create", { yearbook_id: bookId, kind: "back", name: NAME });
  expect(made.ok()).toBe(true);
  await openEditor(page, "back");
  expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches), "the emulation must be a coarse pointer").toBe(true);
  await expect(page.locator(".ed-phone")).toHaveCount(0); // 768 px is the desktop shell
  await page.getByRole("toolbar").getByRole("button", { name: "Templates", exact: true }).tap();
  const own = page.getByRole("group", { name: "My templates" });
  // the delete dialog
  await own.getByRole("button", { name: "Delete" }).first().tap();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  const heights = await dialog.getByRole("button").evaluateAll((bs) => bs.map((b) => ({ n: b.textContent.trim(), h: b.getBoundingClientRect().height })));
  console.log("QA-768-DELETE-DIALOG", JSON.stringify(heights));
  for (const b of heights) expect(b.h, `delete dialog button ${b.n}`).toBeGreaterThanOrEqual(43.5);
  await dialog.getByRole("button", { name: "Cancel" }).tap();
  // the apply confirm
  await page.getByRole("button", { name: new RegExp(`^Template ${NAME},`) }).tap();
  await page.getByRole("button", { name: /^Apply to/ }).tap();
  const apply = page.getByRole("dialog");
  await expect(apply).toBeVisible();
  const h2 = await apply.getByRole("button").evaluateAll((bs) => bs.map((b) => ({ n: b.textContent.trim(), h: b.getBoundingClientRect().height })));
  console.log("QA-768-APPLY-DIALOG", JSON.stringify(h2));
  for (const b of h2) expect(b.h, `apply dialog button ${b.n}`).toBeGreaterThanOrEqual(43.5);
});

test("320x640 and 200% text: no sideways scroll, every button 44 px, the sheet and Done are reachable", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await touchPage(browser, 320, 640);
  await saveLayout(page, { v: 1, pages: { cover: { items: [textItem("coverTxt", `Chữ ${RUN}`)] } } });
  await openEditor(page);
  await page.getByRole("button", { name: new RegExp("^Text: Chữ") }).tap();
  await expect(page.getByRole("region", { name: "Properties", exact: true })).toBeVisible();
  const check = async (label) => {
    const o = await page.evaluate(() => ({ sw: document.documentElement.scrollWidth, iw: window.innerWidth }));
    console.log("QA-320", label, JSON.stringify(o), JSON.stringify(await small(page, ".ed-phone button, .ed-phone a.ed-plink, .ed-phone select")));
    expect(o.sw, `${label}: sideways scroll`).toBeLessThanOrEqual(o.iw + 1);
    expect(await small(page, ".ed-phone button, .ed-phone a.ed-plink, .ed-phone select"), `${label}: buttons under 44 px`).toEqual([]);
    const bar = await page.locator(".ed-bottom").boundingBox();
    expect(bar.y + bar.height, `${label}: bottom bar on screen`).toBeLessThanOrEqual(page.viewportSize().height + 1);
  };
  await check("100%");
  await page.evaluate(() => (document.documentElement.style.fontSize = "200%"));
  await check("200% text");
  // the Done button can be reached by scrolling the screen (the sheet is capped, the screen scrolls)
  const done = page.getByRole("button", { name: "Done", exact: true });
  await done.scrollIntoViewIfNeeded();
  await expect(done).toBeVisible();
});

test("a vertical swipe that starts on an item scrolls the screen and changes nothing", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await touchPage(browser, 375, 667);
  const many = Array.from({ length: 20 }, (_, i) => textItem(`swipe${String(i).padStart(2, "0")}`, `Cuộn ${i}`, { x: 20 + (i % 4) * 190, w: 170, h: 120, cy: 100 + Math.floor(i / 4) * 150 }));
  await saveLayout(page, { v: 1, pages: { cover: { items: many } } });
  await openEditor(page);
  const before = await storedLayout(page);
  const hit = page.getByRole("button", { name: "Text: Cuộn 2", exact: true });
  const box = await hit.boundingBox();
  const y0 = await page.evaluate(() => window.scrollY);
  await swipe(page, { x: box.x + box.width / 2, y: box.y + box.height / 2 }, { x: box.x + box.width / 2, y: box.y + box.height / 2 - 150 });
  await page.waitForTimeout(2200);
  const y1 = await page.evaluate(() => window.scrollY);
  console.log("QA-SWIPE scrollY", y0, "->", y1);
  expect(y1, "the screen scrolled").toBeGreaterThan(y0);
  const after = await storedLayout(page);
  expect(after.version).toBe(before.version);
  expect(after.layout).toEqual(before.layout);
  await expect(page.getByRole("button", { name: "Undo", exact: true })).toBeDisabled();
});

test("delete asks once: Cancel keeps the item, Delete removes it and is one undo step", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await touchPage(browser, 375, 812);
  await saveLayout(page, { v: 1, pages: { cover: { items: [textItem("deleteMe", `Xoá ${RUN}`)] } } });
  await openEditor(page);
  await page.getByRole("button", { name: new RegExp("^Text: Xoá") }).tap();
  const sheet = page.getByRole("region", { name: "Properties", exact: true });
  await sheet.getByRole("button", { name: "Delete", exact: true }).tap();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel" }).tap();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("button", { name: new RegExp("^Text: Xoá") })).toHaveCount(1);
  await sheet.getByRole("button", { name: "Delete", exact: true }).tap();
  const saved = page.waitForResponse((r) => r.url().includes("/api/book-layout/save") && r.request().method() === "POST");
  await page.getByRole("dialog").getByRole("button", { name: "Delete", exact: true }).tap();
  expect((await saved).status()).toBe(200);
  await expect(page.getByRole("button", { name: new RegExp("^Text: Xoá") })).toHaveCount(0);
  await page.getByRole("button", { name: "Undo", exact: true }).tap();
  await expect(page.getByRole("button", { name: new RegExp("^Text: Xoá") })).toHaveCount(1);
});

test("another tab saved first: the phone editor shows the conflict banner and pauses", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await touchPage(browser, 375, 812);
  await saveLayout(page, { v: 1, pages: { cover: { items: [textItem("conflict1", `Xung đột ${RUN}`)] } } });
  await openEditor(page);
  await page.getByRole("button", { name: new RegExp("^Text: Xung") }).tap();
  const sheet = page.getByRole("region", { name: "Properties", exact: true });
  await expect(sheet).toBeVisible();
  await saveLayout(page, { v: 1, pages: { cover: { items: [textItem("conflict1", `Xung đột ${RUN}`, { x: 150 })] } } }); // the "other tab"
  await sheet.getByRole("button", { name: "Move right" }).tap();
  await expect(page.getByText(/changed in another tab or device/).first()).toBeVisible({ timeout: 15_000 });
  console.log("QA-CONFLICT banner shown");
});
