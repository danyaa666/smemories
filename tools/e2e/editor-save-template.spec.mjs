// "Save this page as a template" in the Templates panel of the layout editor end to end (T-176, docs/editor-templates.md): the student moves and rotates the
// cover title, adds a Vietnamese text and an own image, saves the page as a template and is told the image is not kept; the book is untouched by the save;
// after "Reset all layout" the template is applied from "My templates" and the title and text come back at the same places and no image; History
// shows "Before template"; the 21st template (the cap of 20 is per account, across page kinds) is refused with its own message and the dialog keeps the
// typed name, deleting one allows the next save; another page kind, and a book on another template, do not list it.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
import { deflateSync, crc32 } from "node:zlib";
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 120_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = {
  name: "Đặng Ngọc Ánh",
  email: `e2e-save-template-${RUN}@example.com`,
  password: `Mật-khẩu-${RUN}-9`,
};
const TITLE = `Lớp 12C – Lưu mẫu ${RUN}`;
const KINDS = ["cover", "profile", "notes", "back"];
const VI_TEXT = "Chúc các bạn thật nhiều niềm vui";
const NAME = "Bìa của tôi";

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

let bookId = "";

/** Signs in again and deletes the own templates (of both layout templates of the book) and the book; the account stays. */
async function cleanUp() {
  if (!BASE) return;
  const api = await request.newContext({ baseURL: BASE, extraHTTPHeaders: { Origin: BASE } });
  try {
    const login = await api.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password } });
    if (!login.ok()) return;
    const list = await (await api.get("/api/yearbook/get-list")).json();
    const books = list.data?.items ?? [];
    for (const b of books)
      for (const template_id of ["memory-book", "navy-classic"]) {
        await api.post("/api/yearbook/update", { data: { id: b.id, template_id } });
        for (const kind of KINDS) {
          const t = await (await api.post("/api/book-layout/template-list", { data: { yearbook_id: b.id, kind } })).json();
          for (const x of t.data?.items ?? [])
            if (x.source === "own") await api.post("/api/book-layout/template-delete", { data: { id: x.id } });
        }
      }
    await api.post("/api/yearbook/update", { data: { id: books[0]?.id, template_id: "memory-book" } });
    for (const y of books) await api.post("/api/yearbook/delete", { data: { id: y.id } });
  } catch (e) {
    console.warn(`clean-up failed (the run's data stays in the test database): ${e.message}`);
  } finally {
    await api.dispose();
  }
}

test.afterAll(cleanUp);
test.describe.configure({ mode: "serial" });

/** A real PNG (RGB gradient), made without a library (the same as editor-palette.spec.mjs). */
function png(w, h, seed = 0) {
  const raw = Buffer.alloc((w * 3 + 1) * h);
  for (let y = 0; y < h; y++) {
    raw[y * (w * 3 + 1)] = 0;
    for (let x = 0; x < w; x++) {
      const o = y * (w * 3 + 1) + 1 + x * 3;
      raw[o] = (x * 255) / w;
      raw[o + 1] = (y * 255) / h;
      raw[o + 2] = (seed * 40) % 256;
    }
  }
  const chunk = (type, data) => {
    const len = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    const body = Buffer.concat([Buffer.from(type), data]);
    const crc = Buffer.alloc(4);
    crc.writeUInt32BE(crc32(body));
    return Buffer.concat([len, body, crc]);
  };
  const head = Buffer.alloc(13);
  head.writeUInt32BE(w, 0);
  head.writeUInt32BE(h, 4);
  head.set([8, 2, 0, 0, 0], 8);
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk("IHDR", head), chunk("IDAT", deflateSync(raw)), chunk("IEND", Buffer.alloc(0))]);
}

