// QA probe of T-170 round 3, committed by T-168: deep link into a notes page with a book-data cache that predates the approved note (stale-cache hardening),
// R1 text en+vi and the live region, R3 h1 focus after Done. The two "Done then Back" timing probes of the draft only logged numbers and are not kept.
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp)
const RUN = () =>
  `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const emails = [];

test.afterAll(async () => {
  if (!BASE) return;
  for (const o of emails) {
    const api = await request.newContext({
      baseURL: BASE,
      extraHTTPHeaders: { Origin: BASE },
    });
    try {
      const login = await api.post("/api/auth/login", {
        data: { email: o.email, password: o.password },
      });
      if (!login.ok()) continue;
      const list = await (await api.get("/api/yearbook/get-list")).json();
      for (const y of list.data?.items ?? [])
        await api.post("/api/yearbook/delete", { data: { id: y.id } });
    } finally {
      await api.dispose();
    }
  }
});

async function setup(owner, tag) {
  const run = RUN();
  const o = {
    name: "Lê Minh Anh",
    email: `e2e-${tag}-${run}@example.com`,
    password: `Mật-khẩu-${run}-9`,
    title: `Lớp 11A2 – ${tag} ${run}`,
  };
  emails.push(o);
  await owner.goto("/register");
  await owner.getByLabel("Your name").fill(o.name);
  await owner.getByLabel("Email").fill(o.email);
  await owner.getByLabel("Password", { exact: true }).fill(o.password);
  await owner
    .getByLabel("I agree to the Terms of Use and the Privacy Policy")
    .check();
  await owner.getByRole("button", { name: "Create account" }).click();
  await owner.getByLabel("6-digit code").fill(OTP);
  await expect(
    owner.getByText("Your email address is verified."),
  ).toBeVisible();
  await owner.getByRole("link", { name: "Continue" }).click();
  await owner.goto("/yearbooks/new");
  await owner.getByLabel("Title").fill(o.title);
  await owner.getByLabel("Yearbook language").selectOption("vi");
  await owner.getByLabel("Page size").selectOption("A5");
  await owner.getByRole("button", { name: "Create yearbook" }).click();
  await expect(
    owner.getByRole("heading", { level: 1, name: o.title }),
  ).toBeVisible();
  const bookId = new URL(owner.url()).pathname.split("/").pop();
  expect(
    (
      await owner.request.post("/api/yearbook/update", {
        data: { id: bookId, template_id: "memory-book" },
        headers: { Origin: BASE },
      })
    ).ok(),
  ).toBe(true);
  return { ...o, bookId };
}

async function approveNote(browser, tabB, bookId) {
  await tabB.goto(`/yearbooks/${bookId}/note-links`);
  await tabB.getByLabel("Name (optional)").fill("Class chat");
  await tabB.getByRole("button", { name: "Create link" }).click();
  await expect(
    tabB.getByRole("heading", { name: "Your link is ready" }),
  ).toBeVisible();
  const link = new URL(
    await tabB.getByLabel("Link for Class chat").inputValue(),
  ).pathname;
  const friend = await (
    await browser.newContext({ locale: "en-US", baseURL: BASE })
  ).newPage();
  await friend.goto(link);
  await friend.getByLabel(/^Your name/).fill("Trần Bảo");
  await friend.getByLabel(/^Your message/).fill("Chúc bạn luôn vui!");
  const sent = friend.waitForResponse(
    (r) => r.url().includes("/api/public-collect/submit-note") && r.ok(),
  );
  await friend.getByRole("button", { name: "Send my note" }).click();
  await sent;
  await tabB.goto(`/yearbooks/${bookId}/notes`);
  await expect(
    tabB.getByText("1 waiting for you · 0 in your book"),
  ).toBeVisible();
  await tabB.getByRole("button", { name: "Approve Trần Bảo" }).click();
  await tabB.getByRole("button", { name: /^Approved/ }).click();
  await expect(tabB.locator(".inbox-card")).toHaveCount(1);
}

test("T-170 QA stale cache: deep link to notes with a book-data cache that predates the note; R1 en+vi, R3", async ({
  browser,
}) => {
  test.skip(!BASE || !OTP, "needs a running API and web server");
  test.setTimeout(240_000);
  const ctx = await browser.newContext({
    locale: "en-US",
    baseURL: BASE,
    viewport: { width: 1280, height: 900 },
  });
  const A = await ctx.newPage();
  const o = await setup(A, "stale");
  const position = A.getByTestId("page-position");
  const live = A.locator('.visually-hidden[role="status"]');

  // 1. open the editor by client-side navigation: book-data is cached with NO notes page; Done -> manage page (client-side, cache survives)
  await A.goto(`/yearbooks/${o.bookId}`);
  await A.getByRole("link", { name: "Edit cover" }).click();
  await expect(position).toHaveText("Page 1 of 3: Cover");
  await A.getByRole("button", { name: "Done" }).click();
  await expect(A.getByRole("link", { name: "Edit cover" })).toBeVisible();
  // R3: focus on the manage page heading, not a button labelled Done
  const focus = await A.evaluate(() => ({
    tag: document.activeElement?.tagName,
    text: document.activeElement?.textContent?.trim().slice(0, 60),
  }));
  console.log("QA-R3-FOCUS", JSON.stringify(focus));
  expect(focus.tag).toBe("H1");
  expect(focus.text).toContain(o.title);
  await A.evaluate(() => (window.__noReload = true));

  // 2. another tab approves a note: the cache of tab A predates it
  const B = await ctx.newPage();
  await approveNote(browser, B, o.bookId);
  await A.bringToFront();

  // 3. tab A (never reloaded): client-side route to the notes deep link
  const link = A.getByRole("link", { name: "Edit notes pages" });
  let viaLink = true;
  try {
    await expect(link).toBeVisible({ timeout: 8000 });
    await link.click();
  } catch {
    viaLink = false; // the manage page itself is stale and has no link yet: route client-side to the same URL
    await A.evaluate((u) => {
      history.pushState({}, "", u);
      dispatchEvent(new PopStateEvent("popstate"));
    }, `/yearbooks/${o.bookId}/editor?page=notes`);
  }
  console.log("QA-STALE-VIA-LINK", viaLink);
  await expect(position).toHaveText("Page 3 of 4: Notes page 1");
  expect(await A.evaluate(() => window.__noReload)).toBe(true); // still the same document: the cache was alive
  // R1: text visible, deep link makes no announcement
  await expect(A.getByTestId("notes-shared")).toHaveText(
    "This layout applies to every notes page.",
  );
  await expect(live).not.toContainText("every notes page");
  // R1 live region on arrival by stepping
  await A.getByRole("button", { name: "Previous page" }).click();
  await expect(position).toHaveText("Page 2 of 4: Profile");
  await expect(A.getByTestId("notes-shared")).toHaveCount(0);
  await A.getByRole("button", { name: "Next page" }).click();
  await expect(position).toHaveText("Page 3 of 4: Notes page 1");
  await expect(live).toContainText("This layout applies to every notes page.");
  console.log("QA-LIVE", JSON.stringify(await live.textContent()));

  // 4. user moved to profile, then fresh data must not pull them back (reload of the list happens on focus/refetch)
  await A.getByRole("button", { name: "Previous page" }).click();
  await A.evaluate(() => dispatchEvent(new Event("focus")));
  await A.waitForTimeout(1500);
  await expect(position).toHaveText("Page 2 of 4: Profile");

  // 5. vi: same text on a deep link in a Vietnamese UI
  const vi = await browser.newContext({ locale: "vi-VN", baseURL: BASE });
  expect(
    (
      await vi.request.post("/api/auth/login", {
        data: { email: o.email, password: o.password },
        headers: { Origin: BASE },
      })
    ).ok(),
  ).toBe(true);
  const V = await vi.newPage();
  await V.goto(`/yearbooks/${o.bookId}/editor?page=notes`);
  await expect(V.getByTestId("notes-shared")).toHaveText(
    "Bố cục này áp dụng cho mọi trang lời nhắn.",
  );
  await expect(V.getByTestId("page-position")).toContainText("3");
});
