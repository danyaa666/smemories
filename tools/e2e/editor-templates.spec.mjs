// The Templates panel of the layout editor end to end (T-175, docs/editor-templates.md): preview and Back to current leave the page alone, apply replaces
// one page kind only, survives a reload and leaves a "Before template" row that restores the old page; a notes template says it applies to every notes
// page and changes the second notes page too; two tabs applying at once give one conflict; an own template is deleted.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
//
// The test never depends on the content of the shipped files: every assertion is about an OWN template that the test makes itself with
// `book-layout/template-create` from a page it saved (text only, so nothing is stripped). If the book's template has shipped templates for a kind
// the test also opens one of them in the panel (preview and Back to current only, no assertion on what it draws).
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 90_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = {
  name: "Phạm Thu Hà",
  email: `e2e-templates-${RUN}@example.com`,
  password: `Mật-khẩu-${RUN}-9`,
};
const TITLE = `Lớp 11B – Mẫu trang ${RUN}`;
const KINDS = ["cover", "profile", "notes", "back"];

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
    const login = await api.post("/api/auth/login", {
      data: { email: OWNER.email, password: OWNER.password },
    });
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
  const res = await post(page, "/api/book-layout/save", {
    yearbook_id: bookId,
    base_version: version,
    layout,
  });
  expect(res.ok(), `save ${res.status()} ${await res.text()}`).toBe(true);
}
const textItem = (id, text) => ({ id, t: "text", text, x: 100, w: 700, h: 200, cy: 450 });
/** A layout with a marker text on every page kind (the notes page is the one every notes page shares). */
const marked = (mark) => ({
  v: 1,
  pages: Object.fromEntries(KINDS.map((k) => [k, { items: [textItem(`${k}Item`, `${mark} ${k}`)] }])),
});
/** Saves a page of `kind` carrying `text`, turns it into an own template named `name` and returns its id. */
async function makeOwn(page, kind, name, text) {
  const doc = marked("x");
  doc.pages[kind] = { items: [textItem(`${kind}Src`, text)] };
  await saveLayout(page, doc);
  const res = await post(page, "/api/book-layout/template-create", { yearbook_id: bookId, kind, name });
  expect(res.ok(), `template-create ${res.status()} ${await res.text()}`).toBe(true);
  return (await res.json()).data.id;
}

async function ownerPage(browser, context) {
  const ctx =
    context ??
    (await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1500, height: 1000 } }));
  const page = await ctx.newPage();
  if (!context) {
    const login = await page.request.post("/api/auth/login", {
      data: { email: OWNER.email, password: OWNER.password },
      headers: { Origin: BASE },
    });
    expect(login.ok(), `login ${login.status()}`).toBe(true);
  }
  return page;
}
async function openEditor(page, kind = "cover") {
  await page.goto(`/yearbooks/${bookId}/editor?page=${kind}`);
  await expect(page.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
  await expect(page.locator(`.ed-pagebox .bk-sheet[data-page='${kind}']`)).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
}
const templatesButton = (page) => page.getByRole("toolbar").getByRole("button", { name: "Templates", exact: true });
const card = (page, name) => page.getByRole("radio", { name: new RegExp(`^Template ${name},`) });
async function openTemplates(page) {
  await templatesButton(page).click();
  await expect(page.getByRole("radiogroup", { name: "Templates" })).toBeVisible();
}
const stage = (page) => page.locator(".ed-pagebox");
const applyResponse = (page) =>
  page.waitForResponse((r) => r.url().includes("/api/book-layout/apply-template") && r.request().method() === "POST");
const confirmApply = (page) =>
  page.getByRole("dialog").getByRole("button", { name: "Apply template" });
const pageText = async (page, kind) => {
  const p = (await storedLayout(page)).layout?.pages?.[kind];
  return (p?.items ?? []).map((i) => i.text).join("|");
};

test("setup: register, create a Memory Book with two approved notes", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const owner = await (
    await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } })
  ).newPage();
  const friend = await (await browser.newContext({ locale: "en-US", baseURL: BASE })).newPage();
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
  await step("two approved notes (two notes pages: one friend per page)", async () => {
    await owner.goto(`/yearbooks/${bookId}/note-links`);
    await owner.getByLabel("Name (optional)").fill("Class chat");
    await owner.getByRole("button", { name: "Create link" }).click();
    await expect(owner.getByRole("heading", { name: "Your link is ready" })).toBeVisible();
    const link = new URL(await owner.getByLabel("Link for Class chat").inputValue()).pathname;
    for (const who of ["Trần Bảo", "Lê Mai"]) {
      await friend.goto(link);
      await friend.getByLabel(/^Your name/).fill(who);
      await friend.getByLabel(/^Your message/).fill("Chúc bạn luôn vui!");
      const sent = friend.waitForResponse((r) => r.url().includes("/api/public-collect/submit-note") && r.ok());
      await friend.getByRole("button", { name: "Send my note" }).click();
      await sent;
    }
    await owner.goto(`/yearbooks/${bookId}/notes`);
    await expect(owner.getByText("2 waiting for you · 0 in your book")).toBeVisible();
    for (const who of ["Trần Bảo", "Lê Mai"]) await owner.getByRole("button", { name: `Approve ${who}` }).click();
    await owner.getByRole("button", { name: /^Approved/ }).click();
    await expect(owner.locator(".inbox-card")).toHaveCount(2);
  });
});

