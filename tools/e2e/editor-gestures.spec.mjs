// The layout editor's gestures end to end (T-160, docs/e2e.md "The layout editor journey"): drag, corner resize and rotate with the mouse measured against
// the pointer on A5, A4 and Letter at two zoom levels (at most 1 px), 44 x 44 px hit areas, the keyboard (move, resize, rotate, delete, undo), Escape and
// one undo step per gesture, an item flush to the page edge resized with the mouse, touch at 768 px (CDP touch events), and the frame rate of a drag
// among 100 items under a 4x CPU throttle.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
// Every step is a test.step with a hard limit; no fixed sleeps except the "nothing is sent" check of a cancelled gesture, which waits for the autosave's
// 1.5 s debounce to have passed. Credentials: the registered fixed email code (DEV-SHORTCUT(otp), SMEM_ENV=test only).
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 90_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Đặng Thị Bích Ngọc", email: `e2e-gest-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9` };
const TITLE = `Lớp 10C3 – Cử chỉ ${RUN}`;
const PAPERS = ["A5", "A4", "Letter"];
const ZOOMS = [100, 125];
const STAR = { id: "dragStar01", t: "asset", asset: "sticker.gold-star", x: 300, w: 150, h: 150, cy: 250, r: 0 };
const FLUSH = { id: "edgeStar01", t: "asset", asset: "sticker.gold-star", x: 850, w: 150, h: 150, cy: 1000, r: 0 };
const MAX_ERROR_PX = 1;

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
test.describe.configure({ mode: "serial" });

let bookId = "";
const numbers = []; // the measured errors, printed at the end and quoted in the PR

/** A page of the owner, signed in by the API (the first test registered the account). */
async function ownerPage(browser, viewport, extra = {}) {
  const ctx = await browser.newContext({ locale: "en-US", baseURL: BASE, viewport, ...extra });
  const page = await ctx.newPage();
  const login = await page.request.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password }, headers: { Origin: BASE } });
  expect(login.ok(), `login ${login.status()}`).toBe(true);
  return page;
}
const post = (page, path, data) => page.request.post(path, { data, headers: { Origin: BASE } });
async function saveLayout(page, items) {
  const got = await (await page.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json();
  const res = await post(page, "/api/book-layout/save", { yearbook_id: bookId, base_version: got.data.version, layout: { v: 1, pages: { cover: { items } } } });
  expect(res.ok(), `layout save ${res.status()}`).toBe(true);
}
async function openEditor(page, paper) {
  if (paper) {
    const up = await post(page, "/api/yearbook/update", { id: bookId, page_size: paper });
    expect(up.ok(), `page size ${paper}: ${up.status()}`).toBe(true);
  }
  await page.goto(`/yearbooks/${bookId}/editor`);
  await expect(page.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
  await expect(page.locator(".ed-pagebox .bk-sheet[data-page='cover']")).toBeVisible();
}
/** Brings the stage to a zoom level (percent) with the zoom buttons. */
async function setZoom(page, pct) {
  const cur = () => page.locator(".ed-pagebox").evaluate((el) => Number(el.dataset.zoom));
  for (let i = 0; i < 8 && (await cur()) !== pct; i++) {
    await page.getByRole("button", { name: (await cur()) < pct ? "Zoom in" : "Zoom out" }).click();
  }
  expect(await cur()).toBe(pct);
}
const drawn = (page, id) => page.locator(`.ed-pagebox [data-layout-item='${id}']`);
const box = (loc) =>
  loc.evaluate((el) => {
    const r = el.getBoundingClientRect();
    return { l: r.left, t: r.top, w: r.width, h: r.height, cx: r.left + r.width / 2, cy: r.top + r.height / 2 };
  });
const angleOf = (loc) =>
  loc.evaluate((el) => {
    const m = new DOMMatrix(getComputedStyle(el).transform);
    return (Math.atan2(m.b, m.a) * 180) / Math.PI;
  });
const scale = (page) => page.locator(".ed-pagebox .bk-sheet").evaluate((el) => el.getBoundingClientRect().width / 816); // screen px per design px
const centreOf = async (loc) => {
  const b = await box(loc);
  return [b.cx, b.cy];
};
const undoDisabled = (page) => page.getByRole("button", { name: "Undo" }).isDisabled();

/** Holds Alt (Option on a Mac) for the body: snapping is off, so the item follows the pointer exactly. */
async function withAlt(page, body) {
  await page.keyboard.down("Alt");
  try {
    return await body();
  } finally {
    await page.keyboard.up("Alt");
  }
}

test("setup: register, create a Memory Book, put a sticker on the cover", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const page = await (await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } })).newPage();
  await step("register and verify the email", async () => {
    await page.goto("/register");
    await page.getByLabel("Your name").fill(OWNER.name);
    await page.getByLabel("Email").fill(OWNER.email);
    await page.getByLabel("Password", { exact: true }).fill(OWNER.password);
    await page.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
    const registered = page.waitForResponse((r) => r.url().includes("/api/auth/register") && r.request().method() === "POST");
    await page.getByRole("button", { name: "Create account" }).click();
    if ((await registered).status() === 429) throw new Error("registration is rate limited: run `make reset-limits SMEM_ENV=test` (make e2e does)");
    await expect(page.getByRole("heading", { name: "Verify your email" })).toBeVisible();
    await page.getByLabel("6-digit code").fill(OTP); // the sixth digit submits
    await expect(page.getByText("Your email address is verified.")).toBeVisible();
  });
  await step("create the book and give the cover one sticker", async () => {
    await page.goto("/yearbooks/new");
    await page.getByLabel("Title").fill(TITLE);
    await page.getByLabel("Yearbook language").selectOption("vi");
    await page.getByLabel("Page size").selectOption("A5");
    await page.getByRole("button", { name: "Create yearbook" }).click();
    await expect(page.getByRole("heading", { level: 1, name: TITLE })).toBeVisible();
    bookId = new URL(page.url()).pathname.split("/").pop();
    const res = await post(page, "/api/yearbook/update", { id: bookId, template_id: "memory-book" });
    expect(res.ok(), `template change ${res.status()}`).toBe(true);
    await saveLayout(page, [STAR]);
  });
});

