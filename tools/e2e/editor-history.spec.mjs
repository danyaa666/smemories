// The version history of the layout editor end to end (T-165, docs/editor-history.md): three editing bursts, open History, preview an earlier version,
// back to current, restore, reload, undo does nothing; a design image deleted after the snapshot is dropped on restore; two tabs restoring at once give one
// conflict; the keyboard-only walk.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
//
// A save stores a snapshot only when the newest one is older than 10 minutes (T-163). The test cannot wait that long, so `backdate()` moves the stored
// snapshots 11 minutes into the past with the `mysql` command line client against SMEM_DB_DSN (a throwaway database, guard.mjs): the same rows the
// server wrote, only older. The server clock is not touched. `clearSnapshots()` empties the history of the test's book between tests.
import { execFileSync } from "node:child_process";
import { deflateSync, crc32 } from "node:zlib";
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const DSN = process.env.SMEM_DB_DSN;
const STEP_MS = 90_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = {
  name: "Lê Thị Hoài Thương",
  email: `e2e-history-${RUN}@example.com`,
  password: `Mật-khẩu-${RUN}-9`,
};
const TITLE = `Lớp 10C – Lịch sử ${RUN}`;

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
  const api = await request.newContext({
    baseURL: BASE,
    extraHTTPHeaders: { Origin: BASE },
  });
  try {
    const login = await api.post("/api/auth/login", {
      data: { email: OWNER.email, password: OWNER.password },
    });
    if (!login.ok()) return;
    const list = await (await api.get("/api/yearbook/get-list")).json();
    for (const y of list.data?.items ?? [])
      await api.post("/api/yearbook/delete", { data: { id: y.id } });
  } catch (e) {
    console.warn(
      `clean-up failed (the run's data stays in the test database): ${e.message}`,
    );
  } finally {
    await api.dispose();
  }
}

test.afterAll(cleanUp);
test.describe.configure({ mode: "serial" });

let bookId = "";

/** Runs one SQL statement against the test database with the mysql client (password in MYSQL_PWD, not on the command line). */
function sql(statement) {
  const m = /^([^:]*):(.*)@tcp\(([^:)]+):(\d+)\)\/([^?]+)/.exec(DSN ?? "");
  if (!m)
    throw new Error(
      "SMEM_DB_DSN must look like user:pass@tcp(host:port)/db to back-date snapshots",
    );
  const [, user, pass, host, port, db] = m;
  execFileSync(
    "mysql",
    ["-h", host, "-P", port, "-u", user, db, "-e", statement],
    {
      env: { ...process.env, MYSQL_PWD: pass },
      stdio: ["ignore", "pipe", "inherit"],
    },
  );
}
const ofBook = () =>
  `(SELECT id FROM yearbook_tab WHERE public_id = '${bookId.replace(/[^A-Za-z0-9]/g, "")}')`;
const backdate = () =>
  sql(
    `UPDATE book_layout_version_tab SET created_at = created_at - 660000 WHERE yearbook_id = ${ofBook()}`,
  );
const clearSnapshots = () =>
  sql(`DELETE FROM book_layout_version_tab WHERE yearbook_id = ${ofBook()}`);

/** A real PNG (RGB gradient), made without a library. */
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
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    chunk("IHDR", head),
    chunk("IDAT", deflateSync(raw)),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

async function ownerPage(browser, context) {
  const ctx =
    context ??
    (await browser.newContext({
      locale: "en-US",
      baseURL: BASE,
      viewport: { width: 1500, height: 1000 },
    }));
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
/** The three history routes share the layout save limit (burst 20 per 10 s, 60 a minute): a 429 from set-up calls waits its Retry-After and tries again. */
async function post(page, path, data) {
  for (let i = 0; ; i++) {
    const res = await page.request.post(path, {
      data,
      headers: { Origin: BASE },
    });
    if (res.status() !== 429 || i >= 8) return res;
    await page.waitForTimeout(
      Math.max(1, Number(res.headers()["retry-after"] ?? 2)) * 1000,
    );
  }
}
const storedLayout = async (page) =>
  (
    await (
      await page.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)
    ).json()
  ).data;
const titleWidth = async (page) =>
  (await storedLayout(page)).layout?.pages?.cover?.overrides?.["cover.title"]
    ?.w;
