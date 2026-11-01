// The palette, own images and page backgrounds of the layout editor end to end (T-162, docs/editor-palette.md): add a sticker, a shape and a frame,
// tint, upload a real PNG through the UI and use it as an image and as a background, replace it, delete it while in use and see the pages cleaned,
// reload and see the same layout, every upload error (a GIF, a text file named .png, the 30-image cap), and the keyboard-only path.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
// Not covered here: a phone photo of 4000 px and real device uploads (QA does them by hand), two tabs (docs/editor-palette.md lists them).
import { deflateSync, crc32 } from "node:zlib";
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 90_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Lê Thị Hoài Thương", email: `e2e-palette-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9` };
const TITLE = `Lớp 10C – Trang trí ${RUN}`;

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

/** A real PNG (RGB, a gradient so the file is not trivially small), made without a library. */
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
const file = (name, seed = 1, w = 400, h = 300) => ({ name, mimeType: "image/png", buffer: png(w, h, seed) });

async function ownerPage(browser, viewport = { width: 1500, height: 1000 }) {
  const ctx = await browser.newContext({ locale: "en-US", baseURL: BASE, viewport });
  const page = await ctx.newPage();
  const login = await page.request.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password }, headers: { Origin: BASE } });
  expect(login.ok(), `login ${login.status()}`).toBe(true);
  return page;
}
const post = (page, path, data) => page.request.post(path, { data, headers: { Origin: BASE } });
const storedLayout = async (page) => (await (await page.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json()).data;
const storedCover = async (page) => (await storedLayout(page)).layout?.pages?.cover ?? {};
const designImages = async (page) => (await (await page.request.get(`/api/media/get-list?yearbook_id=${bookId}&purpose=design&limit=100`)).json()).data.items;
async function resetLayout(page) {
  expect((await post(page, "/api/book-layout/reset", { yearbook_id: bookId })).ok()).toBe(true);
}
async function openEditor(page) {
  await page.goto(`/yearbooks/${bookId}/editor`);
  await expect(page.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
  await expect(page.locator(".ed-pagebox .bk-sheet[data-page='cover']")).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
}
const palette = (page) => page.getByRole("complementary", { name: "Add to the page" });
const tab = (page, name) => palette(page).getByRole("tab", { name });
const items = (page) => page.locator(".ed-pagebox [data-layout-item]");
/** Uploads files through the file input of the visible palette tab. */
async function upload(page, files) {
  await palette(page).getByTestId("design-file").setInputFiles(files);
}
async function uploadViaApi(page, name, seed) {
  const f = file(name, seed, 64, 64);
  const res = await page.request.post("/api/media/create", { headers: { Origin: BASE }, multipart: { yearbook_id: bookId, purpose: "design", file: { name: f.name, mimeType: f.mimeType, buffer: f.buffer } } });
  expect(res.ok(), `upload ${name}: ${res.status()} ${await res.text()}`).toBe(true);
}

test("setup: register and create a Memory Book", async ({ browser }) => {
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
    await page.getByLabel("6-digit code").fill(OTP);
    await expect(page.getByText("Your email address is verified.")).toBeVisible();
  });
  await step("create the book", async () => {
    await page.goto("/yearbooks/new");
    await page.getByLabel("Title").fill(TITLE);
    await page.getByLabel("Yearbook language").selectOption("vi");
    await page.getByLabel("Page size").selectOption("A5");
    await page.getByRole("button", { name: "Create yearbook" }).click();
    await expect(page.getByRole("heading", { level: 1, name: TITLE })).toBeVisible();
    bookId = new URL(page.url()).pathname.split("/").pop();
    expect((await post(page, "/api/yearbook/update", { id: bookId, template_id: "memory-book" })).ok()).toBe(true);
  });
});

test("a sticker, a shape and a frame: added centred at 25 %, tinted, saved, the same after a reload", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  await resetLayout(page);
  await openEditor(page);
  await step("a sticker by search, a shape, a frame", async () => {
    await palette(page).getByRole("searchbox").fill("ngoan ngoeo"); // Vietnamese "ngoằn ngoèo" without diacritics
    await palette(page).getByRole("button", { name: "Squiggle line" }).click();
    await expect(items(page)).toHaveCount(1);
    await tab(page, "Shapes").click();
    await palette(page).getByRole("button", { name: "Heart" }).click();
    await tab(page, "Frames").click();
    await palette(page).getByRole("button", { name: "Polaroid" }).click();
    await expect(items(page)).toHaveCount(3);
    await expect(page.locator(".ed-pagebox [data-layout-item] svg")).toHaveCount(3, { timeout: 15_000 }); // the SVG chunks loaded and drew
  });
  await step("tint the heart; the frame is the last one, on top", async () => {
    await page.getByRole("button", { name: "Sticker", exact: true }).nth(1).focus(); // the three sit on top of each other: Tab/focus picks one
    await page.getByRole("radio", { name: "Accent" }).click();
    await page.getByRole("button", { name: "Original colours" }).waitFor();
    await expect.poll(async () => (await storedCover(page)).items?.map((i) => i.asset + ":" + (i.tint ?? "")), { timeout: 15_000 }).toEqual(["sticker.doodle-squiggle:", "shape.heart:accent", "frame.polaroid:"]);
    const stored = (await storedCover(page)).items;
    for (const i of stored) expect(i).toMatchObject({ t: "asset", w: 250, x: 375, cy: 500 });
  });
  await step("reload: the same page", async () => {
    await page.reload();
    await expect(items(page)).toHaveCount(3);
    expect((await storedCover(page)).items.map((i) => i.asset)).toEqual(["sticker.doodle-squiggle", "shape.heart", "frame.polaroid"]);
    await expect(page.locator(".ed-pagebox [data-layout-item='" + (await storedCover(page)).items[1].id + "'] svg")).toHaveAttribute("data-asset", "shape.heart");
  });
});