test("mouse: drag, corner resize and rotate follow the pointer to within 1 px on A5, A4 and Letter at two zooms; handles have 44 px hit areas", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1600, height: 1100 });
  for (const paper of PAPERS) {
    await openEditor(page, paper);
    for (const zoom of ZOOMS) {
      await step(`${paper} at ${zoom} %`, async () => {
        await setZoom(page, zoom);
        const item = drawn(page, STAR.id);
        const hit = page.getByRole("button", { name: "Sticker", exact: true });
        await hit.evaluate((el) => el.scrollIntoView({ block: "center", inline: "center" }));
        await hit.click();
        await expect(hit).toHaveAttribute("aria-pressed", "true");
        const row = { paper, zoom };

        // --- drag: 70 right, 45 down, snapping off
        const b0 = await box(item);
        await withAlt(page, async () => {
          await page.mouse.move(b0.cx, b0.cy);
          await page.mouse.down();
          await page.mouse.move(b0.cx + 35, b0.cy + 20, { steps: 3 });
          await page.mouse.move(b0.cx + 70, b0.cy + 45, { steps: 3 });
          await page.mouse.up();
        });
        await expect.poll(async () => Math.round(((await box(item)).l - b0.l) * 10)).toBeGreaterThan(600);
        const b1 = await box(item);
        row.drag = Math.hypot(b1.l - b0.l - 70, b1.t - b0.t - 45);
        expect(row.drag, `${paper} ${zoom} %: drag error in screen px`).toBeLessThanOrEqual(MAX_ERROR_PX);
        // a gesture is one undo step: one Ctrl+Z puts the item back
        await page.keyboard.press("ControlOrMeta+z");
        await expect.poll(async () => Math.abs((await box(item)).l - b0.l)).toBeLessThan(0.01);
        const b2 = await box(item);
        expect(Math.hypot(b2.l - b0.l, b2.t - b0.t), "one undo step undoes the whole drag").toBeLessThanOrEqual(0.01);
        await expect(page.getByRole("button", { name: "Undo" })).toBeDisabled();

        // --- the 44 x 44 px hit area of a handle, at this zoom
        const se = page.getByRole("button", { name: "Resize from bottom right" });
        const hb = await box(se);
        const hits = await page.evaluate(
          ([cx, cy]) => [[-21, -21], [21, -21], [-21, 21], [21, 21], [0, 0]].map(([dx, dy]) => document.elementFromPoint(cx + dx, cy + dy)?.getAttribute("aria-label")),
          [hb.cx, hb.cy],
        );
        expect(hits, `${paper} ${zoom} %: the corners of a 44 px square around the handle all hit it`).toEqual(Array(5).fill("Resize from bottom right"));

        // --- corner resize: 40 right, 30 down; the top left corner stays
        await withAlt(page, async () => {
          await page.mouse.move(hb.cx, hb.cy);
          await page.mouse.down();
          await page.mouse.move(hb.cx + 20, hb.cy + 15, { steps: 3 });
          await page.mouse.move(hb.cx + 40, hb.cy + 30, { steps: 3 });
          await page.mouse.up();
        });
        await expect.poll(async () => Math.round(((await box(item)).w - b0.w) * 10)).toBeGreaterThan(300);
        const r1 = await box(item);
        row.resize = Math.hypot(r1.w - b0.w - 40, r1.h - b0.h - 30);
        row.corner = Math.hypot(r1.l - b0.l, r1.t - b0.t);
        expect(row.resize, `${paper} ${zoom} %: resize error in screen px`).toBeLessThanOrEqual(MAX_ERROR_PX);
        expect(row.corner, `${paper} ${zoom} %: the opposite corner did not move`).toBeLessThanOrEqual(MAX_ERROR_PX);
        await page.keyboard.press("ControlOrMeta+z");
        await expect.poll(async () => Math.abs((await box(item)).w - b0.w)).toBeLessThan(0.01);

        // --- rotate: the pointer turns 30 degrees about the centre
        const rot = page.getByRole("button", { name: "Rotate", exact: true });
        const rb = await box(rot);
        const c = [b0.cx, b0.cy];
        const rho = Math.hypot(rb.cx - c[0], rb.cy - c[1]);
        const turn = (deg) => {
          const a = (deg * Math.PI) / 180;
          const vx = rb.cx - c[0];
          const vy = rb.cy - c[1];
          return [c[0] + vx * Math.cos(a) - vy * Math.sin(a), c[1] + vx * Math.sin(a) + vy * Math.cos(a)];
        };
        await page.mouse.move(rb.cx, rb.cy);
        await page.mouse.down();
        await page.mouse.move(...turn(15), { steps: 3 });
        await page.mouse.move(...turn(30), { steps: 3 });
        await page.mouse.up();
        await expect.poll(() => angleOf(item)).toBeGreaterThan(20);
        const got = await angleOf(item);
        row.rotate = (rho * Math.abs(got - 30) * Math.PI) / 180; // the handle's distance from where the pointer is
        expect(row.rotate, `${paper} ${zoom} %: rotate error at the handle, screen px`).toBeLessThanOrEqual(MAX_ERROR_PX);
        await page.keyboard.press("ControlOrMeta+z");
        await expect.poll(() => angleOf(item)).toBeCloseTo(0, 5);
        await expect(page.getByRole("button", { name: "Undo" })).toBeDisabled();
        numbers.push(row);
      });
    }
  }
  await page.context().close();
});

