// The whole editor with the keyboard only (T-182, docs/editor-accessibility.md): manage page -> editor -> every region by Tab -> move, resize, rotate, reorder and delete an
// item by keys -> add a text and a sticker -> undo and redo -> Templates (preview, apply, save a page as a template) -> History (preview, restore) -> Done.
// No mouse: after the set-up (account and book through the UI and the API) every step uses `page.keyboard` only. Each Tab stop is checked for a visible focus ring, the order of the stops is
// asserted, no panel or dialog traps the keyboard (Escape or a button always leaves, and focus returns to the opener).
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 120_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Đặng Thị Bích Ngọc", email: `e2e-kbd-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9` };
const TITLE = `Lớp 11A2 – Bàn phím ${RUN}`;
const VI_TEXT = "Chúc các bạn thật nhiều niềm vui";
const TEMPLATE_NAME = "Bìa bàn phím";

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

async function cleanUp() {
  if (!BASE) return;
  const api = await request.newContext({ baseURL: BASE, extraHTTPHeaders: { Origin: BASE } });
  try {
    const login = await api.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password } });
    if (!login.ok()) return;
    const list = await (await api.get("/api/yearbook/get-list")).json();
    for (const b of list.data?.items ?? []) {
      const t = await (await api.post("/api/book-layout/template-list", { data: { yearbook_id: b.id, kind: "cover" } })).json();
      for (const x of t.data?.items ?? []) if (x.source === "own") await api.post("/api/book-layout/template-delete", { data: { id: x.id } });
      await api.post("/api/yearbook/delete", { data: { id: b.id } });
    }
  } catch (e) {
    console.warn(`clean-up failed (the run's data stays in the test database): ${e.message}`);
  } finally {
    await api.dispose();
  }
}

test.afterAll(cleanUp);
test.describe.configure({ mode: "serial" });

/** What the keyboard is on: the accessible name (aria-label, label, text), the tag and whether a focus ring is drawn (outline or box-shadow of the element or, for the
 *  hit area of a stage item, the selection outline of its box). Runs in the page. */
function focusInfo() {
  const el = document.activeElement;
  if (!el || el === document.body) return { name: "", tag: "body", ring: false };
  const label = el.getAttribute("aria-label") || [...(el.labels ?? [])].map((l) => l.textContent).join(" ") || el.textContent || el.getAttribute("title") || "";
  const shown = (cs) => (cs.boxShadow !== "none" && cs.boxShadow !== "") || (cs.outlineStyle !== "none" && cs.outlineWidth !== "0px");
  const box = el.closest(".ed-box.is-selected");
  return { name: label.replace(/\s+/g, " ").trim(), tag: el.tagName.toLowerCase(), ring: shown(getComputedStyle(el)) || (!!box && shown(getComputedStyle(box))), cls: el.className };
}

/** Presses Tab (Shift+Tab when `back`) until the focus is on the control whose name matches; checks the ring on every stop passed and returns the names in order. */
async function tabTo(page, name, { max = 70, back = false, wrap = false } = {}) {
  const seen = [];
  for (let i = 0; i < max; i++) {
    await page.keyboard.press(back ? "Shift+Tab" : "Tab");
    const f = await page.evaluate(focusInfo);
    seen.push(f.name);
    if (f.tag === "body" && wrap) continue; // Tab left the page and comes back at its top
    expect(f.ring, `no visible focus ring on "${f.name}" (${f.tag}.${f.cls}); stops so far: ${seen.join(" | ")}`).toBe(true);
    if (typeof name === "function" ? await page.evaluate(name) : name instanceof RegExp ? name.test(f.name) : f.name === name) return seen;
  }
  throw new Error(`Tab never reached "${name}"; stops: ${seen.join(" | ")}`);
}
const focused = (page) => page.evaluate(focusInfo);
const live = (page) => page.locator(".visually-hidden[role=status]");
const num = (page, name) => page.getByRole("spinbutton", { name, exact: true });
const savedLayout = (page) => page.waitForResponse((r) => r.url().includes("/api/book-layout/save") && r.request().method() === "POST" && r.ok());
const stored = async (page) => (await (await page.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json()).data;

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
  await step("create the book on Memory Book (A5, Vietnamese)", async () => {
    await owner.goto("/yearbooks/new");
    await owner.getByLabel("Title").fill(TITLE);
    await owner.getByLabel("Yearbook language").selectOption("vi");
    await owner.getByLabel("Page size").selectOption("A5");
    await owner.getByRole("button", { name: "Create yearbook" }).click();
    await expect(owner.getByRole("heading", { level: 1, name: TITLE })).toBeVisible();
    bookId = new URL(owner.url()).pathname.split("/").pop();
    const res = await owner.request.post("/api/yearbook/update", { data: { id: bookId, template_id: "memory-book" }, headers: { Origin: BASE } });
    expect(res.ok(), `template change ${res.status()}`).toBe(true);
  });
});

