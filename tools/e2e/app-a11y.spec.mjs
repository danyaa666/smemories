// Skip link and focus after a route change, app-wide (T-187, findings F-08 and F-09 of docs/editor-accessibility.md), keyboard only, English and Vietnamese.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
import { expect, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Lê Thị Hoa", email: `e2e-app-a11y-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9` };

/** The focused element is the page's <h1> (tabindex -1) and it names the page. */
async function expectTitleFocused(page, name) {
  const h1 = page.getByRole("heading", { level: 1, name });
  await expect(h1).toBeFocused();
  await expect(h1).toHaveAttribute("tabindex", "-1");
}

test("skip link and route focus, keyboard only, en and vi", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const page = await (await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } })).newPage();
  await page.goto("/register");
  await page.getByLabel("Your name").fill(OWNER.name);
  await page.getByLabel("Email").fill(OWNER.email);
  await page.getByLabel("Password", { exact: true }).fill(OWNER.password);
  await page.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
  const registered = page.waitForResponse((r) => r.url().includes("/api/auth/register") && r.request().method() === "POST");
  await page.getByRole("button", { name: "Create account" }).click();
  if ((await registered).status() === 429) throw new Error("registration is rate limited: run `make reset-limits SMEM_ENV=test` (make e2e does)");
  await page.getByLabel("6-digit code").fill(OTP);
  await expect(page.getByText("Your email address is verified.")).toBeVisible();

  for (const [lang, skip, classes] of [
    ["en", "Skip to content", "My classes"],
    ["vi", "Bỏ qua đến nội dung", null],
  ]) {
    await page.goto("/yearbooks"); // a full load: the focus stays on the page, not on a title
    if (lang === "vi") await page.getByRole("group", { name: "Language" }).getByRole("button", { name: "Tiếng Việt" }).click();
    const skipLink = page.getByRole("link", { name: skip });
    if (lang === "vi") await page.reload(); // the language is kept; a fresh load starts Tab at the top (the button clicked above keeps the Tab start)
    await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible(); // the page is drawn before the first Tab
    await page.keyboard.press("Tab");
    await expect(skipLink).toBeFocused();
    const box = await skipLink.boundingBox();
    expect(box && box.y >= 0 && box.y + box.height <= 900, "the skip link is on screen when focused").toBe(true);
    await page.keyboard.press("Enter");
    await expect(page.getByRole("main")).toBeFocused();
    expect(new URL(page.url()).hash, "the skip link does not change the URL").toBe("");

    // Route change by keyboard: Yearbooks -> Classes through the sidebar.
    const nav = page.getByRole("navigation").first();
    await nav.getByRole("link").nth(1).focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/classes$/);
    const heading = page.getByRole("main").getByRole("heading", { level: 1 });
    await expect(heading).toBeFocused();
    await expect(heading).toHaveAttribute("tabindex", "-1");
    if (lang === "en") await expectTitleFocused(page, classes);
    // The next Tab goes on from the title, not from the removed link.
    await page.keyboard.press("Tab");
    expect(await page.evaluate(() => document.querySelector("main").contains(document.activeElement))).toBe(true);
  }
});

test("public pages (App layout): the first load keeps the focus on the page, the skip link is the first Tab stop, and a link click moves the focus to the new h1", async ({ browser }) => {
  test.skip(!BASE, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const page = await (await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } })).newPage();
  await page.goto("/login");
  await expect(page.getByRole("main").getByRole("heading", { level: 1 })).toBeVisible();
  expect(await page.evaluate(() => document.activeElement === document.body), "first load does not move focus").toBe(true);
  await page.keyboard.press("Tab");
  const skip = page.getByRole("link", { name: "Skip to content" });
  await expect(skip).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("main")).toBeFocused();
  // Route change by keyboard: the "Create account" link of the login page.
  await page.getByRole("main").getByRole("link", { name: /create (an )?account|register|sign up/i }).first().focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/register$/);
  const h1 = page.getByRole("main").getByRole("heading", { level: 1 });
  await expect(h1).toBeFocused();
  await expect(h1).toHaveAttribute("tabindex", "-1");
});