test("mouse: Shift keeps the ratio and snaps rotation to 15; Alt off, the item snaps to the page centre; Escape cancels and sends nothing", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1600, height: 1100 });
  await openEditor(page, "A5");
  await setZoom(page, 100);
  const item = drawn(page, STAR.id);
  const hit = page.getByRole("button", { name: "Sticker", exact: true });
  await hit.click();
  const sheet = await box(page.locator(".ed-pagebox .bk-sheet"));
  const saves = [];
  page.on("request", (r) => r.url().includes("/api/book-layout/save") && saves.push(r.url()));

  await step("Shift on a corner keeps the aspect ratio", async () => {
    const hb = await box(page.getByRole("button", { name: "Resize from bottom right" }));
    const b0 = await box(item);
    await withAlt(page, async () => {
      await page.keyboard.down("Shift");
      await page.mouse.move(hb.cx, hb.cy);
      await page.mouse.down();
      await page.mouse.move(hb.cx + 60, hb.cy + 5, { steps: 4 });
      await page.mouse.up();
      await page.keyboard.up("Shift");
    });
    await expect.poll(async () => (await box(item)).w).toBeGreaterThan(b0.w + 20);
    const b1 = await box(item);
    expect(Math.abs(b1.w / b1.h - b0.w / b0.h)).toBeLessThan(0.03);
    await page.keyboard.press("ControlOrMeta+z");
  });

  await step("Shift while rotating snaps to 15 degrees", async () => {
    const b0 = await box(item);
    const rb = await box(page.getByRole("button", { name: "Rotate", exact: true }));
    const a = (22 * Math.PI) / 180;
    const vx = rb.cx - b0.cx;
    const vy = rb.cy - b0.cy;
    await page.keyboard.down("Shift");
    await page.mouse.move(rb.cx, rb.cy);
    await page.mouse.down();
    await page.mouse.move(b0.cx + vx * Math.cos(a) - vy * Math.sin(a), b0.cy + vx * Math.sin(a) + vy * Math.cos(a), { steps: 4 });
    await page.mouse.up();
    await page.keyboard.up("Shift");
    await expect.poll(() => angleOf(item)).toBeCloseTo(15, 3);
    await page.keyboard.press("ControlOrMeta+z");
  });

  await step("without Alt the item snaps onto the page centre line; with Alt it does not", async () => {
    const k = await scale(page);
    const b0 = await box(item);
    const cx = sheet.l + sheet.w / 2; // the page's centre line on screen
    const grab = b0.cx; // drag so that the item's centre ends 3 screen px beside it
    const dx = cx + 3 - grab;
    await page.mouse.move(b0.cx, b0.cy);
    await page.mouse.down();
    await page.mouse.move(b0.cx + dx / 2, b0.cy, { steps: 3 });
    await page.mouse.move(b0.cx + dx, b0.cy, { steps: 3 });
    await expect(page.locator(".ed-guide-v")).toHaveCount(1); // the guide is drawn while snapped
    await page.mouse.up();
    await expect.poll(async () => Math.abs((await box(item)).cx - cx)).toBeLessThan(0.7);
    void k;
    await page.keyboard.press("ControlOrMeta+z");
    await expect.poll(async () => Math.abs((await box(item)).cx - b0.cx)).toBeLessThan(0.01);
    await withAlt(page, async () => {
      await page.mouse.move(b0.cx, b0.cy);
      await page.mouse.down();
      await page.mouse.move(b0.cx + dx, b0.cy, { steps: 5 });
      await expect(page.locator(".ed-guide-v")).toHaveCount(0);
      await page.mouse.up();
    });
    await expect.poll(async () => Math.abs((await box(item)).cx - (cx + 3))).toBeLessThan(1);
    await page.keyboard.press("ControlOrMeta+z");
  });

  await step("Escape during a drag restores the start box, records no step and sends no save", async () => {
    await expect(page.getByRole("button", { name: "Undo" })).toBeDisabled();
    await page.waitForTimeout(2000); // the autosave of the earlier undo steps has gone out
    saves.length = 0;
    const b0 = await box(item);
    await page.mouse.move(b0.cx, b0.cy);
    await page.mouse.down();
    await page.mouse.move(b0.cx + 80, b0.cy + 60, { steps: 4 });
    await expect.poll(async () => (await box(item)).l).toBeGreaterThan(b0.l + 70);
    await page.keyboard.press("Escape");
    await expect.poll(async () => Math.abs((await box(item)).l - b0.l)).toBeLessThan(0.01);
    await page.mouse.up();
    await expect(hit).toHaveAttribute("aria-pressed", "true"); // the selection stays
    await expect(page.getByRole("button", { name: "Undo" })).toBeDisabled();
    await page.waitForTimeout(2000); // longer than the autosave's 1.5 s debounce
    expect(saves, "a cancelled gesture sends nothing").toEqual([]);
  });
  await page.context().close();
});