/** The layout routes share the save limit (burst 20 per 10 s, 60 a minute): a 429 waits its Retry-After and tries again. */
async function post(page, path, data) {
  for (let i = 0; ; i++) {
    const res = await page.request.post(path, { data, headers: { Origin: BASE } });
    if (res.status() !== 429 || i >= 8) return res;
    await page.waitForTimeout(Math.max(1, Number(res.headers()["retry-after"] ?? 2)) * 1000);
  }
}
const storedLayout = async (page) => (await (await page.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json()).data;
const storedCover = async (page) => (await storedLayout(page)).layout?.pages?.cover ?? {};
async function ownTemplates(page, kind) {
  const r = await post(page, "/api/book-layout/template-list", { yearbook_id: bookId, kind });
  expect(r.ok(), `template-list ${r.status()}`).toBe(true);
  return ((await r.json()).data.items ?? []).filter((x) => x.source === "own");
}
async function ownerPage(browser) {
  const ctx = await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1500, height: 1000 } });
  const page = await ctx.newPage();
  const login = await page.request.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password }, headers: { Origin: BASE } });
  expect(login.ok(), `login ${login.status()}`).toBe(true);
  return page;
}
async function openEditor(page, kind = "cover") {
  await page.goto(`/yearbooks/${bookId}/editor?page=${kind}`);
  await expect(page.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
  await expect(page.locator(`.ed-pagebox .bk-sheet[data-page='${kind}']`)).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
}
/** The palette is a fourth column: closed at first below 1700 px (the toolbar button opens it). */
async function showPalette(page) {
  const toggle = page.getByRole("toolbar").getByRole("button", { name: "Add to page" });
  if ((await toggle.getAttribute("aria-expanded")) === "false") await toggle.click();
  await expect(page.getByRole("complementary", { name: "Add to the page" })).toBeVisible();
}
const palette = (page) => page.getByRole("complementary", { name: "Add to the page" });
const templatesButton = (page) => page.getByRole("toolbar").getByRole("button", { name: "Templates", exact: true });
const card = (page, name) => page.getByRole("radio", { name: new RegExp(`^Template ${name},`) });
const saveButton = (page) => page.getByRole("button", { name: "Save this page as a template" });
const saveDialog = (page) => page.getByRole("dialog", { name: "Save this page as a template" });
const stage = (page) => page.locator(".ed-pagebox");
const num = (page, name) => page.getByRole("spinbutton", { name, exact: true });
const savedLayout = (page) => page.waitForResponse((r) => r.url().includes("/api/book-layout/save") && r.request().method() === "POST" && r.ok());
const applyResponse = (page) => page.waitForResponse((r) => r.url().includes("/api/book-layout/apply-template") && r.request().method() === "POST");
const createResponse = (page) => page.waitForResponse((r) => r.url().includes("/api/book-layout/template-create") && r.request().method() === "POST");
async function openTemplates(page) {
  await templatesButton(page).click();
  await expect(page.getByRole("radiogroup", { name: "Templates" })).toBeVisible();
}
async function saveAs(page, name) {
  await saveButton(page).click();
  const field = saveDialog(page).getByLabel("Template name");
  await expect(field).toBeFocused();
  await field.fill(name);
  const created = createResponse(page);
  await field.press("Enter");
  return created;
}

test("setup: register, create a Memory Book", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const owner = await (await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } })).newPage();
  await step("register and verify the email", async () => {
    await owner.goto("/register");
    await owner.getByLabel("Your name").fill(OWNER.name);
    await owner.getByLabel("Email").fill(OWNER.email);
    await owner.getByLabel("Password", { exact: true }).fill(OWNER.password);
    await owner.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
    const registered = owner.waitForResponse((r) => r.url().includes("/api/auth/register") && r.request().method() === "POST");
    await owner.getByRole("button", { name: "Create account" }).click();
    if ((await registered).status() === 429) throw new Error("registration is rate limited: run `make reset-limits SMEM_ENV=test` (make e2e does)");
    await owner.getByLabel("6-digit code").fill(OTP);
    await expect(owner.getByText("Your email address is verified.")).toBeVisible();
  });
  await step("create the book on Memory Book", async () => {
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

test("save a customised cover as a template, reset, apply it again: title and text back, no image (AC1, AC3)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  const calls = [];
  page.on("request", (r) => {
    const m = /\/api\/book-layout\/([a-z-]+)/.exec(r.url());
    if (m && r.method() === "POST") calls.push(m[1]);
  });
  await openEditor(page);
  await showPalette(page);
  await step("move and rotate the cover title, add a Vietnamese text and an own image", async () => {
    await page.getByRole("button", { name: "Cover title", exact: true }).click();
    let done = savedLayout(page);
    await num(page, "Left").fill("12");
    await num(page, "Left").press("Enter");
    await done;
    done = savedLayout(page);
    await num(page, "Rotation (degrees)").fill("5");
    await num(page, "Rotation (degrees)").press("Enter");
    await done;
    await page.getByRole("toolbar").getByRole("button", { name: "Add text" }).click();
    await expect(page.getByRole("textbox", { name: "Text of the box", exact: true })).toBeFocused();
    await page.keyboard.type(VI_TEXT);
    await page.keyboard.press("Escape");
    await expect(page.locator(".ed-pagebox [data-layout-item] .bk-fit")).toHaveText(VI_TEXT);
    await palette(page).getByRole("tab", { name: "My images" }).click();
    await palette(page).getByTestId("design-file").setInputFiles({ name: "holiday.png", mimeType: "image/png", buffer: png(400, 300, 1) });
    await palette(page).getByRole("button", { name: "Add image 1 of 1" }).click();
    await expect.poll(async () => ((await storedCover(page)).items ?? []).map((i) => i.t).sort(), { timeout: 15_000 }).toEqual(["image", "text"]);
  });
  const before = await storedLayout(page);
  const title = before.layout.pages.cover.overrides["cover.title"];
  expect(title).toMatchObject({ x: 120, r: 5 });

  await step("the button is in the Templates panel; nothing is sent before it is pressed", async () => {
    await page.waitForTimeout(300);
    expect(calls).not.toContain("template-create");
    await openTemplates(page);
    await expect(saveButton(page)).toBeEnabled();
    await expect(page.getByRole("group", { name: "My templates" })).toContainText("Up to 20 own templates per account, across all page types.");
  });
  await step("the dialog: default name, what is kept, how many images are not; a bad name sends nothing", async () => {
    await saveButton(page).click();
    const dialog = saveDialog(page);
    const field = dialog.getByLabel("Template name");
    await expect(field).toBeFocused();
    await expect(field).toHaveValue("My cover 1");
    await expect(dialog).toContainText("This saves the cover as it is now: 1 item, 1 moved element. 1 image is not saved (your own images are not copied to templates).");
    await field.fill("");
    await dialog.getByRole("button", { name: "Save" }).click();
    await expect(dialog.getByRole("alert")).toContainText("Enter a name for the template.");
    await field.fill("ệ".repeat(61));
    await expect(dialog).toContainText("61 of 60 characters");
    await dialog.getByRole("button", { name: "Save" }).click();
    await expect(dialog.getByRole("alert")).toContainText("Use at most 60 characters.");
    expect(calls).not.toContain("template-create");
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(saveButton(page)).toBeFocused();
  });
  await step("save: Enter, one request on a double click, the note about images, the new card focused; the book is untouched", async () => {
    await page.waitForTimeout(2000); // nothing of the editing is still waiting to be autosaved
    const savesBefore = calls.filter((c) => c === "save").length;
    await saveButton(page).click();
    const field = saveDialog(page).getByLabel("Template name");
    await field.fill(NAME);
    const created = createResponse(page);
    await saveDialog(page).getByRole("button", { name: "Save" }).dblclick();
    const res = await created;
    expect(res.status()).toBe(200);
    expect((await res.json()).data).toMatchObject({ name: NAME, kind: "cover", item_count: 1, stripped: 1 });
    expect(calls.filter((c) => c === "template-create")).toHaveLength(1);
    await expect(saveDialog(page)).toHaveCount(0);
    await expect(card(page, NAME)).toBeFocused();
    await expect(card(page, NAME)).toHaveAccessibleName(/, new$/);
    await expect(page.locator(".visually-hidden[role=status]")).toHaveText(`Template '${NAME}' saved. 1 image was not saved.`);
    await expect(page.locator(".banner").filter({ hasText: "1 image was not saved." })).toBeVisible();
    await expect(page.getByRole("group", { name: "My templates" }).getByText(/Images not included/)).toBeVisible();
    await page.waitForTimeout(2000); // longer than the 1.5 s autosave debounce: a template save leaves no autosave behind
    expect(calls.filter((c) => c === "save")).toHaveLength(savesBefore); // a template save sends no layout save
    expect(await storedLayout(page)).toEqual(before); // document and version are exactly as before
    await expect(page.getByRole("toolbar").getByRole("button", { name: "Undo", exact: true })).toBeEnabled(); // the undo stack is kept
  });
  await step("reload, Reset all layout: the cover is plain again", async () => {
    await openEditor(page);
    await page.getByRole("button", { name: "Reset all layout" }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Reset all layout" }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    expect(await storedCover(page)).toEqual({});
    await expect(stage(page)).not.toContainText(VI_TEXT);
  });
  await step("apply it from My templates: the title and the text are back where they were, there is no image", async () => {
    await openTemplates(page);
    await card(page, NAME).click();
    const applied = applyResponse(page);
    await page.getByRole("button", { name: "Apply to the cover" }).click(); // nothing to lose after a reset: no confirm
    expect((await applied).status()).toBe(200);
    await expect(templatesButton(page)).toBeFocused(); // the panel is closed (the stage drew the template already while it was previewed)
    await expect(stage(page)).toContainText(VI_TEXT);
    const cover = await storedCover(page);
    expect(cover.overrides["cover.title"]).toEqual(title);
    expect(cover.items).toHaveLength(1);
    expect(cover.items[0]).toMatchObject({ t: "text", text: VI_TEXT });
    expect(cover.items[0]).toEqual(before.layout.pages.cover.items.find((i) => i.t === "text"));
    await expect(stage(page).locator("[data-layout-item] img")).toHaveCount(0);
  });
  await step("it can be applied again over a changed page, and History has the 'Before template' row", async () => {
    await page.getByRole("button", { name: "Cover title", exact: true }).click();
    const done = savedLayout(page);
    await num(page, "Width").fill("40");
    await num(page, "Width").press("Enter");
    await done;
    await openTemplates(page);
    await card(page, NAME).click();
    await page.getByRole("button", { name: "Apply to the cover" }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Apply template" }).click();
    await expect(page.getByRole("radiogroup", { name: "Templates" })).toHaveCount(0);
    await page.getByRole("toolbar").getByRole("button", { name: "History" }).click();
    await expect(page.getByRole("radiogroup", { name: "Earlier versions" }).getByRole("radio").first()).toHaveAccessibleName(/Before template/);
  });
});

test("the 21st template is refused; deleting one allows the next save; other kinds and other templates do not list it (AC2)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  test.setTimeout(240_000);
  const page = await ownerPage(browser);
  await step("fill the account to 20 own templates through the API (the cap is per account, across page kinds)", async () => {
    const have = (await Promise.all(["cover", "profile", "back"].map((k) => ownTemplates(page, k)))).flat().length;
    expect(have).toBeGreaterThan(0); // the first test saved one
    const { version, layout } = await storedLayout(page);
    const text = (id, t) => ({ id, t: "text", text: t, x: 100, w: 700, h: 200, cy: 450 });
    const doc = {
      v: 1,
      pages: Object.fromEntries(["cover", "profile", "back"].map((k) => [k, { ...layout?.pages?.[k], items: [text(`${k}Page`, `Trang ${k} ${RUN}`)] }])),
    };
    const saved = await post(page, "/api/book-layout/save", { yearbook_id: bookId, base_version: version, layout: doc });
    expect(saved.ok(), `save ${saved.status()}`).toBe(true);
    for (let i = have; i < 20; i++) {
      const res = await post(page, "/api/book-layout/template-create", { yearbook_id: bookId, kind: ["cover", "profile", "back"][i % 3], name: `Mẫu ${i} ${RUN}` });
      expect(res.ok(), `create ${i}: ${res.status()} ${await res.text()}`).toBe(true);
    }
    await page.waitForTimeout(11_000); // the burst of 20 per 10 s is spent: the UI steps below must not meet a 429
    expect((await Promise.all(KINDS.map((k) => ownTemplates(page, k)))).flat()).toHaveLength(20);
  });
  await step("the 21st, from the profile page (another kind than most): its own message, the dialog and the name stay", async () => {
    await openEditor(page, "profile");
    await openTemplates(page);
    await saveButton(page).click();
    const field = saveDialog(page).getByLabel("Template name");
    await field.fill("Cái thứ 21");
    const created = createResponse(page);
    await field.press("Enter");
    expect((await created).status()).toBe(409);
    await expect(saveDialog(page).getByRole("alert")).toHaveText(
      "You have 20 own templates, the most you can keep across all page types. Delete one in the panel first.",
    );
    await expect(field).toHaveValue("Cái thứ 21");
    await expect(page.getByRole("group", { name: "My templates" })).toBeInViewport();
    await saveDialog(page).getByRole("button", { name: "Cancel" }).click();
    // now the button explains instead of opening a dialog that is bound to fail, and nothing is sent
    let sent = 0;
    page.on("request", (r) => r.url().includes("/api/book-layout/template-create") && sent++);
    await saveButton(page).click();
    await expect(saveDialog(page)).toHaveCount(0);
    await expect(page.locator("#ed-tpl-save-note")).toContainText("You have 20 own templates");
    expect(sent).toBe(0);
  });
  await step("delete one in the panel: the next save goes through", async () => {
    const own = page.getByRole("group", { name: "My templates" });
    await own.getByRole("button", { name: "Delete" }).first().click();
    const deleted = page.waitForResponse((r) => r.url().includes("/api/book-layout/template-delete"));
    await page.getByRole("dialog").getByRole("button", { name: "Delete" }).click();
    expect((await deleted).status()).toBe(200);
    await expect(page.locator("#ed-tpl-save-note")).toHaveText("");
    const created = await saveAs(page, "Cái thứ 21");
    expect((await created).status()).toBe(200);
    await expect(card(page, "Cái thứ 21")).toBeFocused();
    expect((await Promise.all(KINDS.map((k) => ownTemplates(page, k)))).flat()).toHaveLength(20);
  });
  await step("the template is not offered for another page kind", async () => {
    await openEditor(page, "back");
    await openTemplates(page);
    await expect(card(page, "Cái thứ 21")).toHaveCount(0);
    expect((await ownTemplates(page, "back")).map((x) => x.label_en)).not.toContain("Cái thứ 21");
  });
  await step("a book switched to another template does not list it (and the cover has the saved ones back on Memory Book)", async () => {
    expect((await post(page, "/api/yearbook/update", { id: bookId, template_id: "navy-classic" })).ok()).toBe(true);
    try {
      await openEditor(page, "profile");
      await openTemplates(page);
      await expect(card(page, "Cái thứ 21")).toHaveCount(0);
      await expect(page.getByRole("group", { name: "My templates" })).toContainText("No templates for this page yet.");
      expect(await ownTemplates(page, "profile")).toEqual([]);
    } finally {
      expect((await post(page, "/api/yearbook/update", { id: bookId, template_id: "memory-book" })).ok()).toBe(true);
    }
    await openEditor(page, "profile");
    await openTemplates(page);
    await expect(card(page, "Cái thứ 21")).toBeVisible();
  });
});
