// The layout editor on Navy Classic end to end (T-166, docs/e2e.md "Navy Classic in the layout editor"): a student with a Navy Classic book opens the editor,
// moves and rotates the cover title, hides the motto, adds a sticker and a Vietnamese text, sets a background colour, reloads and finds the same page, opens the
// print route and finds the same boxes, then switches the template to Memory Book and loses the layout. Pixel parity of the print is the print-check's job.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
// Credentials: the registered fixed email code (DEV-SHORTCUT(otp), SMEM_ENV=test only).
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 90_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Lý Thị Kim Chi", email: `e2e-navy-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9` };
const TITLE = `Lớp 12A2 – Navy ${RUN}`;
const VI_TEXT = "Chúc mừng tốt nghiệp";
const LIGHT = "rgb(247, 243, 234)"; // --nc-light of the navy theme: the colour of a new text on the dark cover
const GOLD = "rgb(201, 168, 106)"; // --nc-gold

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
    for (const y of list.data?.items ?? []) await api.post("/api/yearbook/delete", { data: { id: y.id } });
  } catch (e) {
    console.warn(`clean-up failed (the run's data stays in the test database): ${e.message}`);
  } finally {
    await api.dispose();
  }
}

test.afterAll(cleanUp);

test("Navy Classic in the layout editor: move, hide, add, background, reload, print route, template switch", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const page = await (await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1500, height: 1000 } })).newPage();
  let bookId = "";
  const api = (p, data) => page.request.post(p, { data, headers: { Origin: BASE } });
  const num = (name) => page.getByRole("spinbutton", { name, exact: true });
  const saved = () => page.waitForResponse((r) => r.url().includes("/api/book-layout/save") && r.request().method() === "POST" && r.ok());
  const stored = async () => (await (await page.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json()).data;
  const storedCover = async () => (await stored()).layout?.pages?.cover ?? {};
  const palette = () => page.getByRole("complementary", { name: "Add to the page" });
  const cover = () => page.locator(".ed-pagebox .bk-sheet[data-page='cover']");
  const openEditor = async () => {
    await page.goto(`/yearbooks/${bookId}/editor`);
    await expect(page.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
    await expect(cover()).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    const toggle = page.getByRole("toolbar").getByRole("button", { name: "Add to page" });
    if ((await toggle.getAttribute("aria-expanded")) === "false") await toggle.click();
    await expect(palette()).toBeVisible();
  };
  /** Every moved element and every item of the cover: its box as fractions of the sheet, keyed by layout id or item id. */
  const boxes = (root) =>
    root.evaluate((sheet) => {
      const s = sheet.getBoundingClientRect();
      const out = {};
      for (const e of sheet.querySelectorAll(".mb-ov[data-layout-id], [data-layout-item]")) {
        const b = e.getBoundingClientRect();
        out[e.dataset.layoutId ?? e.dataset.layoutItem] = [(b.left - s.left) / s.width, (b.top - s.top) / s.width, b.width / s.width, b.height / s.width];
      }
      return out;
    });

  await step("1. register, create a book, make it a Navy Classic with a school, a class, a year and a motto", async () => {
    await page.goto("/register");
    await page.getByLabel("Your name").fill(OWNER.name);
    await page.getByLabel("Email").fill(OWNER.email);
    await page.getByLabel("Password", { exact: true }).fill(OWNER.password);
    await page.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
    const registered = page.waitForResponse((r) => r.url().includes("/api/auth/register") && r.request().method() === "POST");
    await page.getByRole("button", { name: "Create account" }).click();
    if ((await registered).status() === 429) throw new Error("registration is rate limited: run `make reset-limits SMEM_ENV=test` (make e2e does)");
    await expect(page.getByRole("heading", { name: "Verify your email" })).toBeVisible();
    await page.getByLabel("6-digit code").fill(OTP);
    await expect(page.getByText("Your email address is verified.")).toBeVisible();
    await page.goto("/yearbooks/new");
    await page.getByLabel("Title").fill(TITLE);
    await page.getByLabel("Yearbook language").selectOption("vi");
    await page.getByLabel("Page size").selectOption("A5");
    await page.getByRole("button", { name: "Create yearbook" }).click();
    await expect(page.getByRole("heading", { level: 1, name: TITLE })).toBeVisible();
    bookId = new URL(page.url()).pathname.split("/").pop();
    const res = await api("/api/yearbook/update", { id: bookId, template_id: "navy-classic", school_name: "THPT Chu Văn An", class_name: "12A2", graduation_year: 2026, motto: "Học, học nữa, học mãi" });
    expect(res.ok(), `update ${res.status()} ${await res.text()}`).toBe(true);
  });

  await step("2. the editor opens on Navy Classic: its page kinds and its elements", async () => {
    await openEditor();
    await expect(page.getByRole("navigation", { name: "Pages" }).getByRole("button")).toHaveCount(3); // no notes yet: cover, profile, back
    for (const name of ["School line", "Class-of year", "Cover title", "Motto", "Class name"]) await expect(page.getByRole("button", { name, exact: true })).toBeVisible();
    await page.getByRole("button", { name: /^Page 2 of 3/ }).click();
    for (const name of ["Profile small heading", "Profile heading", "Profile photo"]) await expect(page.getByRole("button", { name, exact: true })).toBeVisible();
    await page.getByRole("button", { name: /^Page 3 of 3/ }).click();
    for (const name of ["Thank-you line", "Back cover footer"]) await expect(page.getByRole("button", { name, exact: true })).toBeVisible();
    await page.getByRole("button", { name: /^Page 1 of 3/ }).click();
    await expect(page.getByTestId("safe-margin")).toBeVisible();
  });

  await step("3. move and rotate the cover title, hide the motto", async () => {
    await page.getByRole("button", { name: "Cover title", exact: true }).click();
    let done = saved();
    await num("Left").fill("12");
    await num("Left").press("Enter");
    await done;
    done = saved();
    await num("Rotation (degrees)").fill("5");
    await num("Rotation (degrees)").press("Enter");
    await done;
    await page.getByRole("button", { name: "Motto", exact: true }).click();
    done = saved();
    await page.getByRole("button", { name: "Hide", exact: true }).click();
    await done;
    const c = await storedCover();
    expect(c.overrides["cover.title"]).toMatchObject({ x: 120, r: 5 });
    expect(c.overrides["cover.motto"]).toMatchObject({ hidden: true });
    await expect(cover().locator(".nc-motto")).toHaveCount(0);
    await expect(cover().locator(".mb-ov[data-layout-id='cover.title']")).toHaveCount(1);
  });

  await step("4. add a sticker and a Vietnamese text (light on the dark cover), set a background colour", async () => {
    await palette().getByRole("searchbox").fill("ngoan ngoeo");
    await palette().getByRole("button", { name: "Squiggle line" }).click();
    await expect(page.locator(".ed-pagebox [data-layout-item]")).toHaveCount(1);
    await page.getByRole("toolbar").getByRole("button", { name: "Add text" }).click();
    const field = page.getByRole("textbox", { name: "Text of the box", exact: true });
    await expect(field).toBeFocused();
    await page.keyboard.type(VI_TEXT);
    await page.keyboard.press("Escape");
    await expect(page.locator(".ed-pagebox [data-layout-item] .bk-fit")).toHaveText(VI_TEXT);
    expect(await page.locator(".ed-pagebox [data-layout-item] .bk-fit").evaluate((e) => getComputedStyle(e).color)).toBe(LIGHT);
    await palette().getByRole("tab", { name: "Background" }).click();
    const done = saved();
    await palette().getByRole("radiogroup", { name: "Background colour" }).getByRole("radio", { name: "Gold" }).click();
    await done;
    await expect.poll(async () => (await storedCover()).bg, { timeout: 15_000 }).toEqual({ color: "gold" });
    await expect.poll(async () => (await storedCover()).items?.length, { timeout: 15_000 }).toBe(2);
    expect(await cover().locator(".nc-page").evaluate((e) => getComputedStyle(e).backgroundColor)).toBe(GOLD);
  });

  let layout;
  let editorBoxes;
  await step("5. reload: the same layout and the same page", async () => {
    await expect(page.locator("[data-save-state='saved']")).toBeVisible();
    layout = (await stored()).layout;
    await page.reload();
    await expect(cover()).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    expect((await stored()).layout).toEqual(layout);
    await expect(cover().locator(".nc-motto")).toHaveCount(0);
    await expect(page.locator(".ed-pagebox [data-layout-item]")).toHaveCount(2);
    expect(await cover().locator(".nc-page").evaluate((e) => getComputedStyle(e).backgroundColor)).toBe(GOLD);
    editorBoxes = await boxes(cover());
    expect(Object.keys(editorBoxes)).toContain("cover.title");
  });

  await step("6. the print route draws the same boxes, the same background and no motto", async () => {
    await page.goto(`/yearbooks/${bookId}/book`);
    const sheet = page.locator(".bk-sheet[data-page='cover']");
    await expect(sheet).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    await expect(sheet.locator(".nc-motto")).toHaveCount(0);
    await expect(sheet.locator("[data-layout-item] .bk-fit")).toHaveText(VI_TEXT);
    expect(await sheet.locator(".nc-page").evaluate((e) => getComputedStyle(e).backgroundColor)).toBe(GOLD);
    await expect.poll(async () => {
      const b = await boxes(sheet);
      if (Object.keys(b).sort().join() !== Object.keys(editorBoxes).sort().join()) return Infinity;
      return Math.max(...Object.keys(b).flatMap((k) => b[k].map((v, i) => Math.abs(v - editorBoxes[k][i]))));
    }, { timeout: 30_000 }).toBeLessThan(0.003); // 0.3 % of the page width, about 2 design px (the stage and the print scale the same box)
  });

  await step("7. switching to Memory Book removes the layout; the editor opens on the other template", async () => {
    const res = await api("/api/yearbook/update", { id: bookId, template_id: "memory-book" });
    expect(res.ok(), `template change ${res.status()}`).toBe(true);
    const got = await stored();
    expect(got.layout).toBeNull();
    expect(got.template_id).toBe("memory-book");
    await openEditor();
    await expect(page.getByRole("button", { name: "Cover sticker", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "School line", exact: true })).toHaveCount(0);
  });
});
