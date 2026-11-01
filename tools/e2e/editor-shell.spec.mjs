// The layout editor shell end to end (T-159, docs/e2e.md "The layout editor journey"): a student with a Memory Book opens the editor from the customise screen,
// selects the cover title, changes its width in the property panel, changes the rotation and undoes that with the keyboard, reloads and finds the saved layout;
// the handles of an item flush to the page edge are reachable on A5, A4 and Letter; below 768 px the editor does not mount.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
// Every step is a test.step with a hard limit; no fixed sleeps. Credentials: the registered fixed email code (DEV-SHORTCUT(otp), SMEM_ENV=test only).
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 60_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Đặng Thị Bích Ngọc", email: `e2e-editor-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9` };
const TITLE = `Lớp 10C3 – Bố cục ${RUN}`;

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

/** Deletes what the run created (the account stays). Never fails the test. */
async function cleanUp() {
  if (!BASE) return;
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
}

test.afterAll(cleanUp);

test("layout editor shell: open, select, change the width, undo, reload; handles at the page edge; narrow screens", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const owner = await (await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } })).newPage();
  let bookId = "";
  const num = (name) => owner.getByRole("spinbutton", { name, exact: true }); // a handle is also named "Resize from left"
  const editorUrl = () => `/yearbooks/${bookId}/editor`;
  const api = (p, data) => owner.request.post(p, { data, headers: { Origin: BASE } });
  const saved = () => owner.waitForResponse((r) => r.url().includes("/api/book-layout/save") && r.request().method() === "POST" && r.ok());
  const openEditor = async () => {
    await owner.goto(editorUrl());
    await expect(owner.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
    await expect(owner.locator(".ed-pagebox .bk-sheet[data-page='cover']")).toBeVisible();
  };
  /** The share of the page width the cover title's box takes on the stage. */
  const titleShare = () =>
    owner.evaluate(() => {
      const t = document.querySelector(".ed-pagebox [data-layout-id='cover.title']");
      const box = t?.classList.contains("mb-ov") ? t : t?.firstElementChild;
      const s = document.querySelector(".ed-pagebox .bk-sheet");
      return box && s ? box.getBoundingClientRect().width / s.getBoundingClientRect().width : null;
    });

  await step("1. register, create a Memory Book (A5, Vietnamese)", async () => {
    await owner.goto("/register");
    await owner.getByLabel("Your name").fill(OWNER.name);
    await owner.getByLabel("Email").fill(OWNER.email);
    await owner.getByLabel("Password", { exact: true }).fill(OWNER.password);
    await owner.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
    const registered = owner.waitForResponse((r) => r.url().includes("/api/auth/register") && r.request().method() === "POST");
    await owner.getByRole("button", { name: "Create account" }).click();
    if ((await registered).status() === 429) {
      throw new Error("registration is rate limited: run `make reset-limits SMEM_ENV=test` (make e2e does), or point SMEM_REDIS_URL at a fresh Redis db, then run again");
    }
    await expect(owner.getByRole("heading", { name: "Verify your email" })).toBeVisible();
    await owner.getByLabel("6-digit code").fill(OTP); // the sixth digit submits
    await expect(owner.getByText("Your email address is verified.")).toBeVisible();
    await owner.getByRole("link", { name: "Continue" }).click();
    await owner.goto("/yearbooks/new");
    await owner.getByLabel("Title").fill(TITLE);
    await owner.getByLabel("Yearbook language").selectOption("vi");
    await owner.getByLabel("Page size").selectOption("A5");
    await owner.getByRole("button", { name: "Create yearbook" }).click();
    await expect(owner.getByRole("heading", { level: 1, name: TITLE })).toBeVisible();
    bookId = new URL(owner.url()).pathname.split("/").pop();
    const res = await api("/api/yearbook/update", { id: bookId, template_id: "memory-book" });
    expect(res.ok(), `template change ${res.status()}`).toBe(true);
  });

  await step("2. the customise screen opens the editor: strip, stage and panel", async () => {
    await owner.goto(`/yearbooks/${bookId}/customise`);
    await owner.getByRole("link", { name: "Free layout" }).click();
    await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}/editor$`));
    await expect(owner.getByRole("heading", { name: "Free layout" })).toBeVisible();
    await expect(owner.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
    // no notes yet: cover, profile and back
    await expect(owner.getByRole("navigation", { name: "Pages" }).getByRole("button")).toHaveCount(3);
    await expect(owner.getByRole("button", { name: /^Page 1 of 3: Cover/ })).toHaveAttribute("aria-current", "page");
    await expect(owner.getByRole("heading", { name: "Page: Cover" })).toBeVisible();
    await expect(owner.getByTestId("safe-margin")).toBeVisible();
    await owner.getByRole("button", { name: /^Page 2 of 3/ }).click(); // arrow keys and clicks move between pages
    await expect(owner.locator(".ed-pagebox .bk-sheet[data-page='profile']")).toBeVisible();
    await owner.getByRole("button", { name: /^Page 1 of 3/ }).click();
  });

  await step("3. select the cover title and change its width in the panel; the page and the save follow", async () => {
    await owner.getByRole("button", { name: "Cover title" }).click();
    await expect(owner.getByRole("heading", { name: "Cover title" })).toBeVisible();
    const width = num("Width");
    const before = Number(await width.inputValue());
    expect(before, "the title is wide").toBeGreaterThan(50);
    const share0 = await titleShare();
    expect(Math.abs(share0 * 100 - before), "the panel shows the width the page draws").toBeLessThan(2);
    await width.fill("250"); // out of range: the old value comes back with a message (R4)
    await width.press("Enter");
    await expect(owner.getByRole("alert").filter({ hasText: "Between 1 and 200" })).toBeVisible();
    await expect(width).toHaveValue(String(before));
    const done = saved();
    await width.fill("40");
    await width.press("Enter");
    await done;
    await expect(owner.locator("[data-save-state='saved']")).toBeVisible();
    await expect.poll(titleShare).toBeGreaterThan(0.38);
    expect(await titleShare()).toBeLessThan(0.42);
    // the whole box was written: the other numbers did not jump
    await expect(num("Left")).toHaveValue(/^1[0-9]\.[0-9]$|^1[0-9]$/);
  });

  await step("4. change the rotation, then undo it from the keyboard; the undo is saved too", async () => {
    const rotation = num("Rotation (degrees)");
    let done = saved();
    await rotation.fill("4");
    await rotation.press("Enter");
    await done;
    await expect(rotation).toHaveValue("4");
    done = saved();
    await owner.keyboard.press("ControlOrMeta+z");
    await done;
    await expect(rotation).toHaveValue("0");
    await expect(num("Width")).toHaveValue("40");
    await expect(owner.getByRole("button", { name: "Redo" })).toBeEnabled();
  });

  await step("5. after a reload the saved layout is drawn: width 40, no rotation", async () => {
    await openEditor();
    await expect.poll(titleShare).toBeGreaterThan(0.38);
    await owner.getByRole("button", { name: "Cover title" }).click();
    await expect(num("Width")).toHaveValue("40");
    await expect(num("Rotation (degrees)")).toHaveValue("0");
    await expect(owner.getByRole("button", { name: "Undo" })).toBeDisabled(); // history is not saved, only the document
    // "reset this element" drops the override: the title is wide again and that is saved
    const done = saved();
    await owner.getByRole("button", { name: "Reset this element" }).click();
    await done;
    await expect.poll(titleShare).toBeGreaterThan(0.6);
  });

  await step("5b. 'Preview as PDF' shows the just-edited layout even when the save is slow (R2)", async () => {
    // every save takes 1.5 s; the student edits the width and clicks Preview at once, three times
    await owner.route("**/api/book-layout/save", async (route) => {
      await new Promise((r) => setTimeout(r, 1500));
      await route.continue();
    });
    try {
      await owner.getByRole("button", { name: "Cover title" }).click();
      for (const want of [35, 45, 55]) {
        await num("Width").fill(String(want));
        await num("Width").press("Enter");
        const popup = owner.context().waitForEvent("page");
        await owner.getByRole("link", { name: "Preview as PDF" }).click();
        const tab = await popup;
        await tab.waitForURL(/\/book$/, { timeout: STEP_MS });
        await tab.waitForSelector("html[data-book-ready]", { timeout: STEP_MS });
        const share = await tab.evaluate(() => {
          const el = document.querySelector("[data-page='cover'] [data-layout-id='cover.title']");
          const s = el?.closest(".bk-inner");
          return el && s ? el.getBoundingClientRect().width / s.getBoundingClientRect().width : null;
        });
        expect(share, `the tab shows width ${want}`).toBeGreaterThan(want / 100 - 0.02);
        expect(share).toBeLessThan(want / 100 + 0.02);
        await tab.close();
      }
    } finally {
      await owner.unroute("**/api/book-layout/save");
    }
    // back to the template's own place for the steps below
    const done = saved();
    await owner.getByRole("button", { name: "Reset this element" }).click();
    await done;
  });

  await step("6. the handles of an item flush to the page edge are reachable on A5, A4 and Letter", async () => {
    // an item whose box touches the right and the bottom edge; saved through the API (the palette that adds items is a later task)
    const got = await (await owner.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json();
    const flush = { id: "edgeStar01", t: "asset", asset: "sticker.gold-star", x: 850, w: 150, h: 150, cy: 1000, r: 0 };
    const res = await api("/api/book-layout/save", {
      yearbook_id: bookId,
      base_version: got.data.version,
      layout: { v: 1, pages: { cover: { items: [flush] } } },
    });
    expect(res.ok(), `layout save ${res.status()}`).toBe(true);
    for (const size of ["A5", "A4", "Letter"]) {
      const up = await api("/api/yearbook/update", { id: bookId, page_size: size });
      expect(up.ok(), `page size ${size}: ${up.status()}`).toBe(true);
      await openEditor();
      await owner.getByRole("button", { name: "Sticker", exact: true }).click();
      const probe = await owner.evaluate(() => {
        const handle = document.querySelector(".ed-h-se");
        const sheet = document.querySelector(".ed-pagebox .bk-sheet");
        const h = handle.getBoundingClientRect();
        const s = sheet.getBoundingClientRect();
        // the outer corner of the handle lies beyond the page; the page's clip must not hide it
        const x = h.right - 1, y = h.bottom - 1;
        const hit = document.elementFromPoint(x, y);
        return { outside: x > s.right && y > s.bottom, hitsHandle: hit === handle, sheetRight: s.right, x };
      });
      expect(probe.outside, `${size}: the handle sticks out of the page`).toBe(true);
      expect(probe.hitsHandle, `${size}: the outer corner of the handle is what a click would hit`).toBe(true);
    }
  });

  await step("7. below 768 px the editor does not mount", async () => {
    await owner.setViewportSize({ width: 700, height: 900 });
    await owner.goto(editorUrl());
    await expect(owner.getByText(/needs a screen at least 768 px wide/)).toBeVisible();
    await expect(owner.getByRole("toolbar", { name: "Editor toolbar" })).toHaveCount(0);
    await expect(owner.getByRole("link", { name: "Back to customise" })).toBeVisible();
  });
});