test("keyboard: select, move, resize, rotate, delete and undo without a pointer", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1600, height: 1100 });
  await openEditor(page, "A4");
  await setZoom(page, 100);
  const item = drawn(page, STAR.id);
  const hit = page.getByRole("button", { name: "Sticker", exact: true });
  const k = await scale(page);
  const live = () => page.locator(".visually-hidden[role=status]").textContent();

  await step("Tab reaches the sticker and selects it", async () => {
    await page.getByRole("button", { name: "Cover title" }).focus();
    for (let i = 0; i < 3; i++) await page.keyboard.press("Tab"); // photo, cover sticker, then the added sticker
    await expect(hit).toBeFocused();
    await expect(hit).toHaveAttribute("aria-pressed", "true");
  });
  await step("arrows move 1 design px, Shift+arrow 10; the burst is one undo step; the live region speaks", async () => {
    const b0 = await box(item);
    for (let i = 0; i < 3; i++) await page.keyboard.press("ArrowRight");
    await page.keyboard.press("Shift+ArrowDown");
    await expect.poll(async () => (await box(item)).t - b0.t).toBeGreaterThan(9 * k);
    const b1 = await box(item);
    expect(Math.abs(b1.l - b0.l - 3 * k)).toBeLessThanOrEqual(0.7);
    expect(Math.abs(b1.t - b0.t - 10 * k)).toBeLessThanOrEqual(0.7);
    expect(await live()).toMatch(/Moved down 10\. Position .* percent from the left/);
    await page.keyboard.press("ControlOrMeta+z");
    await expect.poll(async () => Math.abs((await box(item)).l - b0.l)).toBeLessThan(0.01);
    expect(Math.abs((await box(item)).t - b0.t)).toBeLessThan(0.01);
    await expect(page.getByRole("button", { name: "Undo" })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Redo" })).toBeEnabled();
    await page.keyboard.press("ControlOrMeta+Shift+z");
    await expect.poll(async () => (await box(item)).t - b0.t).toBeGreaterThan(9 * k);
    await page.keyboard.press("ControlOrMeta+z");
  });
  await step("Alt+Shift+Arrow resizes by 10 design px; Alt+Arrow alone does not navigate away", async () => {
    const b0 = await box(item);
    await page.keyboard.press("Alt+ArrowLeft"); // browser Back on Windows and Linux: the editor must not use it
    await page.keyboard.press("Alt+Shift+ArrowRight");
    await page.keyboard.press("Alt+Shift+ArrowDown");
    await expect.poll(async () => (await box(item)).w - b0.w).toBeGreaterThan(9 * k);
    const b1 = await box(item);
    expect(Math.abs(b1.h - b0.h - 10 * k)).toBeLessThanOrEqual(0.7);
    expect(Math.abs(b1.l - b0.l)).toBeLessThanOrEqual(0.7);
    await page.keyboard.press("ControlOrMeta+z");
    await expect.poll(async () => Math.abs((await box(item)).w - b0.w)).toBeLessThan(0.01);
  });
  await step("comma and period rotate by 1 degree, Shift by 15", async () => {
    await page.keyboard.press(".");
    await page.keyboard.press(".");
    await expect.poll(() => angleOf(item)).toBeCloseTo(2, 3);
    await page.keyboard.press("Shift+.");
    await expect.poll(() => angleOf(item)).toBeCloseTo(17, 3);
    await page.keyboard.press(",");
    await expect.poll(() => angleOf(item)).toBeCloseTo(16, 3);
    await page.keyboard.press("ControlOrMeta+z");
    await expect.poll(() => angleOf(item)).toBeCloseTo(0, 3);
  });
  await step("? lists the shortcuts; Escape closes the list and a second Escape clears the selection", async () => {
    await page.keyboard.press("Shift+?");
    await expect(page.getByRole("region", { name: "Keyboard shortcuts" }).getByRole("row")).toHaveCount(14);
    await page.keyboard.press("Escape");
    await expect(page.getByRole("region", { name: "Keyboard shortcuts" })).toHaveCount(0);
    await hit.focus();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("heading", { name: "Page: Cover" })).toBeVisible();
  });
  await step("Delete removes the item and undo brings it back", async () => {
    await hit.press("Enter"); // Enter on the focused button selects it again (a click from the keyboard)
    await expect(hit).toHaveAttribute("aria-pressed", "true");
    await page.keyboard.press("Delete");
    await expect(item).toHaveCount(0);
    expect(await live()).toBe("Deleted");
    await page.keyboard.press("ControlOrMeta+z");
    await expect(item).toHaveCount(1);
  });
  await page.context().close();
});