async function saveLayout(page, layout) {
  const { version } = await storedLayout(page);
  const res = await post(page, "/api/book-layout/save", {
    yearbook_id: bookId,
    base_version: version,
    layout,
  });
  expect(res.ok(), `save ${res.status()} ${await res.text()}`).toBe(true);
}
const widthDoc = (permille) => ({
  v: 1,
  pages: { cover: { overrides: { "cover.title": { w: permille } } } },
});
/** The previous test may have used the burst of 20 calls; the bucket refills at one a second, so wait before a test that makes many. */
const calm = (page) => page.waitForTimeout(8000);
async function freshHistory(page) {
  expect(
    (await post(page, "/api/book-layout/reset", { yearbook_id: bookId })).ok(),
  ).toBe(true);
  clearSnapshots();
}
async function openEditor(page) {
  await page.goto(`/yearbooks/${bookId}/editor`);
  await expect(
    page.getByRole("toolbar", { name: "Editor toolbar" }),
  ).toBeVisible();
  await expect(
    page.locator(".ed-pagebox .bk-sheet[data-page='cover']"),
  ).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
}
const historyButton = (page) =>
  page.getByRole("toolbar").getByRole("button", { name: "History" });
const rows = (page) =>
  page.getByRole("radiogroup", { name: "Earlier versions" }).getByRole("radio");
async function openHistory(page) {
  await historyButton(page).click();
  await expect(rows(page).first()).toBeVisible();
}
/** The share of the page width the cover title's box takes on the stage. */
const titleShare = (page) =>
  page.evaluate(() => {
    const t = document.querySelector(
      ".ed-pagebox [data-layout-id='cover.title']",
    );
    const box = t?.classList.contains("mb-ov") ? t : t?.firstElementChild;
    const s = document.querySelector(".ed-pagebox .bk-sheet");
    return box && s
      ? box.getBoundingClientRect().width / s.getBoundingClientRect().width
      : null;
  });
const expectShare = async (page, percent) => {
  await expect
    .poll(async () => Math.round((await titleShare(page)) * 100), {
      timeout: 15_000,
    })
    .toBeGreaterThan(percent - 2);
  expect(Math.round((await titleShare(page)) * 100)).toBeLessThan(percent + 2);
};
const saved = (page) =>
  page.waitForResponse(
    (r) =>
      r.url().includes("/api/book-layout/save") &&
      r.request().method() === "POST" &&
      r.ok(),
  );
const restoreResponse = (page) =>
  page.waitForResponse((r) => r.url().includes("/api/book-layout/restore"));
const bar = (page) =>
  page.getByRole("status").filter({ hasText: "Previewing a version from" });

test("setup: register and create a Memory Book", async ({ browser }) => {
  test.skip(
    !BASE || !OTP || !DSN,
    "run through `make e2e` (scripts/e2e.sh starts the API and the web server)",
  );
  const page = await (
    await browser.newContext({
      locale: "en-US",
      baseURL: BASE,
      viewport: { width: 1280, height: 900 },
    })
  ).newPage();
  await step("register and verify the email", async () => {
    await page.goto("/register");
    await page.getByLabel("Your name").fill(OWNER.name);
    await page.getByLabel("Email").fill(OWNER.email);
    await page.getByLabel("Password", { exact: true }).fill(OWNER.password);
    await page
      .getByLabel("I agree to the Terms of Use and the Privacy Policy")
      .check();
    const registered = page.waitForResponse(
      (r) =>
        r.url().includes("/api/auth/register") &&
        r.request().method() === "POST",
    );
    await page.getByRole("button", { name: "Create account" }).click();
    if ((await registered).status() === 429)
      throw new Error(
        "registration is rate limited: run `make reset-limits SMEM_ENV=test` (make e2e does)",
      );
    await expect(
      page.getByRole("heading", { name: "Verify your email" }),
    ).toBeVisible();
    await page.getByLabel("6-digit code").fill(OTP);
    await expect(
      page.getByText("Your email address is verified."),
    ).toBeVisible();
  });
  await step("create the book", async () => {
    await page.goto("/yearbooks/new");
    await page.getByLabel("Title").fill(TITLE);
    await page.getByLabel("Yearbook language").selectOption("vi");
    await page.getByLabel("Page size").selectOption("A5");
    await page.getByRole("button", { name: "Create yearbook" }).click();
    await expect(
      page.getByRole("heading", { level: 1, name: TITLE }),
    ).toBeVisible();
    bookId = new URL(page.url()).pathname.split("/").pop();
    expect(
      (
        await post(page, "/api/yearbook/update", {
          id: bookId,
          template_id: "memory-book",
        })
      ).ok(),
    ).toBe(true);
  });
});