test("keyboard only: find by search, add with Enter, tint, move with the arrows, delete", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  await resetLayout(page);
  await openEditor(page);
  await tab(page, "Stickers").focus();
  await page.keyboard.press("Tab"); // the search field
  await page.keyboard.type("ngoan ngoeo"); // "Nét ngoằn ngoèo" is found without the accents
  await page.keyboard.press("Tab"); // the one tab stop of the grid
  await expect(palette(page).getByRole("button", { name: "Squiggle line" })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(items(page)).toHaveCount(1);
  await expect(page.locator(".ed-box.is-selected .ed-hit")).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("ArrowRight");
  await expect.poll(async () => (await storedCover(page)).items?.[0]?.x, { timeout: 15_000 }).toBeGreaterThan(375);
  await page.getByRole("radio", { name: "Accent" }).focus();
  await page.keyboard.press("Space");
  await expect.poll(async () => (await storedCover(page)).items?.[0]?.tint, { timeout: 15_000 }).toBe("accent");
  await page.locator(".ed-box.is-selected .ed-hit").focus();
  await page.keyboard.press("Delete");
  await expect(items(page)).toHaveCount(0);
  await expect.poll(async () => (await storedCover(page)).items?.length ?? 0, { timeout: 15_000 }).toBe(0);
});

