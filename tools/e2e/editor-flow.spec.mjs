// The editing flow end to end (T-170, docs/editor-flow.md, docs/e2e.md "The editing flow"): manage page -> "Edit page by page" -> every page -> Done -> manage page;
// deep links; Done with a forced 409 stays; Print waits for the save; leaving with unsaved changes asks.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
// Every step is a test.step with a hard limit; no fixed sleeps. Credentials: the registered fixed email code (DEV-SHORTCUT(otp), SMEM_ENV=test only).
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 60_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Đặng Thị Bích Ngọc", email: `e2e-flow-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9` };
const TITLE = `Lớp 10C3 – Luồng sửa ${RUN}`;

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

test("editing flow: page by page, Done, deep links, a refused save, print, the leave guard", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const owner = await (await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } })).newPage();
  let bookId = "";
  const api = (p, data) => owner.request.post(p, { data, headers: { Origin: BASE } });
  const hideProfile = async (hidden) => {
    const res = await api("/api/book-design/update", { yearbook_id: bookId, options: { sections: { hidden } } });
    expect(res.ok(), `book-design/update ${res.status()}`).toBe(true);
  };
  const stored = async () => (await (await owner.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json()).data;
  const storedWidth = async () => (await stored()).layout?.pages?.cover?.overrides?.["cover.title"]?.w;
  const position = () => owner.getByTestId("page-position");
  const setWidth = async (v) => {
    await owner.getByRole("button", { name: "Cover title" }).click();
    const w = owner.getByRole("spinbutton", { name: "Width", exact: true });
    await w.fill(String(v));
    await w.press("Enter");
  };
  const saved = () => owner.waitForResponse((r) => r.url().includes("/api/book-layout/save") && r.request().method() === "POST" && r.ok());
  const manage = () => owner.goto(`/yearbooks/${bookId}`);

  await step("1. register, create a Memory Book (A5, Vietnamese); the profile section is hidden", async () => {
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
    await owner.getByLabel("6-digit code").fill(OTP);
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
    await hideProfile(["profile"]);
  });

  await step("2. manage page -> Edit page by page -> next through every page (the hidden one is skipped) -> Done -> manage page", async () => {
    await manage();
    await expect(owner.getByRole("heading", { name: "Design the pages" })).toBeVisible();
    await expect(owner.getByRole("link", { name: "Edit cover" })).toBeVisible();
    await expect(owner.getByRole("link", { name: "Edit back page" })).toBeVisible();
    await expect(owner.getByRole("link", { name: "Edit profile page" })).toHaveCount(0);
    await owner.getByRole("link", { name: "Edit page by page" }).click();
    await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}/editor$`));
    await expect(position()).toHaveText("Page 1 of 2: Cover");
    const prev = owner.getByRole("button", { name: "Previous page" });
    const next = owner.getByRole("button", { name: "Next page" });
    await expect(prev).toHaveAttribute("aria-disabled", "true");
    await setWidth(40);
    await next.click();
    await expect(position()).toHaveText("Page 2 of 2: Back cover");
    await expect(owner.locator(".ed-pagebox .bk-sheet[data-page='back']")).toBeVisible();
    await expect(next).toHaveAttribute("aria-disabled", "true");
    await expect(owner.getByRole("navigation", { name: "Pages" }).getByRole("button", { name: /^Page 2 of 2/ })).toHaveAttribute("aria-current", "page");
    await owner.keyboard.press("PageUp"); // the key does what the button does
    await expect(position()).toHaveText("Page 1 of 2: Cover");
    await owner.getByRole("button", { name: "Done" }).click();
    await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}$`));
    await expect(owner.getByRole("heading", { name: "Design the pages" })).toBeVisible();
    expect(await storedWidth(), "Done saved the edit before it left").toBe(400);
    // Back from the manage page returns to the editor, which shows the saved width
    await owner.goBack();
    await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}/editor$`));
    await owner.getByRole("button", { name: "Cover title" }).click();
    await expect(owner.getByRole("spinbutton", { name: "Width", exact: true })).toHaveValue("40");
  });

  await step("3. deep links open the right page; unknown values and hidden sections open the cover", async () => {
    await hideProfile([]);
    await manage();
    await owner.getByRole("link", { name: "Edit profile page" }).click();
    await expect(position()).toHaveText("Page 2 of 3: Profile");
    await expect(owner.locator(".ed-pagebox .bk-sheet[data-page='profile']")).toBeVisible();
    await manage();
    await owner.getByRole("link", { name: "Edit back page" }).click();
    await expect(position()).toHaveText("Page 3 of 3: Back cover");
    await manage();
    await owner.getByRole("link", { name: "Edit cover" }).click();
    await expect(position()).toHaveText("Page 1 of 3: Cover");
    await owner.goto(`/yearbooks/${bookId}/editor?page=nonsense`);
    await expect(position()).toHaveText("Page 1 of 3: Cover");
    await hideProfile(["profile"]);
    await owner.goto(`/yearbooks/${bookId}/editor?page=profile`);
    await expect(position()).toHaveText("Page 1 of 2: Cover");
  });

  await step("4. Done with a forced 409 stays in the editor and loses nothing", async () => {
    await owner.goto(`/yearbooks/${bookId}/editor`);
    await expect(position()).toHaveText("Page 1 of 2: Cover");
    await owner.route("**/api/book-layout/save", (route) =>
      route.fulfill({
        status: 409,
        contentType: "application/json",
        body: JSON.stringify({ status: "ERROR_VERSION_CONFLICT", error_message: "version conflict", request_id: "r", data: undefined }),
      }),
    );
    try {
      await setWidth(55);
      await owner.getByRole("button", { name: "Done" }).click();
      await expect(owner.getByRole("alert").filter({ hasText: "the editor stays open" })).toBeVisible();
      await expect(owner.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
      await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}/editor$`));
      await expect(owner.getByText(/changed in another tab/).first()).toBeVisible();
    } finally {
      await owner.unroute("**/api/book-layout/save");
    }
    expect(await storedWidth(), "nothing was written by the refused save").toBe(400);
    // keeping my version is a fresh save; Done then works
    await owner.getByRole("button", { name: "Keep my version" }).click();
    await expect.poll(storedWidth).toBe(550);
    await owner.getByRole("button", { name: "Done" }).click();
    await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}$`));
  });

  await step("5. Print waits for the save, opens the print route in a new tab and leaves the editor intact", async () => {
    await owner.goto(`/yearbooks/${bookId}/editor`);
    await expect(position()).toHaveText("Page 1 of 2: Cover");
    const link = owner.getByRole("link", { name: "Print or save as PDF" });
    await expect(link).toHaveAccessibleDescription(/all pages of the book, A5 \(148 × 210 mm\)/);
    await owner.route("**/api/book-layout/save", async (route) => {
      await new Promise((r) => setTimeout(r, 1500));
      await route.continue();
    });
    try {
      await setWidth(35);
      const popup = owner.context().waitForEvent("page");
      await link.click();
      const tab = await popup;
      await tab.waitForURL(/\/book$/, { timeout: STEP_MS });
      expect(await storedWidth(), "the print route opened after the save settled").toBe(350);
      await tab.close();
    } finally {
      await owner.unroute("**/api/book-layout/save");
    }
    await expect(owner.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
    await expect(position()).toHaveText("Page 1 of 2: Cover");
  });

  await step("6. leaving with unsaved changes asks first (a link, the tab, Back); Done then leaves without a question", async () => {
    await owner.route("**/api/book-layout/save", async (route) => {
      await new Promise((r) => setTimeout(r, 4000));
      await route.continue();
    });
    try {
      await setWidth(45); // dirty for the next 4 s
      await owner.getByRole("link", { name: "Back to customise" }).click();
      await expect(owner.getByRole("dialog").getByText("Leave the editor?")).toBeVisible();
      await owner.getByRole("button", { name: "Stay" }).click();
      await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}/editor$`));
      const seen = [];
      owner.on("dialog", (d) => {
        seen.push(d.type());
        void d.dismiss();
      });
      await owner.close({ runBeforeUnload: true });
      await expect.poll(() => seen, { message: "the browser's own prompt for closing the tab" }).toEqual(["beforeunload"]);
      expect(owner.isClosed(), "dismissing the prompt keeps the tab").toBe(false);
      owner.removeAllListeners("dialog");
      await owner.goBack(); // the browser's Back asks too
      await expect(owner.getByRole("dialog").getByText("Leave the editor?")).toBeVisible();
      await owner.getByRole("button", { name: "Stay" }).click();
    } finally {
      await owner.unroute("**/api/book-layout/save");
    }
    await owner.getByRole("button", { name: "Done" }).click();
    await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}$`));
    expect(await storedWidth()).toBe(450);
  });
});
