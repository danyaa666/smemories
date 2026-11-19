// The stage 2 exit story end to end (T-183, docs/e2e.md "The stage 2 story", docs/stage2-exit-report.md): one student, real stack, no mocks.
// Signs up, makes a book, and on Memory Book and then on Navy Classic: on the cover moves, resizes and rotates the title, adds a Vietnamese text, a sticker
// and an own image; undoes and redoes; applies a shipped template to the notes pages (every notes page changes); saves the profile page as an own template,
// takes the page apart and applies the own template back; opens History and restores the older version; presses Done, opens Print and gets the PDF; reloads
// and finds the same layout. Then the student joins a class, edits the class page, and the class admin's class book shows that member's layout and leaves a
// member without a layout as it was. Two projects run it (playwright.config.mjs): "journey-desktop" (1280) and "journey-tablet" (1024, touch).
//   make e2e                                  (every spec)       scripts/e2e.sh stage2-journey.spec.mjs   (this one)
// Credentials: the registered fixed email code (DEV-SHORTCUT(otp), SMEM_ENV=test only).
import { deflateSync, crc32 } from "node:zlib";
import { mkdtempSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { expect, request, test } from "@playwright/test";
import { inspect, savePdf, text } from "../print-check/lib.mjs";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 120_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const VI_TEXT = "Chúc mừng tốt nghiệp";
const PROFILE_TEXT = `Hồ sơ của tôi ${RUN}`;
const OWN = "Hồ sơ mẫu";
const NOTES = ["Trần Bảo", "Lê Mai", "Phạm Hà", "Võ Nam", "Đỗ Quyên"]; // five friends: Navy Classic makes two notes pages, Memory Book five

const squash = (s) => s.normalize("NFC").replace(/\s+/g, "").toLocaleLowerCase("vi");

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

test.describe.configure({ mode: "serial" });

test("stage 2 story: edit Memory Book and Navy Classic, print, reload; then the class admin sees the member's layout", async ({ browser }, testInfo) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  test.setTimeout(600_000);
  const tag = testInfo.project.name.replace("journey-", "");
  const touch = tag === "tablet";
  const viewport = touch ? { width: 1024, height: 768 } : { width: 1280, height: 900 };
  const user = (name, who) => ({ name, email: `e2e-s2-${tag}-${who}-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-${who}9` });
  const STUDENT = user("Đặng Ngọc Ánh", "s");
  const OTHER = user("Lê Thị Bích Ngọc", "t");
  const ADMIN = user("Cô Nguyễn Thu Hà", "a");
  const TITLE = `Lớp 12C – Câu chuyện ${tag} ${RUN}`;
  const CLASS_NAME = `Lớp 12A1 ${tag} ${RUN}`;
  const secrets = [];
  const created = { classId: "" };

  const ctx = (extra = {}) => browser.newContext({ locale: "en-US", baseURL: BASE, viewport, hasTouch: touch, ...extra });
  const page = await (await ctx()).newPage();
  const friend = await (await browser.newContext({ locale: "en-US", baseURL: BASE })).newPage();
  const press = (loc) => (touch ? loc.tap() : loc.click()); // the tablet project taps
  let bookId = "";

  const post = async (p, url, data) => {
    for (let i = 0; ; i++) {
      const res = await p.request.post(url, { data, headers: { Origin: BASE } });
      if (res.status() !== 429 || i >= 8) return res; // the layout routes share a save limit: wait the Retry-After
      await p.waitForTimeout(Math.max(1, Number(res.headers()["retry-after"] ?? 2)) * 1000);
    }
  };
  const stored = async (p = page, id = bookId) => (await (await p.request.get(`/api/book-layout/get?yearbook_id=${id}`)).json()).data;
  const storedPage = async (kind) => (await stored()).layout?.pages?.[kind] ?? {};
  const palette = () => page.getByRole("complementary", { name: "Add to the page" });
  const num = (name) => page.getByRole("spinbutton", { name, exact: true });
  const saved = () => page.waitForResponse((r) => r.url().includes("/api/book-layout/save") && r.request().method() === "POST" && r.ok());
  const position = () => page.getByTestId("page-position");
  const stage = () => page.locator(".ed-pagebox");
  const templatesButton = () => page.getByRole("toolbar").getByRole("button", { name: "Templates", exact: true });
  const applyResponse = () => page.waitForResponse((r) => r.url().includes("/api/book-layout/apply-template") && r.request().method() === "POST");
  const confirmApply = () => page.getByRole("dialog").getByRole("button", { name: "Apply template" });

  const signUp = async (p, u, from) => {
    if (from) {
      await p.goto(from);
      await p.locator("main").getByRole("link", { name: "Create account" }).click();
    } else {
      await p.goto("/register");
    }
    await p.getByLabel("Your name").fill(u.name);
    await p.getByLabel("Email").fill(u.email);
    await p.getByLabel("Password", { exact: true }).fill(u.password);
    await p.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
    const registered = p.waitForResponse((r) => r.url().includes("/api/auth/register") && r.request().method() === "POST");
    await p.getByRole("button", { name: "Create account" }).click();
    if ((await registered).status() === 429) throw new Error("registration is rate limited: run `make reset-limits SMEM_ENV=test` (make e2e does)");
    await expect(p.getByRole("heading", { name: "Verify your email" })).toBeVisible();
    await p.getByLabel("6-digit code").fill(OTP);
    await expect(p.getByText("Your email address is verified.")).toBeVisible();
  };
  const openEditor = async (kind = "cover") => {
    await page.goto(`/yearbooks/${bookId}/editor?page=${kind}`);
    await expect(page.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
    await expect(page.locator(`.ed-pagebox .bk-sheet[data-page='${kind}']`)).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
  };
  const showPalette = async () => {
    const toggle = page.getByRole("toolbar").getByRole("button", { name: "Add to page" });
    if ((await toggle.getAttribute("aria-expanded")) === "false") await press(toggle);
    await expect(palette()).toBeVisible();
  };
  /** The sheet's layout layer without ids that change between pages: what the template put on it. */
  const layer = () => page.locator(".ed-pagebox .bk-sheet[data-page='notes']").evaluate((s) => [...s.querySelectorAll("[data-layout-item]")].map((e) => e.dataset.layoutItem).join(","));

  /** Deletes what the run created (the accounts stay). Never fails the test. */
  const cleanUp = async () => {
    for (const [u, deleteClass] of [[ADMIN, true], [STUDENT, false], [OTHER, false]]) {
      const api = await request.newContext({ baseURL: BASE, extraHTTPHeaders: { Origin: BASE } });
      try {
        const login = await api.post("/api/auth/login", { data: { email: u.email, password: u.password } });
        if (!login.ok()) continue;
        if (deleteClass && created.classId) await api.post("/api/class/delete", { data: { id: created.classId } });
        const list = await (await api.get("/api/yearbook/get-list")).json();
        for (const y of list.data?.items ?? []) await api.post("/api/yearbook/delete", { data: { id: y.id } });
      } catch (e) {
        console.warn(`clean-up failed (the run's data stays in the test database): ${e.message}`);
      } finally {
        await api.dispose();
      }
    }
  };

  try {
    await step("1. sign up, create the book, five approved friend notes", async () => {
      await signUp(page, STUDENT);
      await page.goto("/yearbooks/new");
      await page.getByLabel("Title").fill(TITLE);
      await page.getByLabel("Yearbook language").selectOption("vi");
      await page.getByLabel("Page size").selectOption("A5");
      await page.getByRole("button", { name: "Create yearbook" }).click();
      await expect(page.getByRole("heading", { level: 1, name: TITLE })).toBeVisible();
      bookId = new URL(page.url()).pathname.split("/").pop();
      const res = await post(page, "/api/yearbook/update", { id: bookId, template_id: "memory-book" });
      expect(res.ok(), `update ${res.status()}`).toBe(true);
      await page.goto(`/yearbooks/${bookId}/note-links`);
      await page.getByLabel("Name (optional)").fill("Class chat");
      await page.getByRole("button", { name: "Create link" }).click();
      await expect(page.getByRole("heading", { name: "Your link is ready" })).toBeVisible();
      const shown = await page.getByLabel("Link for Class chat").inputValue();
      secrets.push(shown, new URL(shown).pathname);
      const link = new URL(shown).pathname;
      for (const who of NOTES) {
        await friend.goto(link);
        await friend.getByLabel(/^Your name/).fill(who);
        await friend.getByLabel(/^Your message/).fill(`Chúc bạn luôn vui! ${who}`);
        const sent = friend.waitForResponse((r) => r.url().includes("/api/public-collect/submit-note") && r.ok());
        await friend.getByRole("button", { name: "Send my note" }).click();
        await sent;
      }
      await page.goto(`/yearbooks/${bookId}/notes`);
      await expect(page.getByText(`${NOTES.length} waiting for you · 0 in your book`)).toBeVisible();
      for (const who of NOTES) await page.getByRole("button", { name: `Approve ${who}` }).click();
      await page.getByRole("button", { name: /^Approved/ }).click();
      await expect(page.locator(".inbox-card")).toHaveCount(NOTES.length);
    });

    /** The whole editing story on the template the book has now. */
    const story = async (template) => {
      const label = template === "navy-classic" ? "Navy Classic" : "Memory Book";
      let notesPages = 0;
      await step(`${label}: cover title moved, resized and rotated; a Vietnamese text, a sticker and an own image added`, async () => {
        await openEditor("cover");
        await showPalette();
        await press(page.getByRole("button", { name: "Cover title", exact: true }));
        for (const [name, value] of [["Left", "12"], ["Width", "50"], ["Rotation (degrees)", "5"]]) {
          const done = saved();
          await num(name).fill(value);
          await num(name).press("Enter");
          await done;
        }
        await press(page.getByRole("toolbar").getByRole("button", { name: "Add text" }));
        await expect(page.getByRole("textbox", { name: "Text of the box", exact: true })).toBeFocused();
        await page.keyboard.type(VI_TEXT);
        await page.keyboard.press("Escape");
        await expect(stage().locator("[data-layout-item] .bk-fit")).toHaveText(VI_TEXT);
        await palette().getByRole("tab", { name: "Stickers" }).click().catch(() => {}); // the first tab is already the stickers on most widths
        await palette().getByRole("searchbox").fill("ngoan ngoeo");
        await press(palette().getByRole("button", { name: "Squiggle line" }));
        await palette().getByRole("tab", { name: "My images" }).click();
        await palette().getByTestId("design-file").setInputFiles({ name: "holiday.png", mimeType: "image/png", buffer: png(400, 300, 1) });
        await press(palette().getByRole("button", { name: "Add image 1 of 1" }));
        await expect.poll(async () => ((await storedPage("cover")).items ?? []).map((i) => i.t).sort(), { timeout: 20_000 }).toEqual(["asset", "image", "text"]);
        const title = (await storedPage("cover")).overrides["cover.title"];
        expect(title).toMatchObject({ x: 120, w: 500, r: 5 });
      });

      await step(`${label}: undo and redo`, async () => {
        // the picture is still the selected item (it was added last); its width is the edit
        const width = async () => ((await storedPage("cover")).items ?? []).find((i) => i.t === "image").w;
        const w0 = await width();
        let done = saved();
        await num("Width").fill("40");
        await num("Width").press("Enter");
        await done;
        expect(await width()).toBe(400);
        done = saved();
        await press(page.getByRole("toolbar").getByRole("button", { name: "Undo", exact: true }));
        await done;
        expect(await width()).toBe(w0);
        done = saved();
        await press(page.getByRole("toolbar").getByRole("button", { name: "Redo", exact: true }));
        await done;
        expect(await width()).toBe(400);
        expect(((await storedPage("cover")).items ?? []).length, "undo and redo kept the three items").toBe(3);
      });

      await step(`${label}: a shipped template on the notes pages changes every notes page`, async () => {
        await expect(page.locator("[data-save-state='saved']")).toBeVisible();
        await openEditor("notes");
        expect(await storedPage("notes")).toEqual({});
        await press(templatesButton());
        await expect(page.getByRole("group", { name: "Templates", exact: true })).toBeVisible();
        await expect(page.getByText("A notes template applies to every notes page.")).toBeVisible();
        const shipped = page.getByRole("group", { name: "Shipped" }).locator(".ed-tpl-pick");
        expect(await shipped.count(), "the book's template ships page templates for the notes pages").toBeGreaterThan(0);
        await press(shipped.first());
        const applied = applyResponse();
        await press(page.getByRole("button", { name: "Apply to every notes page" }));
        if (await confirmApply().isVisible().catch(() => false)) await press(confirmApply());
        expect((await applied).status()).toBe(200);
        await expect(templatesButton()).toBeFocused();
        const notes = await storedPage("notes");
        expect(Object.keys(notes).length, "the notes kind has a layout now").toBeGreaterThan(0);
        const first = await layer();
        const m = /Page 3 of (\d+): Notes page 1/.exec((await position().textContent()) ?? "");
        expect(m, `on the first notes page: ${await position().textContent()}`).not.toBeNull();
        notesPages = Number(m[1]) - 3; // cover, profile, notes..., back
        expect(notesPages).toBe(template === "navy-classic" ? 2 : NOTES.length);
        for (let i = 2; i <= notesPages; i++) {
          await press(page.getByRole("button", { name: "Next page" }));
          await expect(position()).toHaveText(new RegExp(`Notes page ${i}$`));
          expect(await layer(), `notes page ${i} has the same template items as page 1`).toBe(first);
        }
      });

      await step(`${label}: the profile page saved as an own template, taken apart, applied back`, async () => {
        await openEditor("profile");
        await showPalette();
        await press(page.getByRole("toolbar").getByRole("button", { name: "Add text" }));
        await expect(page.getByRole("textbox", { name: "Text of the box", exact: true })).toBeFocused();
        await page.keyboard.type(PROFILE_TEXT);
        await page.keyboard.press("Escape");
        await expect.poll(async () => ((await storedPage("profile")).items ?? []).length, { timeout: 20_000 }).toBe(1);
        const before = (await storedPage("profile")).items[0];
        await page.waitForTimeout(2000); // nothing of the editing waits to be autosaved
        await press(templatesButton());
        await press(page.getByRole("button", { name: "Save this page as a template" }));
        const dialog = page.getByRole("dialog", { name: "Save this page as a template" });
        await dialog.getByLabel("Template name").fill(OWN);
        const created2 = page.waitForResponse((r) => r.url().includes("/api/book-layout/template-create") && r.request().method() === "POST");
        await dialog.getByLabel("Template name").press("Enter");
        expect((await created2).status()).toBe(200);
        await expect(dialog).toHaveCount(0);
        // take the page apart: the text is deleted
        await page.keyboard.press("Escape");
        await press(page.getByRole("button", { name: `Text: ${PROFILE_TEXT}`, exact: true }));
        const done = saved();
        await press(page.getByRole("button", { name: "Delete", exact: true }));
        await done;
        expect((await storedPage("profile")).items ?? []).toHaveLength(0);
        await expect(stage()).not.toContainText(PROFILE_TEXT);
        // apply the own template back
        if (!(await page.getByRole("group", { name: "Templates", exact: true }).isVisible())) await press(templatesButton());
        await press(page.getByRole("button", { name: new RegExp(`^Template ${OWN},`) }));
        const applied = applyResponse();
        await press(page.getByRole("button", { name: "Apply to the profile page" }));
        if (await confirmApply().isVisible().catch(() => false)) await press(confirmApply());
        expect((await applied).status()).toBe(200);
        await expect(stage()).toContainText(PROFILE_TEXT);
        const after = (await storedPage("profile")).items;
        expect(after).toHaveLength(1);
        expect(after[0]).toMatchObject({ t: "text", text: PROFILE_TEXT, x: before.x, w: before.w, h: before.h, cy: before.cy });
      });

      let finalLayout;
      await step(`${label}: History restores the version from before the notes template`, async () => {
        await press(page.getByRole("toolbar").getByRole("button", { name: "History" }));
        const rows = page.getByRole("group", { name: "Earlier versions" }).getByRole("button");
        const n = await rows.count();
        expect(n, "History lists versions").toBeGreaterThan(0);
        let target = -1;
        for (let i = 0; i < n; i++) if (/Before template/.test((await rows.nth(i).getAttribute("aria-label")) ?? (await rows.nth(i).textContent()) ?? "")) target = i; // the oldest one
        expect(target, "History has a 'Before template' row").toBeGreaterThanOrEqual(0);
        await press(rows.nth(target));
        await expect(page.getByRole("status").filter({ hasText: "Previewing a version from" })).toBeVisible();
        await press(page.getByRole("button", { name: "Restore this version" }));
        const restored = page.waitForResponse((r) => r.url().includes("/api/book-layout/restore"));
        await press(page.getByRole("dialog").getByRole("button", { name: "Restore this version" }));
        expect((await restored).status()).toBe(200);
        await expect(page.getByText("Version restored.", { exact: true }).first()).toBeVisible();
        finalLayout = (await stored()).layout;
        expect(finalLayout.pages.notes ?? {}, "the notes template is gone").toEqual({});
        expect((finalLayout.pages.profile?.items ?? []).length, "the profile text is gone").toBe(0);
        expect(finalLayout.pages.cover.items, "the cover keeps its three items").toHaveLength(3);
        expect(finalLayout.pages.cover.overrides["cover.title"]).toMatchObject({ x: 120, r: 5 });
      });

      await step(`${label}: Done, Print gets the PDF, reload shows the same layout`, async () => {
        await press(page.getByRole("button", { name: "Done" }));
        await expect(page).toHaveURL(new RegExp(`/yearbooks/${bookId}$`));
        await openEditor("cover");
        const link = page.getByRole("link", { name: "Print or save as PDF" });
        const popup = page.context().waitForEvent("page");
        await press(link);
        const tab = await popup;
        await tab.waitForURL(/\/book$/, { timeout: STEP_MS });
        await tab.waitForFunction(() => document.documentElement.dataset.bookReady, null, { timeout: STEP_MS });
        const file = path.join(mkdtempSync(path.join(os.tmpdir(), "e2e-s2-")), "book.pdf");
        await savePdf(tab, file);
        const info = inspect(file);
        expect(info.pages, "cover, profile, notes pages, back").toBe(3 + notesPages);
        expect(info.allFontsEmbedded, "every font is embedded").toBe(true);
        expect(squash(text(file)), "the PDF carries the Vietnamese text").toContain(squash(VI_TEXT));
        await tab.close();
        await page.reload();
        await expect(page.locator(".ed-pagebox .bk-sheet[data-page='cover']")).toBeVisible();
        await page.evaluate(() => document.fonts.ready);
        expect((await stored()).layout, "the same layout after a reload").toEqual(finalLayout);
        await expect(stage().locator("[data-layout-item] .bk-fit")).toHaveText(VI_TEXT);
      });
    };

    await story("memory-book");

    await step("the same book switches to Navy Classic (the layout is reset)", async () => {
      const res = await post(page, "/api/yearbook/update", { id: bookId, template_id: "navy-classic", school_name: "THPT Chu Văn An", class_name: "12C", graduation_year: 2026, motto: "Học, học nữa, học mãi" });
      expect(res.ok(), `update ${res.status()} ${await res.text()}`).toBe(true);
      expect((await stored()).layout, "a template change removes the layout").toBeNull();
    });
    await story("navy-classic");

    // the class: the student joins, edits the class page; a second member has no layout; the admin's class book
    const admin = await (await ctx()).newPage();
    const other = await (await ctx()).newPage();
    let invitePath = "";
    let classId = "";
    const classBook = { s: "", t: "" };
    const LAYOUT_TEXT = `Chữ của lớp ${RUN}`;

    await step("class 1. the admin signs up, creates the class and an invite", async () => {
      await signUp(admin, ADMIN);
      await admin.getByRole("link", { name: "Continue" }).click();
      await admin.goto("/classes/new");
      await admin.getByLabel("Class name").fill(CLASS_NAME);
      await admin.getByLabel("School").fill("Trường THPT Chuyên Lê Hồng Phong");
      await admin.getByRole("button", { name: "Create class" }).click();
      await expect(admin.getByRole("heading", { level: 1, name: CLASS_NAME })).toBeVisible();
      classId = new URL(admin.url()).pathname.split("/").pop();
      created.classId = classId;
      await admin.getByRole("button", { name: "Create invite link" }).click();
      const shown = await admin.getByLabel("Invite link", { exact: true }).inputValue();
      secrets.push(shown);
      invitePath = new URL(shown).pathname;
      secrets.push(invitePath, invitePath.split("/").pop());
      await admin.getByRole("button", { name: "Done, hide the link" }).click();
    });

    const joinAs = async (p, id) => {
      await expect(p.getByRole("heading", { name: `Join ${CLASS_NAME}` })).toBeVisible();
      await p.getByRole("button", { name: "Join this class" }).click();
      await expect(p.getByRole("heading", { name: `You joined ${CLASS_NAME}` })).toBeVisible();
      await p.getByRole("button", { name: "Create my page" }).click();
      await expect(p).toHaveURL(/\/yearbooks\/[0-9A-Z]{26}$/);
      classBook[id] = new URL(p.url()).pathname.split("/").pop();
    };
    await step("class 2. the student (logged in) and a second student join and make their class pages", async () => {
      await page.goto(invitePath);
      await joinAs(page, "s");
      await signUp(other, OTHER, invitePath);
      await other.getByRole("link", { name: "Continue" }).click();
      await joinAs(other, "t");
      for (const [p, id, name] of [[page, "s", STUDENT.name], [other, "t", OTHER.name]]) {
        await p.getByLabel("Full name").fill(name);
        const done = p.waitForResponse((r) => r.url().includes("/api/yearbook/update-profile") && r.ok());
        await p.getByRole("button", { name: "Save profile" }).click();
        await done;
        expect(classBook[id]).toMatch(/^[0-9A-Z]{26}$/);
      }
    });

    await step("class 3. the student edits the class page in the editor: the profile heading moves, a text is added", async () => {
      const id = classBook.s;
      const tpl = await post(page, "/api/yearbook/update", { id, template_id: "memory-book" }); // a class page starts on the class template, which has no free layout
      expect(tpl.ok(), `template change ${tpl.status()} ${await tpl.text()}`).toBe(true);
      await page.goto(`/yearbooks/${id}/editor?page=profile`);
      await expect(page.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
      await expect(page.locator(".ed-pagebox .bk-sheet[data-page='profile']")).toBeVisible();
      await page.evaluate(() => document.fonts.ready);
      await press(page.getByRole("button", { name: "Profile heading", exact: true }));
      let done = saved();
      await num("Left").fill("12");
      await num("Left").press("Enter");
      await done;
      done = saved();
      await press(page.getByRole("toolbar").getByRole("button", { name: "Add text" }));
      await expect(page.getByRole("textbox", { name: "Text of the box", exact: true })).toBeFocused();
      await page.keyboard.type(LAYOUT_TEXT);
      await page.keyboard.press("Escape");
      await done;
      await expect.poll(async () => JSON.stringify((await stored(page, id)).layout ?? {}), { timeout: 20_000 }).toContain(LAYOUT_TEXT);
      await press(page.getByRole("button", { name: "Done" }));
    });

    await step("class 4. the admin's class book carries the member's layout; the member without one is unchanged", async () => {
      const cb = (await (await admin.request.get(`/api/class-book/get?class_id=${classId}`)).json()).data;
      const ms = cb.members.find((m) => m.name === STUDENT.name);
      const mt = cb.members.find((m) => m.name === OTHER.name);
      expect(JSON.stringify(ms.layout)).toContain(LAYOUT_TEXT);
      expect(ms.layout.pages.profile.overrides["profile.heading"].x).toBe(120);
      expect(mt.layout, "the other member has no layout").toBeNull();
      await admin.goto(`/classes/${classId}/book`);
      await admin.waitForFunction(() => document.documentElement.dataset.bookReady);
      const sheets = admin.locator(".bk-sheet");
      await expect(sheets.filter({ hasText: LAYOUT_TEXT })).toHaveCount(1);
      await expect(sheets.filter({ hasText: LAYOUT_TEXT }).locator("[data-layout-id='profile.heading']")).toHaveCount(1);
      await expect(admin.locator("[data-layout-item]")).toHaveCount(1); // the student's text only: the other member's pages carry no layout item
      await expect(admin.locator(".bk-sheet", { hasText: OTHER.name }).locator("[data-layout-id], [data-layout-item]")).toHaveCount(0);
      for (const [who, c] of [["student", page], ["other member", other]]) {
        const r = await c.request.get(`/api/class-book/get?class_id=${classId}`);
        expect(r.status(), `${who} reads class-book/get`).toBe(403);
        expect(await r.text()).not.toContain(LAYOUT_TEXT);
      }
    });
  } catch (e) {
    if (e instanceof Error) {
      const scrub = (s) => secrets.filter(Boolean).reduce((out, token) => out.replaceAll(token, "<token>"), s);
      e.message = scrub(e.message);
      if (e.stack) e.stack = scrub(e.stack);
    }
    throw e;
  } finally {
    await cleanUp();
  }
});