test("cover: preview and Back to current leave the page alone; apply, reload, History row, restore (AC1, AC2, AC4, AC5)", async ({
  browser,
}) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  const SRC = `Mẫu nguồn ${RUN}`;
  const NAME = `Bìa xanh ${RUN}`;
  const NOW = `Bìa hiện tại ${RUN}`;
  await step("an own template of the cover, then a distinct current cover on every page kind", async () => {
    await makeOwn(page, "cover", NAME, SRC);
    await saveLayout(page, marked(NOW));
  });
  const before = (await storedLayout(page)).layout;
  const calls = [];
  page.on("request", (r) => {
    const m = /\/api\/book-layout\/([a-z-]+)/.exec(r.url());
    if (m && r.method() === "POST") calls.push(m[1]);
  });
  await openEditor(page);
  await expect(stage(page)).toContainText(`${NOW} cover`);

  await step("nothing is fetched before the panel opens; the panel is bound to the cover", async () => {
    await page.waitForTimeout(500);
    expect(calls.filter((c) => c === "template-list")).toEqual([]);
    await openTemplates(page);
    await expect(page.getByRole("heading", { level: 2, name: "Templates for the cover" })).toBeVisible();
    await expect(card(page, NAME)).toBeVisible();
    expect(calls.filter((c) => c === "template-list")).toHaveLength(1);
    await expect(page.getByRole("group", { name: "My templates" }).getByText(/Images not included/)).toBeVisible();
  });
  await step("a shipped template (if the book's template has one): preview, Back to current", async () => {
    const shipped = page.getByRole("group", { name: "Shipped" }).getByRole("radio");
    if ((await shipped.count()) > 0) {
      await shipped.first().click();
      await expect(page.getByRole("status").filter({ hasText: "Previewing the template" })).toBeVisible();
      await page.getByRole("button", { name: "Back to current" }).click();
      await expect(stage(page)).toContainText(`${NOW} cover`);
    }
  });
  await step("preview: the stage shows the template, read-only, nothing is stored", async () => {
    await card(page, NAME).click();
    const bar = page.getByRole("status").filter({ hasText: `Previewing the template '${NAME}' on the cover` });
    await expect(bar).toBeVisible();
    await expect(stage(page)).toContainText(SRC);
    await expect(stage(page)).not.toContainText(`${NOW} cover`);
    await expect(page.getByRole("toolbar").getByRole("button", { name: "Add text" })).toBeDisabled();
    await page.waitForTimeout(2000); // longer than the 1.5 s autosave debounce
    expect(calls).not.toContain("apply-template");
    expect(calls).not.toContain("save");
    await page.getByRole("button", { name: "Back to current" }).click();
    await expect(stage(page)).toContainText(`${NOW} cover`);
    expect((await storedLayout(page)).layout).toEqual(before); // the real page underneath is unchanged
  });
  await step("apply: one confirmation, one request on a double click, the panel closes", async () => {
    await card(page, NAME).click();
    await page.getByRole("button", { name: "Apply to the cover" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toContainText("This replaces the layout of the cover");
    await expect(dialog).toContainText("Your current version is kept in the history");
    const applied = applyResponse(page);
    await confirmApply(page).dblclick();
    const res = await applied;
    expect(res.status()).toBe(200);
    expect(calls.filter((c) => c === "apply-template")).toHaveLength(1);
    await expect(stage(page)).toContainText(SRC);
    await expect(page.getByRole("radiogroup", { name: "Templates" })).toHaveCount(0);
    await expect(templatesButton(page)).toBeFocused();
    await expect(page.locator(".visually-hidden[role=status]")).toHaveText("Template applied to the cover.");
    await expect(page.getByRole("toolbar").getByRole("button", { name: "Undo", exact: true })).toBeDisabled();
  });
  await step("only the cover changed, and it is still there after a reload", async () => {
    const after = (await storedLayout(page)).layout;
    for (const k of ["profile", "notes", "back"]) expect(after.pages[k]).toEqual(before.pages[k]);
    expect(await pageText(page, "cover")).toBe(SRC);
    await openEditor(page);
    await expect(stage(page)).toContainText(SRC);
    await page.getByRole("button", { name: "Next page" }).click();
    await expect(stage(page)).toContainText(`${NOW} profile`);
  });
  await step("History has a 'Before template' row; restoring it brings the old cover back", async () => {
    await openEditor(page);
    await page.getByRole("toolbar").getByRole("button", { name: "History" }).click();
    const rows = page.getByRole("radiogroup", { name: "Earlier versions" }).getByRole("radio");
    await expect(rows.first()).toHaveAccessibleName(/Before template/);
    await rows.first().click();
    await expect(stage(page)).toContainText(`${NOW} cover`);
    await page.getByRole("button", { name: "Restore this version" }).click();
    const restored = page.waitForResponse((r) => r.url().includes("/api/book-layout/restore") && r.ok());
    await page.getByRole("dialog").getByRole("button", { name: "Restore this version" }).click();
    await restored;
    await openEditor(page);
    await expect(stage(page)).toContainText(`${NOW} cover`);
    expect(await pageText(page, "cover")).toBe(`${NOW} cover`);
  });
  await step("the toast offers Undo in History on the new row", async () => {
    await openEditor(page);
    await openTemplates(page);
    await card(page, NAME).click();
    await page.getByRole("button", { name: "Apply to the cover" }).click();
    await confirmApply(page).click();
    await expect(stage(page)).toContainText(SRC);
    await page.getByRole("button", { name: "Undo in History" }).click();
    await expect(
      page.getByRole("radiogroup", { name: "Earlier versions" }).getByRole("radio").first(),
    ).toHaveAccessibleName(/Before template/);
  });
});

test("keyboard only: open, walk the cards, preview, apply with the confirm, focus returns (AC6)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  const SRC = `Phím nguồn ${RUN}`;
  const NAME = `Bàn phím ${RUN}`;
  await makeOwn(page, "cover", NAME, SRC);
  await saveLayout(page, marked(`Cũ ${RUN}`));
  await openEditor(page);
  await templatesButton(page).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("radiogroup", { name: "Templates" })).toBeVisible();
  const radios = page.getByRole("radiogroup", { name: "Templates" }).getByRole("radio");
  const n = await radios.count();
  expect(n).toBeGreaterThan(0);
  await page.keyboard.press("Tab"); // Refresh
  await page.keyboard.press("Tab"); // the first card
  await expect(radios.first()).toBeFocused();
  await page.keyboard.press("End");
  await expect(radios.nth(n - 1)).toBeFocused();
  await page.keyboard.press("Home");
  await expect(radios.first()).toBeFocused();
  // the own cards follow the shipped ones, newest first: this test's template is the first own card
  const shipped = await page.getByRole("group", { name: "Shipped" }).getByRole("radio").count();
  for (let i = 0; i < shipped; i++) await page.keyboard.press("ArrowDown");
  await expect(card(page, NAME)).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(stage(page)).toContainText(SRC);
  await page.keyboard.press("Escape"); // leaves the preview
  await expect(stage(page)).toContainText("Cũ");
  await page.keyboard.press("Escape"); // closes the panel
  await expect(templatesButton(page)).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(radios.first()).toBeVisible();
  await card(page, NAME).focus();
  await page.keyboard.press("Enter"); // preview again
  await page.getByRole("button", { name: "Apply to the cover" }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("button", { name: "Cancel" })).toBeFocused(); // the confirm: the safe choice first
  await page.keyboard.press("Tab");
  const applied = applyResponse(page);
  await page.keyboard.press("Enter");
  expect((await applied).status()).toBe(200);
  await expect(templatesButton(page)).toBeFocused();
  await expect(stage(page)).toContainText(SRC);
});