test("an item flush to the page edge can be resized with the mouse from a handle outside the page", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1600, height: 1100 });
  await saveLayout(page, [FLUSH]);
  for (const paper of PAPERS) {
    await step(`${paper}`, async () => {
      await openEditor(page, paper);
      await setZoom(page, 100);
      const item = drawn(page, FLUSH.id);
      await page.getByRole("button", { name: "Sticker", exact: true }).click();
      const b0 = await box(item);
      const sheet = await box(page.locator(".ed-pagebox .bk-sheet"));
      const hb = await box(page.getByRole("button", { name: "Resize from bottom right" }));
      expect(hb.cx + 5, "the handle sticks out of the page").toBeGreaterThan(sheet.l + sheet.w);
      const x = hb.cx + 4; // press on the part of the handle that lies outside the clip
      const y = hb.cy + 4;
      await withAlt(page, async () => {
        await page.mouse.move(x, y);
        await page.mouse.down();
        await page.mouse.move(x - 15, y - 15, { steps: 3 });
        await page.mouse.move(x - 30, y - 30, { steps: 3 });
        await page.mouse.up();
      });
      await expect.poll(async () => b0.w - (await box(item)).w).toBeGreaterThan(28);
      const b1 = await box(item);
      expect(Math.hypot(b0.w - b1.w - 30, b0.h - b1.h - 30), `${paper}: shrink by the pointer distance`).toBeLessThanOrEqual(MAX_ERROR_PX);
      await page.keyboard.press("ControlOrMeta+z");
    });
  }
  await page.context().close();
});