test("keyboard only: the whole editor", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  test.setTimeout(240_000);
  const page = await (await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } })).newPage();
  const login = await page.request.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password }, headers: { Origin: BASE } });
  expect(login.ok(), `login ${login.status()}`).toBe(true);
  const key = (k) => page.keyboard.press(k);

  await step("1. manage page: Tab to 'Edit page by page', Enter opens the editor", async () => {
    await page.goto(`/yearbooks/${bookId}`);
    await expect(page.getByRole("heading", { name: "Design the pages" })).toBeVisible();
    await tabTo(page, "Edit page by page");
    await key("Enter");
    await expect(page).toHaveURL(new RegExp(`/yearbooks/${bookId}/editor$`));
    await expect(page.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
  });

  await step("2. the Tab order of the editor: toolbar, page flow, page strip, stage bar, items; every stop has a focus ring", async () => {
    const seen = await tabTo(page, "Back to customise", { wrap: true }); // a route change leaves the Tab start where the link was (finding)
    expect(seen.length).toBeLessThan(40);
    const order = (await tabTo(page, "Cover sticker")).slice(0, -1);
    expect(order).toEqual([
      "Add text", "Add to page", "History", "Templates", "Reset all layout", "Keyboard shortcuts", "Previous page", "Next page", "Print or save as PDF", "Done",
      "Page 1 of 3: Cover", "Zoom out", "Zoom in", "Fit to window", "Safe margin", "Cover title", "Cover photo",
    ]);
    // the selected item is described and the panel follows it
    await expect(page.getByRole("complementary", { name: "Properties" }).getByRole("heading", { level: 2 })).toHaveText("Cover sticker");
    await expect(live(page)).toHaveText("Selected: Cover sticker");
  });

  const value = async (name) => parseFloat(await num(page, name).inputValue());
  await step("3. move, resize and rotate the cover title with the keys; the panel and the live region follow; the change is saved", async () => {
    await tabTo(page, "Cover title", { back: true, max: 3 });
    await expect(live(page)).toHaveText("Selected: Cover title");
    const left = await value("Left");
    const top = await value("Top");
    const wide = await value("Width");
    const turn = await value("Rotation (degrees)");
    const saved = savedLayout(page);
    for (let i = 0; i < 3; i++) await key("ArrowRight");
    await expect.poll(() => value("Left")).toBeCloseTo(left + 0.3, 0);
    await expect(live(page)).toContainText("Moved right 1.");
    await key("Shift+ArrowDown");
    await expect.poll(() => value("Top")).toBeGreaterThan(top + 0.5);
    await key("Alt+Shift+ArrowRight");
    await expect.poll(() => value("Width")).toBeGreaterThan(wide + 0.5);
    await expect(live(page)).toContainText("Size ");
    await key(".");
    await expect.poll(() => value("Rotation (degrees)")).toBe(turn + 1);
    await expect(live(page)).toContainText("Rotated to ");
    await saved;
    const o = (await stored(page)).layout.pages.cover.overrides["cover.title"];
    expect(o.r).toBe(turn + 1);
  });

  await step("4. add a text with the toolbar button, type Vietnamese, Escape returns to the box, move it, send it to the back, delete it, undo brings it back", async () => {
    await tabTo(page, "Add text", { back: true });
    await key("Enter");
    const field = page.getByRole("textbox", { name: "Text of the box", exact: true });
    await expect(field).toBeFocused();
    await expect(live(page)).toContainText("Text box added.");
    await key("Control+a");
    await page.keyboard.type(VI_TEXT);
    await key("Escape");
    await expect.poll(async () => (await focused(page)).cls).toBe("ed-hit");
    await expect(page.locator(".ed-pagebox [data-layout-item] .bk-fit")).toHaveText(VI_TEXT);
    const left = await value("Left");
    await key("ArrowLeft");
    await key("ArrowLeft");
    await expect.poll(() => value("Left")).toBeCloseTo(left - 0.2, 0);
    await key("Delete");
    await expect(page.locator(".ed-pagebox [data-layout-item]")).toHaveCount(0);
    await expect(live(page)).toHaveText("Deleted");
    await key("Control+z");
    await expect(page.locator(".ed-pagebox [data-layout-item] .bk-fit")).toHaveText(VI_TEXT);
  });

  await step("5. open the palette from the toolbar, add a sticker with Enter (it is selected and focused), reorder it with the panel, resize it with the keys", async () => {
    await tabTo(page, "Add to page", { back: true });
    await expect(page.getByRole("button", { name: "Add to page" })).toHaveAttribute("aria-expanded", "false");
    await key("Enter");
    const palette = page.getByRole("complementary", { name: "Add to the page" });
    await expect(palette).toBeVisible();
    await expect(page.getByRole("button", { name: "Add to page" })).toHaveAttribute("aria-expanded", "true");
    await tabTo(page, () => !!document.activeElement?.closest(".ed-grid-tiles")); // the tiles are one Tab stop (arrows move inside)
    const tile = (await focused(page)).name;
    await key("ArrowRight");
    expect((await focused(page)).name).not.toBe(tile);
    await key("Enter");
    await expect(live(page)).toContainText("Added: ");
    await expect.poll(async () => (await focused(page)).cls).toBe("ed-hit");
    await expect(page.locator(".ed-pagebox [data-layout-item]")).toHaveCount(2);
    const w = await value("Width");
    await key("Alt+Shift+ArrowRight");
    await expect.poll(() => value("Width")).toBeGreaterThan(w + 0.5);
    await tabTo(page, "Send to back");
    await key("Enter");
    expect((await focused(page)).name, "the button that just became disabled does not take the focus with it").toBe("Bring forward");
    // the reorder is real: the text is drawn above the sticker now (items are in drawing order)
    await expect.poll(async () => (await stored(page)).layout.pages.cover.items.map((i) => i.t)).toEqual(["asset", "text"]);
  });

  const order = async () => (await stored(page)).layout.pages.cover.items.map((i) => i.t);
  await step("6. undo and redo from the toolbar buttons, with Enter", async () => {
    await tabTo(page, "Undo", { back: true });
    await key("Enter");
    await expect.poll(order).toEqual(["text", "asset"]);
    expect((await focused(page)).name).toBe("Undo");
    await tabTo(page, "Redo");
    await key("Enter");
    await expect.poll(order).toEqual(["asset", "text"]);
    await expect(page.getByRole("button", { name: "Redo" })).toBeDisabled();
    expect((await focused(page)).name, "Redo became disabled: the focus moved to its neighbour").toBe("Undo");
  });

  const dialog = (name) => page.getByRole("dialog", { name });
  await step("7. Templates: open, preview a card, Apply asks (dialog: focus on Cancel, Tab stays inside, Escape returns), apply, focus returns to the toolbar button", async () => {
    await tabTo(page, "Templates");
    await key("Enter");
    const panel = page.getByRole("complementary", { name: "Templates for the cover" });
    await expect(panel).toBeFocused();
    await expect(page.getByRole("button", { name: "Templates", exact: true })).toHaveAttribute("aria-expanded", "true");
    await tabTo(page, /^Template /);
    await key("Enter");
    await expect(live(page)).toHaveText("Previewing a template. Your page is unchanged.");
    await tabTo(page, "Apply to the cover", { back: true });
    await key("Enter");
    const confirm = dialog("Apply this template?");
    await expect(confirm).toBeVisible();
    await expect(confirm).toHaveAttribute("aria-modal", "true");
    expect((await focused(page)).name).toBe("Cancel");
    await key("Tab");
    expect((await focused(page)).name).toBe("Apply template");
    await key("Tab");
    expect((await focused(page)).name).toBe("Cancel"); // the dialog keeps Tab inside
    await key("Escape");
    await expect(confirm).toHaveCount(0);
    expect((await focused(page)).name).toBe("Apply to the cover");
    await key("Enter");
    await key("Tab");
    await key("Enter");
    await expect(confirm).toHaveCount(0);
    await expect(live(page)).toContainText("Template applied to the cover.");
    await expect(panel).toHaveCount(0);
    expect((await focused(page)).name).toBe("Templates");
  });

  await step("8. Save this page as a template: the name field has the focus, Enter saves, the new card has the focus; Escape closes the panel and returns to the button", async () => {
    await key("Enter"); // Templates is focused
    const panel = page.getByRole("complementary", { name: "Templates for the cover" });
    await expect(panel).toBeFocused();
    await tabTo(page, "Save this page as a template");
    await key("Enter");
    const save = dialog("Save this page as a template");
    await expect(save).toBeVisible();
    expect((await focused(page)).name).toBe("Template name");
    await key("Control+a");
    await page.keyboard.type(TEMPLATE_NAME);
    await key("Enter");
    await expect(save).toHaveCount(0);
    await expect.poll(async () => (await focused(page)).name).toMatch(new RegExp(`^Template ${TEMPLATE_NAME},`));
    await key("Escape");
    await expect(panel).toHaveCount(0);
    expect((await focused(page)).name).toBe("Templates");
  });

  await step("9. History: open, preview a version with Enter, Restore asks, Escape cancels, restore; Escape closes the panel and returns to the button", async () => {
    await tabTo(page, "History", { back: true });
    await key("Enter");
    const panel = page.getByRole("complementary", { name: "Version history" });
    await expect(panel).toBeFocused();
    await tabTo(page, /,/); // the first row: "<time>, <kind>, <n items>"
    await key("Enter");
    await expect(live(page)).toHaveText("Previewing an earlier version. Your current work is unchanged.");
    await tabTo(page, "Restore this version", { back: true });
    await key("Enter");
    const confirm = dialog("Restore this version?");
    await expect(confirm).toBeVisible();
    expect((await focused(page)).name).toBe("Cancel");
    await key("Escape");
    await expect(confirm).toHaveCount(0);
    expect((await focused(page)).name).toBe("Restore this version");
    await key("Enter");
    await key("Tab");
    await key("Enter");
    await expect(live(page)).toContainText("Version restored.");
    await key("Escape");
    await expect(panel).toHaveCount(0);
    await expect.poll(async () => (await focused(page)).name).toBe("History");
  });

  await step("10. Done with the keyboard leaves the editor for the manage page", async () => {
    await tabTo(page, "Done");
    await key("Enter");
    await expect(page).toHaveURL(new RegExp(`/yearbooks/${bookId}$`));
    await expect(page.getByRole("heading", { name: "Design the pages" })).toBeVisible();
  });
});