test("notes: the panel says it applies to every notes page, and the second notes page changes too (AC3)", async ({
  browser,
}) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  const SRC = `Lưu bút nguồn ${RUN}`;
  const NAME = `Lưu bút êm ${RUN}`;
  await makeOwn(page, "notes", NAME, SRC);
  await saveLayout(page, marked(`Cũ ${RUN}`));
  await openEditor(page, "notes");
  await expect(page.getByTestId("page-position")).toHaveText(/Page 3 of 5: Notes page 1/);
  await openTemplates(page);
  await expect(page.getByRole("heading", { level: 2, name: "Templates for the notes pages" })).toBeVisible();
  await expect(page.getByText("A notes template applies to every notes page.")).toBeVisible();
  await card(page, NAME).click();
  await page.getByRole("button", { name: "Apply to every notes page" }).click();
  await confirmApply(page).click();
  await expect(stage(page)).toContainText(SRC);
  await page.getByRole("button", { name: "Next page" }).click();
  await expect(page.getByTestId("page-position")).toHaveText(/Page 4 of 5: Notes page 2/);
  await expect(stage(page)).toContainText(SRC);
  // the other kinds are as before
  expect(await pageText(page, "cover")).toBe(`Cũ ${RUN} cover`);
  expect(await pageText(page, "back")).toBe(`Cũ ${RUN} back`);
});

