// QA probes for T-170 (not part of the dev's editor-flow spec): a whole book with notes walked by keyboard only, the notes page label (spec 29 requirement 1),
// Done offline, a real second tab (409), Done then the browser's Back, and the stage 1 customise leave guard (Back, link, tab) which now shares useLeaveGuard.
// Run: cd tools/e2e && npx playwright test -c <config with testMatch editor-flow-qa.spec.mjs> against a running API and web server (E2E_BASE_URL, SMEM_DEV_FIXED_OTP).
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp)
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Lê Minh Anh", email: `e2e-flowqa-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9` };
const TITLE = `Lớp 11A2 – QA luồng ${RUN}`;

test.afterAll(async () => {
  if (!BASE) return;
  const api = await request.newContext({ baseURL: BASE, extraHTTPHeaders: { Origin: BASE } });
  try {
    const login = await api.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password } });
    if (!login.ok()) return;
    const list = await (await api.get("/api/yearbook/get-list")).json();
    for (const y of list.data?.items ?? []) await api.post("/api/yearbook/delete", { data: { id: y.id } });
  } finally {
    await api.dispose();
  }
});

test("T-170 QA: keyboard walk with a notes page, offline Done, two tabs, Back after Done, customise guard", async ({ browser }) => {
  test.skip(!BASE || !OTP, "needs a running API and web server");
  test.setTimeout(240_000);
  const ctx = await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } });
  const owner = await ctx.newPage();
  const friend = await (await browser.newContext({ locale: "en-US", baseURL: BASE })).newPage();
  let bookId = "";
  const api = (p, data) => owner.request.post(p, { data, headers: { Origin: BASE } });
  const position = (p = owner) => p.getByTestId("page-position");
  const stored = async () => (await (await owner.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json()).data;
  const storedWidth = async () => (await stored()).layout?.pages?.cover?.overrides?.["cover.title"]?.w;
  const setWidth = async (p, v) => {
    await p.getByRole("button", { name: "Cover title" }).click();
    const w = p.getByRole("spinbutton", { name: "Width", exact: true });
    await w.fill(String(v));
    await w.press("Enter");
  };
  const active = (p = owner) => p.evaluate(() => ({ tag: document.activeElement?.tagName, name: document.activeElement?.getAttribute("aria-label") || document.activeElement?.textContent?.trim().slice(0, 40) }));

  // set-up: account, book, approved note
  await owner.goto("/register");
  await owner.getByLabel("Your name").fill(OWNER.name);
  await owner.getByLabel("Email").fill(OWNER.email);
  await owner.getByLabel("Password", { exact: true }).fill(OWNER.password);
  await owner.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
  await owner.getByRole("button", { name: "Create account" }).click();
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
  expect((await api("/api/yearbook/update", { id: bookId, template_id: "memory-book" })).ok()).toBe(true);
  await owner.goto(`/yearbooks/${bookId}/note-links`);
  await owner.getByLabel("Name (optional)").fill("Class chat");
  await owner.getByRole("button", { name: "Create link" }).click();
  await expect(owner.getByRole("heading", { name: "Your link is ready" })).toBeVisible();
  const link = new URL(await owner.getByLabel("Link for Class chat").inputValue()).pathname;
  await friend.goto(link);
  await friend.getByLabel(/^Your name/).fill("Trần Bảo");
  await friend.getByLabel(/^Your message/).fill("Chúc bạn luôn vui!");
  const sent = friend.waitForResponse((r) => r.url().includes("/api/public-collect/submit-note") && r.ok());
  await friend.getByRole("button", { name: "Send my note" }).click();
  await sent;
  await owner.goto(`/yearbooks/${bookId}/notes`);
  await expect(owner.getByText("1 waiting for you · 0 in your book")).toBeVisible();
  await owner.getByRole("button", { name: "Approve Trần Bảo" }).click();
  await owner.getByRole("button", { name: /^Approved/ }).click();
  await expect(owner.locator(".inbox-card")).toHaveCount(1);

  // 1. keyboard-only walk through all four page kinds
  await owner.goto(`/yearbooks/${bookId}`);
  for (const n of ["Edit cover", "Edit profile page", "Edit notes pages", "Edit back page"]) await expect(owner.getByRole("link", { name: n })).toBeVisible();
  await owner.getByRole("link", { name: "Edit notes pages" }).focus();
  await owner.keyboard.press("Enter");
  await expect(position()).toHaveText("Page 3 of 4: Notes page 1");
  await expect(owner.getByText(/applies to every notes page/i), "spec 29 req. 1: the notes page says its layout is shared").toBeVisible();
  const next = owner.getByRole("button", { name: "Next page" });
  const prev = owner.getByRole("button", { name: "Previous page" });
  await next.focus();
  await owner.keyboard.press("Enter");
  await expect(position()).toHaveText("Page 4 of 4: Back cover");
  console.log("QA-FOCUS-AFTER-NEXT", JSON.stringify(await active()));
  await owner.keyboard.press("Enter"); // aria-disabled at the end: nothing happens
  await expect(position()).toHaveText("Page 4 of 4: Back cover");
  await prev.focus();
  await owner.keyboard.press("Space");
  await expect(position()).toHaveText("Page 3 of 4: Notes page 1");
  await owner.keyboard.press("PageUp");
  await expect(position()).toHaveText("Page 2 of 4: Profile");
  await owner.keyboard.press("PageUp");
  await expect(position()).toHaveText("Page 1 of 4: Cover");
  await owner.keyboard.press("PageUp");
  await expect(position()).toHaveText("Page 1 of 4: Cover");
  // PageDown inside a number field must not change the page
  await owner.getByRole("button", { name: "Cover title" }).click();
  await owner.getByRole("spinbutton", { name: "Width", exact: true }).focus();
  await owner.keyboard.press("PageDown");
  await expect(position()).toHaveText("Page 1 of 4: Cover");
  await owner.getByRole("button", { name: "Cover title" }).focus();
  await owner.keyboard.press("Escape");

  // 2. Done offline stays, nothing lost; online again Done works
  await setWidth(owner, 42);
  await ctx.setOffline(true);
  await setWidth(owner, 43);
  await owner.getByRole("button", { name: "Done" }).focus();
  await owner.keyboard.press("Enter");
  await expect(owner.getByRole("alert").filter({ hasText: "the editor stays open" })).toBeVisible();
  await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}/editor`));
  await expect(owner.getByRole("spinbutton", { name: "Width", exact: true })).toHaveValue("43");
  await ctx.setOffline(false);
  await owner.getByRole("button", { name: "Done" }).click();
  await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}$`));
  expect(await storedWidth()).toBe(430);
  console.log("QA-FOCUS-AFTER-DONE", JSON.stringify(await active()));

  // 3. Back after Done goes to the editor (not a loop), Forward returns to the manage page
  // The URL changes before React commits the new screen: Back pressed in that gap cancels the navigation and leaves the editor mounted on the page it was on
  // (a test race, not a remount; found by running this spec 20 times with logging). Wait for the manage page, then go Back.
  await expect(owner.getByRole("link", { name: "Edit notes pages" })).toBeVisible();
  await owner.goBack();
  await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}/editor`));
  await expect(position()).toHaveText("Page 3 of 4: Notes page 1"); // the entry was the deep link ?page=notes, so Back opens that page again
  await owner.goForward();
  await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}$`));

  // 4. a real second tab: B saves a new width, A (stale) edits and presses Done: conflict, A stays, B's value stays stored
  await owner.goto(`/yearbooks/${bookId}/editor`);
  await expect(position()).toHaveText("Page 1 of 4: Cover");
  const tabB = await ctx.newPage();
  await tabB.goto(`/yearbooks/${bookId}/editor`);
  await expect(position(tabB)).toHaveText("Page 1 of 4: Cover");
  const savedB = tabB.waitForResponse((r) => r.url().includes("/api/book-layout/save") && r.ok());
  await setWidth(tabB, 60);
  await savedB;
  await setWidth(owner, 50);
  await owner.getByRole("button", { name: "Done" }).click();
  await expect(owner.getByRole("alert").filter({ hasText: "the editor stays open" })).toBeVisible();
  await expect(owner).toHaveURL(new RegExp(`/yearbooks/${bookId}/editor`));
  expect(await storedWidth(), "tab B's save stays").toBe(600);
  await tabB.close();

  // 5. the stage 1 customise guard (useLeaveGuard changed): a link, Back, the tab
  await owner.goto(`/yearbooks/${bookId}/customise`);
  const colours = owner.getByRole("group", { name: "Colours", exact: true });
  await expect(colours).toBeVisible();
  await colours.getByRole("radio", { name: /Peach/ }).check();
  await expect(owner.getByText("You have changes that are not saved.")).toBeVisible();
  await owner.getByRole("link", { name: "Free layout" }).click();
  await expect(owner.getByRole("dialog").getByText("Leave without saving?")).toBeVisible();
  await owner.getByRole("button", { name: "Stay here" }).click();
  await expect(owner).toHaveURL(new RegExp(`/customise$`));
  await owner.goBack();
  await expect(owner.getByRole("dialog").getByText("Leave without saving?")).toBeVisible();
  await owner.getByRole("button", { name: "Stay here" }).click();
  await expect(owner).toHaveURL(new RegExp(`/customise$`));
  // save, the guard releases, then a link leaves without a question
  const saved = owner.waitForResponse((r) => r.url().includes("/api/book-design/update") && r.ok());
  await owner.getByRole("button", { name: "Save design" }).click();
  await saved;
  await owner.getByRole("link", { name: "Free layout" }).click();
  await expect(owner).toHaveURL(new RegExp(`/editor$`));
  await owner.goBack();
  await expect(owner).toHaveURL(new RegExp(`/customise$`)); // one Back, not two: the sentinel entry is gone
});