test("three bursts, open History, preview, back to current, restore, reload, undo does nothing (AC1, AC4)", async ({
  browser,
}) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  const calls = [];
  page.on(
    "request",
    (r) =>
      /\/api\/book-layout\/(version-|restore)/.test(r.url()) &&
      calls.push(new URL(r.url()).pathname),
  );
  await freshHistory(page);
  await openEditor(page);
  await step(
    "four edits of the cover title width, each a separate burst (the snapshots are back-dated)",
    async () => {
      for (const w of [30, 40, 50, 60]) {
        await page.getByRole("button", { name: "Cover title" }).click();
        const done = saved(page);
        await page
          .getByRole("spinbutton", { name: "Width", exact: true })
          .fill(String(w));
        await page
          .getByRole("spinbutton", { name: "Width", exact: true })
          .press("Enter");
        await done;
        backdate();
      }
      expect(await titleWidth(page)).toBe(600);
      expect(
        calls,
        "nothing of the history was asked for while editing",
      ).toEqual([]);
    },
  );
  await step(
    "open History: one list call, three Autosave rows, newest first",
    async () => {
      await openHistory(page);
      expect(calls).toEqual(["/api/book-layout/version-list"]);
      await expect(rows(page)).toHaveCount(3);
      for (let i = 0; i < 3; i++)
        await expect(rows(page).nth(i)).toHaveAccessibleName(
          /minutes ago, Autosave, 0 items/,
        );
    },
  );
  await step(
    "preview the oldest: 30 % wide, read-only, the current work unchanged underneath",
    async () => {
      await rows(page).nth(2).click();
      await expect(bar(page)).toBeVisible();
      await expectShare(page, 30);
      await expect(
        page.getByRole("button", { name: "Add text" }),
      ).toBeDisabled();
      await page.waitForTimeout(2000); // longer than the 1.5 s debounce: a preview must not save
      expect(await titleWidth(page)).toBe(600);
      await page.getByRole("button", { name: "Back to current" }).click();
      await expect(bar(page)).toBeHidden();
      await expectShare(page, 60);
      await rows(page).nth(2).click(); // again: from the cache, no second version-get
      await expect(bar(page)).toBeVisible();
      expect(calls.filter((c) => c.endsWith("version-get"))).toHaveLength(1);
    },
  );
  await step(
    "restore it: confirm text, toast, undo is empty, the server has it",
    async () => {
      await page.getByRole("button", { name: "Restore this version" }).click();
      await expect(page.getByRole("dialog")).toContainText(
        "Your current version is kept in the history",
      );
      const done = restoreResponse(page);
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Restore this version" })
        .click();
      expect((await done).status()).toBe(200);
      await expect(
        page.getByText("Version restored.", { exact: true }).first(),
      ).toBeVisible();
      await expectShare(page, 30);
      await expect(page.getByRole("button", { name: "Undo" })).toBeDisabled();
      await expect(page.getByRole("button", { name: "Redo" })).toBeDisabled();
      expect(await titleWidth(page)).toBe(300);
      await expect(rows(page)).toHaveCount(4); // the replaced 60 % document is a new row
      await expect(rows(page).first()).toHaveAccessibleName(/Before restore/);
    },
  );
  await step(
    "Ctrl/Cmd+Z does nothing; after a reload the restored layout is still there",
    async () => {
      await page.keyboard.press("ControlOrMeta+z");
      await page.waitForTimeout(2000);
      await expectShare(page, 30);
      expect(await titleWidth(page)).toBe(300);
      await page.reload();
      await expect(
        page.locator(".ed-pagebox .bk-sheet[data-page='cover']"),
      ).toBeVisible();
      await expectShare(page, 30);
      await expect(page.getByRole("button", { name: "Undo" })).toBeDisabled();
    },
  );
});