test("two tabs applying at once give one conflict and overwrite nothing (AC4)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const a = await ownerPage(browser);
  const b = await ownerPage(browser, a.context());
  const SRC = `Hai tab nguồn ${RUN}`;
  const NAME = `Hai tab ${RUN}`;
  await makeOwn(a, "cover", NAME, SRC);
  await saveLayout(a, marked(`Cũ ${RUN}`));
  await openEditor(a);
  await openEditor(b);
  for (const p of [a, b]) {
    await openTemplates(p);
    await card(p, NAME).click();
    await p.getByRole("button", { name: "Apply to the cover" }).click();
    await expect(p.getByRole("dialog")).toBeVisible();
  }
  const done = Promise.all([applyResponse(a), applyResponse(b)]);
  await Promise.all([confirmApply(a).click(), confirmApply(b).click()]);
  const statuses = (await done).map((r) => r.status()).sort();
  expect(statuses).toEqual([200, 409]);
  const loser = (await (await done)[0].json()).status === "OK" ? b : a;
  await expect(loser.getByText(/changed in another tab or device, so no template was applied/)).toBeVisible();
  await expect(loser.getByText(/Editing is paused until you choose/)).toBeVisible();
  expect(await pageText(a, "cover")).toBe(SRC); // the winner's apply is what is stored, nothing overwritten
});

test("delete an own template: confirm, gone from the list and after a reopen (AC4)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  const NAME = `Xóa tôi ${RUN}`;
  const id = await makeOwn(page, "back", NAME, `Bìa sau ${RUN}`);
  await openEditor(page, "back");
  await openTemplates(page);
  const mine = card(page, NAME);
  await expect(mine).toBeVisible();
  const own = page.getByRole("group", { name: "My templates" });
  await own.getByRole("button", { name: "Delete" }).first().click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText(`Delete the template '${NAME}'? Books that already used it are not changed.`);
  const deleted = page.waitForResponse((r) => r.url().includes("/api/book-layout/template-delete"));
  await dialog.getByRole("button", { name: "Delete" }).click();
  expect((await deleted).status()).toBe(200);
  await expect(mine).toHaveCount(0);
  // no Delete on a shipped card
  await expect(page.getByRole("group", { name: "Shipped" }).getByRole("button", { name: "Delete" })).toHaveCount(0);
  await templatesButton(page).click(); // close
  await openTemplates(page);
  await expect(mine).toHaveCount(0);
  // the id is gone for good: applying it is a 404 (checked through the API, the UI no longer offers it)
  const res = await post(page, "/api/book-layout/template-delete", { id });
  expect(res.status()).toBe(404);
});