test("a template element moved over another one is the one a click on the overlap selects (T-160 A1)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1600, height: 1100 });
  await saveLayout(page, []);
  await openEditor(page, "A5");
  await setZoom(page, 100);
  const hit = (name) => page.getByRole("button", { name, exact: true });
  const photo = hit("Cover photo");
  const title = hit("Cover title");
  await step("move the photo down, then drag the title over the photo", async () => {
    await photo.focus(); // focus selects
    const p0 = await box(photo);
    await withAlt(page, async () => {
      await page.mouse.move(p0.cx, p0.cy);
      await page.mouse.down();
      await page.mouse.move(p0.cx, p0.cy + 120, { steps: 4 });
      await page.mouse.up();
    });
    await expect.poll(async () => (await box(photo)).t - p0.t).toBeGreaterThan(100);
    const p1 = await box(photo);
    await title.focus();
    const t0 = await box(title);
    await withAlt(page, async () => {
      await page.mouse.move(t0.l + 8, t0.t + 8); // a point of the title that the photo does not cover
      await page.mouse.down();
      await page.mouse.move(t0.l + 8 + (p1.cx - t0.cx) / 2, t0.t + 8 + (p1.cy - t0.cy) / 2, { steps: 4 });
      await page.mouse.move(t0.l + 8 + (p1.cx - t0.cx), t0.t + 8 + (p1.cy - t0.cy), { steps: 4 });
      await page.mouse.up();
    });
    await expect.poll(async () => Math.abs((await box(title)).cy - p1.cy)).toBeLessThan(2);
  });
  await step("a click on the overlap selects the title (drawn above the photo), and Tab visits the photo before the title", async () => {
    const p = await box(photo);
    const t = await box(title);
    expect(Math.abs(p.cy - t.cy), "the boxes overlap").toBeLessThan(2);
    const ids = await page.evaluate(([x, y]) => document.elementFromPoint(x, y)?.getAttribute("aria-label"), [p.cx, p.cy]);
    expect(ids, "what a click on the overlap reaches").toBe("Cover title");
    await page.mouse.click(p.cx, p.cy);
    await expect(title).toHaveAttribute("aria-pressed", "true");
    const order = await page.locator(".ed-hit").evaluateAll((els) => els.map((e) => e.getAttribute("aria-label")));
    expect(order.indexOf("Cover photo")).toBeLessThan(order.indexOf("Cover title"));
  });
  await page.context().close();
});