test("own images: upload through the UI, use as an image and as a background, replace, delete while in use", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  await resetLayout(page);
  await openEditor(page);
  await tab(page, "My images").click();
  await step("upload a real PNG: it appears in the list and on the server as a design image", async () => {
    await expect(palette(page).getByText(/No images yet/)).toBeVisible();
    await upload(page, file("holiday.png", 1, 400, 300));
    await expect(palette(page).getByRole("button", { name: "Add image 1 of 1" })).toBeVisible();
    const list = await designImages(page);
    expect(list).toHaveLength(1);
    expect(list[0]).toMatchObject({ purpose: "design", width: 400, height: 300 });
    await expect(palette(page).locator(".ed-tile img").first()).toHaveAttribute("src", /variant=thumb/);
    expect(await palette(page).locator(".ed-tile img").first().evaluate((i) => i.naturalWidth)).toBeGreaterThan(0);
  });
  await step("add it as an image: long side 50 % of the page, aspect kept, contain, selected", async () => {
    await palette(page).getByRole("button", { name: "Add image 1 of 1" }).click();
    await expect(items(page)).toHaveCount(1);
    await expect.poll(async () => (await storedCover(page)).items?.[0], { timeout: 15_000 }).toMatchObject({ t: "image", fit: "contain", w: 500, h: 375, x: 250 });
    await expect(page.getByRole("heading", { name: "Picture" })).toBeVisible();
    expect(await page.locator(".ed-pagebox [data-layout-item] img").evaluate((i) => i.complete && i.naturalWidth)).toBeGreaterThan(0);
  });
  await step("corner radius and fit are saved", async () => {
    await page.getByRole("combobox", { name: /Fit/ }).selectOption("cover");
    await page.getByRole("slider", { name: "Corner radius" }).press("End"); // a key press ends with keyup, which commits the step (as a pointer drag ends with pointerup)
    await expect.poll(async () => (await storedCover(page)).items?.[0], { timeout: 15_000 }).toMatchObject({ fit: "cover", radius: 500 });
  });
  await step("use it as the page background (full-bleed, display variant)", async () => {
    await tab(page, "Background").click();
    await expect(palette(page).getByText(/background of the cover/)).toBeVisible();
    await palette(page).getByRole("button", { name: "Use image 1 of 1 as background" }).click();
    const id = (await designImages(page))[0].id;
    await expect.poll(async () => (await storedCover(page)).bg, { timeout: 15_000 }).toEqual({ media: id });
    await expect(page.locator(`.ed-pagebox .bk-sheet[data-page='cover'] img[src*='${id}'][src*='variant=display']`)).toHaveCount(1);
  });
  await step("replace the image keeping the box: upload a second one from the picker and choose it", async () => {
    await page.getByRole("button", { name: "Picture" }).click();
    await page.getByRole("button", { name: "Replace image" }).click();
    await page.locator(".ed-panel").first().getByTestId("design-file").setInputFiles(file("second.png", 2, 300, 500));
    await expect(page.getByRole("button", { name: "Use image 1 of 2" }).first()).toBeVisible();
    const before = (await storedCover(page)).items[0];
    const second = (await designImages(page))[0]; // newest first
    await page.locator(".ed-panel").first().getByRole("button", { name: "Use image 1 of 2" }).click();
    await expect.poll(async () => (await storedCover(page)).items[0].media, { timeout: 15_000 }).toBe(second.id);
    expect((await storedCover(page)).items[0]).toMatchObject({ x: before.x, w: before.w, h: before.h, cy: before.cy });
  });
  await step("reload: the same layout", async () => {
    const saved = await storedLayout(page);
    await page.reload();
    await expect(items(page)).toHaveCount(1);
    expect((await storedLayout(page)).layout).toEqual(saved.layout);
  });
  await step("delete the background image while it is in use: the dialog says where, the pages are cleaned, the server has no trace, undo is gone", async () => {
    const bgId = (await storedCover(page)).bg.media;
    await tab(page, "My images").click();
    const n = (await designImages(page)).findIndex((m) => m.id === bgId) + 1;
    await palette(page).getByRole("button", { name: `Delete image ${n} of 2` }).click();
    const dlg = page.getByRole("dialog");
    await expect(dlg).toContainText("used on 1 page types (the cover)");
    await expect(dlg).toContainText("Undo and redo are cleared");
    await dlg.getByRole("button", { name: "Delete image" }).click();
    await expect(dlg).toHaveCount(0);
    const after = await storedLayout(page);
    expect(JSON.stringify(after.layout)).not.toContain(bgId);
    expect(after.layout.pages.cover.bg).toBeUndefined();
    expect((await designImages(page)).map((m) => m.id)).not.toContain(bgId);
    await expect(page.getByRole("button", { name: "Undo" })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Redo" })).toBeDisabled();
    await expect(palette(page).locator(".ed-tile img")).toHaveCount(1);
  });
  await step("delete the image that an item uses: the item goes too", async () => {
    const used = (await storedCover(page)).items[0].media;
    await palette(page).getByRole("button", { name: "Delete image 1 of 1" }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Delete image" }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0); // closes when the image is deleted
    await expect(items(page)).toHaveCount(0);
    expect(JSON.stringify((await storedLayout(page)).layout)).not.toContain(used);
    expect(await designImages(page)).toHaveLength(0);
  });
});

test("every upload error has its own message; the 31st image is refused with the cap", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  await openEditor(page);
  await tab(page, "My images").click();
  await step("a GIF is refused before it is sent", async () => {
    let sent = 0;
    page.on("request", (r) => r.url().includes("/api/media/create") && sent++);
    await upload(page, { name: "funny.gif", mimeType: "image/gif", buffer: Buffer.from("GIF89a") });
    await expect(palette(page).getByRole("alert")).toContainText("Only JPEG, PNG and WebP");
    expect(sent).toBe(0);
    await palette(page).getByRole("button", { name: "Dismiss" }).click();
  });
  await step("a text file named .png is refused by the server in plain words", async () => {
    await upload(page, { name: "notes.png", mimeType: "image/png", buffer: Buffer.from("this is not an image at all\n".repeat(20)) });
    await expect(palette(page).getByRole("alert")).toContainText(/Only JPEG, PNG and WebP|could not be read/);
    await palette(page).getByRole("button", { name: "Dismiss" }).click();
  });
  await step("a truncated PNG says it could not be read", async () => {
    await upload(page, { name: "cut.png", mimeType: "image/png", buffer: png(200, 200, 3).subarray(0, 120) });
    await expect(palette(page).getByRole("alert")).toContainText(/could not be read/);
    await palette(page).getByRole("button", { name: "Dismiss" }).click();
  });
  await step("a file over 10 MB is refused before it is sent", async () => {
    await upload(page, { name: "huge.png", mimeType: "image/png", buffer: Buffer.alloc(10 * 1024 * 1024 + 1) });
    await expect(palette(page).getByRole("alert")).toContainText("larger than 10 MB");
    await palette(page).getByRole("button", { name: "Dismiss" }).click();
  });
  await step("30 images in the book, then the 31st through the UI: the cap and the count", async () => {
    for (let i = 0; i < 30; i++) await uploadViaApi(page, `fill${i}.png`, i);
    await page.reload();
    await tab(page, "My images").click();
    await upload(page, file("one-too-many.png", 9, 64, 64));
    await expect(palette(page).getByRole("alert")).toContainText("already has 30 of 30");
    expect(await designImages(page)).toHaveLength(30);
  });
  await step("the list pages: 24 first, the rest on 'Load more'", async () => {
    await expect(palette(page).locator(".ed-thumbs .ed-tile")).toHaveCount(24);
    await palette(page).getByRole("button", { name: "Load more" }).click();
    await expect(palette(page).locator(".ed-thumbs .ed-tile")).toHaveCount(30);
    await expect(palette(page).getByText("30 of 30 images")).toBeVisible(); // the count is shown once the whole list is loaded
  });
  await step("clean up: the images are deleted with the book", async () => {
    for (const m of await designImages(page)) await post(page, "/api/media/delete", { id: m.id });
  });
});