test("a design image deleted after the snapshot: the preview counts it, the restore drops it and says so (AC2, AC3)", async ({
  browser,
}) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  let mediaId = "";
  await freshHistory(page);
  await step(
    "upload a design image, save a layout that uses it, save another layout; the first becomes a snapshot",
    async () => {
      const f = png(64, 64, 3);
      const res = await page.request.post("/api/media/create", {
        headers: { Origin: BASE },
        multipart: {
          yearbook_id: bookId,
          purpose: "design",
          file: { name: "d.png", mimeType: "image/png", buffer: f },
        },
      });
      expect(res.ok(), `upload ${res.status()}`).toBe(true);
      mediaId = (await res.json()).data.id;
      await saveLayout(page, {
        v: 1,
        pages: {
          cover: {
            items: [
              {
                id: "imgAAA",
                t: "image",
                media: mediaId,
                x: 100,
                w: 400,
                h: 300,
                cy: 500,
                fit: "cover",
              },
            ],
          },
        },
      });
      await saveLayout(page, {
        v: 1,
        pages: {
          cover: {
            items: [
              {
                id: "txtBBB",
                t: "text",
                text: "Xin chào",
                x: 100,
                w: 300,
                h: 100,
                cy: 300,
              },
            ],
          },
        },
      });
      expect(
        (await post(page, "/api/media/delete", { id: mediaId })).ok(),
      ).toBe(true);
    },
  );
  await openEditor(page);
  await openHistory(page);
  await step("preview: one item will be dropped", async () => {
    await expect(rows(page)).toHaveCount(1);
    await rows(page).first().click();
    await expect(bar(page)).toContainText("1 item will be dropped");
  });
  await step(
    "restore: the message names the dropped item, and the stored layout never mentions the image",
    async () => {
      await page.getByRole("button", { name: "Restore this version" }).click();
      const done = restoreResponse(page);
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Restore this version" })
        .click();
      const answer = await done;
      expect(answer.status()).toBe(200);
      expect((await answer.json()).data.dropped).toBe(1);
      await expect(
        page
          .getByText(
            /1 item could not be restored because its image or sticker no longer exists/,
          )
          .first(),
      ).toBeVisible();
      expect(JSON.stringify(await storedLayout(page))).not.toContain(mediaId);
    },
  );
});

test("two tabs restoring the same version: one succeeds, the other gets the conflict and nothing is overwritten (AC3)", async ({
  browser,
}) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const first = await ownerPage(browser);
  const second = await ownerPage(browser, first.context());
  await calm(first);
  await freshHistory(first);
  await saveLayout(first, widthDoc(300));
  backdate();
  await saveLayout(first, widthDoc(700));
  await openEditor(first);
  await openEditor(second);
  for (const p of [first, second]) {
    await openHistory(p);
    await rows(p).first().click();
    await expect(bar(p)).toBeVisible();
    await expectShare(p, 30);
  }
  await step("tab 1 restores", async () => {
    await first.getByRole("button", { name: "Restore this version" }).click();
    const done = restoreResponse(first);
    await first
      .getByRole("dialog")
      .getByRole("button", { name: "Restore this version" })
      .click();
    expect((await done).status()).toBe(200);
    expect(await titleWidth(first)).toBe(300);
  });
  await step(
    "tab 2 restores with an old version: 409, the conflict banner, the stored document is kept",
    async () => {
      await second
        .getByRole("button", { name: "Restore this version" })
        .click();
      const done = restoreResponse(second);
      await second
        .getByRole("dialog")
        .getByRole("button", { name: "Restore this version" })
        .click();
      expect((await done).status()).toBe(409);
      await expect(second.getByText(/Editing is paused/)).toBeVisible();
      await expect(
        second.getByText(/another tab or device, so nothing was restored/),
      ).toBeVisible();
      expect(await titleWidth(second)).toBe(300);
      await second
        .getByRole("button", { name: "Use the saved version" })
        .click();
      await expect(second.getByText(/Editing is paused/)).toBeHidden();
      await expectShare(second, 30);
    },
  );
});

test("keyboard only: open History, walk the rows, preview, restore; Escape gives focus back (AC5)", async ({
  browser,
}) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  await calm(page);
  await freshHistory(page);
  await saveLayout(page, widthDoc(300));
  backdate();
  await saveLayout(page, widthDoc(500));
  backdate();
  await saveLayout(page, widthDoc(700));
  await openEditor(page);
  await historyButton(page).focus();
  await page.keyboard.press("Enter");
  await expect(rows(page)).toHaveCount(2);
  await page.keyboard.press("Tab"); // Refresh
  await page.keyboard.press("Tab"); // the first row
  await expect(rows(page).first()).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await expect(rows(page).nth(1)).toBeFocused();
  await page.keyboard.press("ArrowUp");
  await page.keyboard.press("Enter");
  await expect(bar(page)).toBeVisible();
  await expectShare(page, 50);
  await page.keyboard.press("Shift+Tab"); // Refresh
  await page.keyboard.press("Shift+Tab"); // Back to current
  await page.keyboard.press("Shift+Tab"); // Restore this version
  await expect(
    page.getByRole("button", { name: "Restore this version" }),
  ).toBeFocused();
  await page.keyboard.press("Enter");
  await page.keyboard.press("Tab"); // from Cancel to the confirm button
  const done = restoreResponse(page);
  await page.keyboard.press("Enter");
  expect((await done).status()).toBe(200);
  await expectShare(page, 50);
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("radiogroup", { name: "Earlier versions" }),
  ).toBeHidden();
  await expect(historyButton(page)).toBeFocused();
});