test("touch at 768 px: the first tap selects, a drag of the selected item moves it, a swipe over an unselected item pans the stage", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 768, height: 1024 }, { hasTouch: true });
  await saveLayout(page, [STAR]);
  await openEditor(page, "A5");
  const cdp = await page.context().newCDPSession(page);
  const touch = async (type, x, y) => cdp.send("Input.dispatchTouchEvent", { type, touchPoints: type === "touchEnd" ? [] : [{ x, y, id: 1 }] });
  const item = drawn(page, STAR.id);
  const hit = page.getByRole("button", { name: "Sticker", exact: true });

  await step("a swipe over the unselected item pans the stage and does not move the item", async () => {
    await setZoom(page, 200);
    await hit.evaluate((el) => el.scrollIntoView({ block: "center", inline: "center" }));
    const scroller = page.locator(".ed-scroll");
    const before = await scroller.evaluate((el) => ({ x: el.scrollLeft, y: el.scrollTop }));
    const b0 = await box(item);
    const sheet0 = await box(page.locator(".ed-pagebox .bk-sheet"));
    await touch("touchStart", b0.cx, b0.cy);
    for (let i = 1; i <= 10; i++) await touch("touchMove", b0.cx - i * 8, b0.cy);
    await touch("touchEnd");
    await expect.poll(async () => (await scroller.evaluate((el) => el.scrollLeft)) - before.x).toBeGreaterThan(20);
    const sheet1 = await box(page.locator(".ed-pagebox .bk-sheet"));
    const b1 = await box(item);
    expect(Math.abs(b1.l - b0.l - (sheet1.l - sheet0.l)), "the item stayed where it is on the page").toBeLessThanOrEqual(0.5);
    await expect(page.getByRole("button", { name: "Undo" })).toBeDisabled();
  });
  await step("the first tap selects", async () => {
    await setZoom(page, 100);
    await hit.evaluate((el) => el.scrollIntoView({ block: "center", inline: "center" }));
    await expect(hit).toHaveAttribute("aria-pressed", "false");
    const [x, y] = await centreOf(item);
    await touch("touchStart", x, y);
    await touch("touchEnd");
    await expect(hit).toHaveAttribute("aria-pressed", "true");
    await expect(page.getByRole("button", { name: "Undo" })).toBeDisabled();
  });
  await step("then a drag of the selected item moves it by the finger distance", async () => {
    const b0 = await box(item);
    await touch("touchStart", b0.cx, b0.cy);
    for (let i = 1; i <= 8; i++) await touch("touchMove", b0.cx - i * 3, b0.cy + i * 4); // there is no Alt on a touch screen, so snapping is on
    await touch("touchEnd");
    await expect.poll(async () => (await box(item)).t - b0.t).toBeGreaterThan(25);
    const b1 = await box(item);
    expect(Math.hypot(b1.l - b0.l + 24, b1.t - b0.t - 32), `touch drag moved the item by ${(b1.l - b0.l).toFixed(2)}, ${(b1.t - b0.t).toFixed(2)} (finger -24, 32; snapping is on, it may pull the box up to 6 px onto a guide)`).toBeLessThanOrEqual(6);
    await expect(page.getByRole("button", { name: "Undo" })).toBeEnabled();
  });
  await page.context().close();
});

test("performance: dragging one item among 100 holds at least 30 fps under a 4x CPU throttle", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1600, height: 1100 });
  // 100 stickers and texts on a 10 x 10 grid. The server allows 40 an page, so the document is served by the test, not stored: only this page sees it.
  const items = Array.from({ length: 100 }, (_, i) => {
    const col = i % 10;
    const row = Math.floor(i / 10);
    const base = { id: `perf${String(i).padStart(4, "0")}ab`, x: 40 + col * 94, w: 80, h: 80, cy: 60 + row * 90, r: 0 };
    return i % 2 ? { ...base, t: "text", text: `Lớp 10C3 số ${i}`, size: 20 } : { ...base, t: "asset", asset: "sticker.gold-star" };
  });
  await page.route("**/api/book-layout/get*", (route) => route.fulfill({ json: { status: "OK", data: { version: 5, layout: { v: 1, pages: { cover: { items } } } } } }));
  await page.route("**/api/book-layout/save", (route) => route.fulfill({ json: { status: "OK", data: { version: 6, updated_at: Date.now() } } }));
  await openEditor(page, "A5");
  await setZoom(page, 100);
  await expect(page.locator(".ed-hit")).toHaveCount(100 + 3); // the 100 items and the cover's three elements
  const target = drawn(page, items[44].id);
  await page.locator(`[data-target='${items[44].id}'] .ed-hit`).evaluate((el) => el.scrollIntoView({ block: "center", inline: "center" }));
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: 4 });
  await page.evaluate(() => {
    window.__frames = { stamps: [], on: true };
    const tick = (t) => {
      if (!window.__frames.on) return;
      window.__frames.stamps.push(t);
      requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });
  const b0 = await box(target);
  await page.mouse.move(b0.cx, b0.cy);
  await page.mouse.down();
  const t0 = Date.now();
  await page.evaluate(() => (window.__frames.stamps.length = 0));
  for (let i = 0; Date.now() - t0 < 3000; i++) {
    await page.mouse.move(b0.cx + 40 * Math.sin(i / 6), b0.cy + 40 * Math.cos(i / 9));
    await page.waitForTimeout(8);
  }
  const stamps = await page.evaluate(() => {
    window.__frames.on = false;
    return window.__frames.stamps;
  });
  await page.mouse.up();
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: 1 });
  await expect(page.getByRole("button", { name: "Undo" })).toBeEnabled(); // the drag really changed the layout
  const deltas = stamps.slice(1).map((t, i) => t - stamps[i]).sort((a, b) => a - b);
  const fps = (stamps.length - 1) / ((stamps[stamps.length - 1] - stamps[0]) / 1000);
  const p95 = deltas[Math.floor(deltas.length * 0.95)];
  numbers.push({ fps100: Number(fps.toFixed(1)), p95FrameMs: Number(p95.toFixed(1)), frames: stamps.length });
  expect(fps, `frames per second while dragging among 100 items at 4x throttle (p95 frame ${p95.toFixed(1)} ms)`).toBeGreaterThanOrEqual(30);
});

test.afterAll(() => {
  // the numbers the PR quotes (pointer error in screen px, worst of the three measures per row)
  for (const r of numbers) console.log("T160-NUMBERS " + JSON.stringify(r, (_, v) => (typeof v === "number" ? Number(v.toFixed(2)) : v)));
});
