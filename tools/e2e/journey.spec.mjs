// The M1 journey end to end (T-020, docs/e2e.md): one test, Chromium, the real web build, API, MySQL, Valkey and MinIO.
//   make e2e        (starts the API and the web server itself; see scripts/e2e.sh)
// Every step is a test.step, so a failure names the step. There is no fixed sleep: each action waits for the screen or the network
// response that proves it worked. The PDF checks reuse tools/print-check (poppler helpers).
//
// The link token is a bearer secret: it is only ever in `link`, never logged, and every error leaving a step is scrubbed of it.
import { expect, request, test } from "@playwright/test";
import { mkdtempSync, readFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { inspect, savePdf, text, words } from "../print-check/lib.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const PHOTO = (n) => path.join(HERE, "../print-check/fixtures", `photo-${String(n).padStart(2, "0")}.jpg`);
const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 60_000; // hard limit of one step
// Unique per run: nothing from an earlier run can be mistaken for this one.
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Phạm Thị Thu Hà", email: `e2e-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9` };
const TITLE = `Lớp 12A1 – Kỷ yếu ${RUN}`;
const QUOTE = "Học, học nữa, học mãi. Có công mài sắt có ngày nên kim, và bạn bè là điều đẹp nhất của tuổi học trò.";
// memory-book: one friend per page (web/src/book/templates/memory.tsx). The spec's "3 + ceil(n/4)" holds for the 4-per-page templates.
const NOTES_PER_PAGE = 1;
const NOTES = [
  { name: "Nguyễn Văn Đức", message: "Chúc bạn luôn khỏe và thành công nhé! 😄 Cảm ơn vì những buổi trưa ở căn tin." },
  { name: "Trần Thị Ánh Tuyết", message: "Mình nhớ chuyến dã ngoại ở Đà Lạt, cả lớp hát đến khuya." },
  { name: "Lê Hoàng Phương Uyên", message: "Giữ liên lạc nhé, hẹn ngày gặp lại ở trường cũ!" },
];

let link = ""; // /c/<token>
const secrets = []; // photo-link tokens (D-32): bearer secrets like the collection token, scrubbed from every error too

/** One named step with a hard time limit. A failure says which step broke; the token never leaves it, not even in an error message. */
async function step(title, body) {
  try {
    return await test.step(title, body, { timeout: STEP_MS });
  } catch (e) {
    if (!(e instanceof Error)) throw e;
    const tokens = [link.split("/").pop(), ...secrets].filter(Boolean); // none yet: nothing to hide
    const mark = `STEP FAILED: ${title}\n`;
    const scrub = (s) => tokens.reduce((out, token) => out.replaceAll(token, "<token>"), s);
    const message = mark + scrub(e.message);
    e.stack = e.stack ? mark + scrub(e.stack) : e.stack;
    e.message = message;
    throw e;
  }
}

const norm = (s) => s.normalize("NFC").replace(/\s+/g, " ").trim().toLocaleLowerCase("vi"); // the cover may set the title in capitals

/** Deletes what the run created (the account itself stays: there is no endpoint for that). Never fails the test. */
async function cleanUp() {
  if (!BASE) return;
  const api = await request.newContext({ baseURL: BASE, extraHTTPHeaders: { Origin: BASE } });
  try {
    const login = await api.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password } });
    if (!login.ok()) return; // the run failed before the account existed
    const list = await (await api.get("/api/yearbook/get-list")).json();
    for (const y of list.data?.items ?? []) await api.post("/api/yearbook/delete", { data: { id: y.id } });
  } catch (e) {
    console.warn(`clean-up failed (the run's data stays in the test database): ${e.message}`);
  } finally {
    await api.dispose();
  }
}

test.afterAll(cleanUp);

test("M1 journey: sign up, build a book, collect notes, print the PDF", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const owner = await (await browser.newContext({ locale: "en-US", baseURL: BASE })).newPage();
  const friend = await (await browser.newContext({ locale: "en-US", baseURL: BASE })).newPage(); // anonymous: no cookies
  let bookId = "";
  const bookUrl = (tail = "") => `/yearbooks/${bookId}${tail}`;

  await step("1. register and verify the email with the fixed development code", async () => {
    await owner.goto("/register");
    await owner.getByLabel("Your name").fill(OWNER.name);
    await owner.getByLabel("Email").fill(OWNER.email);
    await owner.getByLabel("Password", { exact: true }).fill(OWNER.password);
    await expect(owner.getByRole("button", { name: "Create account" })).toBeDisabled(); // until the Terms box is ticked
    await owner.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
    const registered = owner.waitForResponse((r) => r.url().includes("/api/auth/register") && r.request().method() === "POST");
    await owner.getByRole("button", { name: "Create account" }).click();
    if ((await registered).status() === 429) {
      throw new Error("registration is rate limited (5 per hour per address): run `make reset-limits SMEM_ENV=test` (make e2e does), or point SMEM_REDIS_URL at a fresh Redis db, then run again");
    }
    await expect(owner.getByRole("heading", { name: "Verify your email" })).toBeVisible();
    await owner.getByLabel("6-digit code").fill(OTP); // the sixth digit submits
    await expect(owner.getByText("Your email address is verified.")).toBeVisible();
    await owner.getByRole("link", { name: "Continue" }).click();
    await owner.getByRole("link", { name: "My yearbooks" }).click();
    await expect(owner.getByRole("heading", { name: "My yearbooks" })).toBeVisible();
    await expect(owner.locator(".user-chip-name span").last()).toHaveText(/^user-[0-9a-z]{8}$/);
  });

  await step("1b. an existing user whose accepted Terms are old is sent to the accept screen and continues after accepting", async () => {
    // The old version is simulated at the browser: get-me answers terms.required until the real accept-terms call has succeeded (the
    // gate itself, 403 ERROR_TERMS_REQUIRED, is covered by the API integration tests of T-111).
    let accepted = false;
    await owner.route("**/api/user/get-me", async (route) => {
      if (accepted) return route.continue();
      const res = await route.fetch();
      const json = await res.json();
      json.data.terms = { ...json.data.terms, accepted_version: "2020-01-01", required: true };
      return route.fulfill({ response: res, json });
    });
    await owner.goto("/yearbooks");
    await expect(owner.getByRole("heading", { name: "Our terms have changed" })).toBeVisible();
    await expect(owner).toHaveURL(/\/accept-terms$/);
    const accept = owner.getByRole("button", { name: "Accept and continue" });
    await expect(accept).toBeDisabled();
    await owner.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
    accepted = true;
    const done = owner.waitForResponse((r) => r.url().includes("/api/user/accept-terms") && r.request().method() === "POST");
    await accept.click();
    expect((await done).ok()).toBe(true);
    await expect(owner.getByRole("heading", { name: "My yearbooks" })).toBeVisible();
    await expect(owner).toHaveURL(/\/yearbooks$/);
    await owner.unroute("**/api/user/get-me");
  });

  await step("2. create the yearbook, fill the profile, upload the cover photo", async () => {
    await owner.getByRole("link", { name: "New yearbook" }).click();
    await owner.getByLabel("Title").fill(TITLE);
    await owner.getByLabel("School").fill("Trường THPT Chuyên Lê Hồng Phong");
    await owner.getByLabel("Yearbook language").selectOption("vi");
    await owner.getByLabel("Page size").selectOption("A5");
    await owner.getByRole("button", { name: "Create yearbook" }).click();
    await expect(owner.getByRole("heading", { level: 1, name: TITLE })).toBeVisible();
    bookId = new URL(owner.url()).pathname.split("/").pop();

    await owner.getByLabel("Full name").fill(OWNER.name);
    await owner.getByLabel("Favourite quote").fill(QUOTE);
    const saved = owner.waitForResponse((r) => r.url().includes("/api/yearbook/update-profile") && r.ok());
    await owner.getByRole("button", { name: "Save profile" }).click();
    await saved;

    const uploaded = owner.waitForResponse((r) => r.url().includes("/api/media/create") && r.ok());
    await owner.locator(".file-button input[type=file]").setInputFiles(PHOTO(1));
    await uploaded;
    const cover = owner.getByRole("button", { name: "Cover photo" });
    await cover.click();
    await expect(cover).toHaveAttribute("aria-pressed", "true");
  });

  await step("3. choose the memory-book template and see the preview change", async () => {
    await owner.getByRole("link", { name: "Preview and print" }).click();
    await expect(owner.getByRole("heading", { name: "Preview and print" })).toBeVisible();
    await expect(owner.locator(".mb-page")).toHaveCount(0); // the plain layout draws no memory-book page
    await owner.getByRole("radio", { name: /Memory Book/ }).click(); // not .check(): the radio is controlled and follows the save
    await expect(owner.getByText("Template saved: Memory Book.")).toBeVisible();
    await expect(owner.locator(".mb-page").first()).toBeVisible();
  });

  await step("4. create a note link and read it once", async () => {
    await owner.getByRole("link", { name: "Note links" }).click();
    await owner.getByLabel("Name (optional)").fill("Class chat");
    await owner.getByRole("button", { name: "Create link" }).click();
    await expect(owner.getByRole("heading", { name: "Your link is ready" })).toBeVisible();
    const shown = await owner.getByLabel("Link for Class chat").inputValue();
    link = new URL(shown).pathname;
    expect(link).toMatch(/^\/c\/[\w-]{20,}$/);
    await owner.getByRole("button", { name: "Done" }).click();
    await expect(owner.getByLabel("Link for Class chat")).toHaveCount(0); // shown once, then gone
  });

  const send = async (n, photo) => {
    await friend.getByLabel(/^Your name/).fill(NOTES[n].name);
    await friend.getByLabel(/^Your message/).fill(NOTES[n].message);
    if (photo) await friend.locator(".note-add-photo input[type=file]").setInputFiles(PHOTO(photo));
    if (photo) await expect(friend.getByRole("img", { name: "Your photo 1" })).toBeVisible();
    const sent = friend.waitForResponse((r) => r.url().includes("/api/public-collect/submit-note") && r.ok());
    await friend.getByRole("button", { name: "Send my note" }).click();
    await sent;
    await expect(friend.getByRole("heading", { name: `Sent! Thank you, ${NOTES[n].name}.` })).toBeVisible();
  };

  await step("5. a friend opens the link in an anonymous browser and sends a note with a photo", async () => {
    await friend.goto(link);
    await expect(friend.getByRole("heading", { name: TITLE })).toBeVisible();
    await expect(friend.getByText("By sending, you agree to the Terms of Use and the Privacy Policy.")).toBeVisible();
    await send(0, 2);
  });

  await step("6. the inbox shows one pending note and the badge; approve it", async () => {
    await owner.goto(bookUrl("/notes"));
    await expect(owner.getByText("1 waiting for you · 0 in your book")).toBeVisible();
    await expect(owner.getByRole("link", { name: /^Notes from friends\s*1$/ })).toBeVisible();
    await owner.getByRole("button", { name: `Approve ${NOTES[0].name}` }).click();
    await owner.getByRole("button", { name: /^Approved/ }).click();
    await expect(owner.locator(".inbox-card")).toHaveCount(1);
    await expect(owner.locator(".inbox-card")).toContainText(NOTES[0].name);
    await expect(owner.getByText(/All caught up · 1 notes? in your book/)).toBeVisible();
  });

  await step("7. two more notes, approve them, reorder two; the order survives a reload", async () => {
    for (const n of [1, 2]) {
      await friend.getByRole("button", { name: "Write another note" }).click();
      await send(n);
    }
    await owner.goto(bookUrl("/notes"));
    await expect(owner.getByText("2 waiting for you · 1 in your book")).toBeVisible();
    await owner.getByRole("button", { name: /^Pending/ }).click();
    for (const n of [1, 2]) {
      await owner.getByRole("button", { name: `Approve ${NOTES[n].name}` }).click();
      await expect(owner.locator(".inbox-card", { hasText: NOTES[n].name })).toHaveCount(0); // it left the Pending tab
    }
    await owner.getByRole("button", { name: /^Approved/ }).click();
    const names = () => owner.locator(".inbox-card-who strong").allInnerTexts();
    await expect.poll(names).toEqual([NOTES[0].name, NOTES[1].name, NOTES[2].name]);
    const saved = owner.waitForResponse((r) => r.url().includes("/api/note/reorder") && r.ok());
    await owner.getByRole("button", { name: `More actions for ${NOTES[2].name}'s note` }).click();
    await owner.getByRole("button", { name: "Move up" }).click();
    await expect.poll(names).toEqual([NOTES[0].name, NOTES[2].name, NOTES[1].name]);
    await saved; // the screen saves the order after a short pause
    await owner.reload();
    await owner.getByRole("button", { name: /^Approved/ }).click();
    await expect.poll(names).toEqual([NOTES[0].name, NOTES[2].name, NOTES[1].name]);
  });

  await step("8. the print screen is ready and the PDF has the right pages, size, fonts and text", async () => {
    await owner.goto(bookUrl("/book"));
    await owner.locator("html[data-book-ready]").waitFor({ timeout: 45_000 });
    await expect(owner.getByRole("button", { name: "Print or save as PDF" })).toBeEnabled();
    const pages = 3 + Math.ceil(NOTES.length / NOTES_PER_PAGE);
    expect(await owner.locator("html").getAttribute("data-book-ready")).toBe(String(pages)); // what the screen itself says
    const file = path.join(mkdtempSync(path.join(os.tmpdir(), "e2e-")), "book.pdf");
    await savePdf(owner, file);
    const pdf = inspect(file);
    expect(pdf.pages, "pages").toBe(pages);
    expect(Math.abs(pdf.sizePt[0] - 420) < 1 && Math.abs(pdf.sizePt[1] - 595) < 1, `A5 is 420 x 595 pt, got ${pdf.sizePt}`).toBe(true);
    expect(pdf.allFontsEmbedded, "every font is embedded").toBe(true);
    const all = norm(text(file));
    for (const s of [TITLE, OWNER.name, ...NOTES.flatMap((n) => [n.name, n.message])])
      expect(all, `the PDF contains "${s}"`).toContain(norm(s));
    expect(all.indexOf(norm(NOTES[0].name)) < all.indexOf(norm(NOTES[2].name)) && all.indexOf(norm(NOTES[2].name)) < all.indexOf(norm(NOTES[1].name)), "notes follow the book order").toBe(true);
    for (const [i, p] of words(file).entries()) {
      const out = p.words.filter((w) => w.x0 < -1 || w.y0 < -1 || w.x1 > p.w + 1 || w.y1 > p.h + 1);
      expect(out.map((w) => w.t), `page ${i + 1}: text outside the page`).toEqual([]);
    }
  });

  await step("9. export as text: the warning, the file, an anonymous photo link works, Disable photo links ends it", async () => {
    await owner.getByRole("button", { name: "Download text file" }).click();
    const warning = owner.getByRole("dialog", { name: "Photo links are public" });
    await expect(warning).toBeVisible();
    const [download] = await Promise.all([
      owner.waitForEvent("download"),
      warning.getByRole("button", { name: "Continue" }).click(),
    ]);
    expect(download.suggestedFilename(), "file name").toMatch(/^[a-z0-9.-]+-\d{8}\.txt$/);
    const body = norm(readFileSync(await download.path(), "utf8"));
    // `.includes` and not `expect(body).toContain`: a failure must not print the text, which holds the links
    for (const s of [TITLE, OWNER.name, ...NOTES.map((n) => n.name)])
      expect(body.includes(norm(s)), `the text file contains "${s}"`).toBe(true);
    const photo = readFileSync(await download.path(), "utf8").match(/https?:\/\/\S+\/api\/public-media\/get-content\?token=\S+/)?.[0];
    expect(photo, "the text file has a photo link").toBeTruthy();
    secrets.push(new URL(photo).searchParams.get("token"));

    const anonymous = await request.newContext(); // a fresh context: no cookies, no session
    try {
      const open = await anonymous.get(photo);
      expect(open.status(), "anonymous photo link").toBe(200);
      expect(open.headers()["content-type"], "photo link content type").toMatch(/^image\//);

      await owner.getByRole("button", { name: "Disable photo links" }).click();
      await owner.getByRole("dialog", { name: "Disable photo links?" }).getByRole("button", { name: "Disable links" }).click();
      await expect(owner.getByText("Photo links disabled")).toBeVisible();
      expect((await anonymous.get(photo)).status(), "the same photo link after Disable photo links").toBe(404);
    } finally {
      await anonymous.dispose();
    }
  });

  await step("10. revoke the link: the friend's page is a dead end after one lookup; sign out closes the owner's pages", async () => {
    await owner.goto(bookUrl("/note-links"));
    await owner.getByRole("button", { name: /^Revoke/ }).click();
    await owner.getByRole("dialog").getByRole("button", { name: "Revoke" }).click();
    await expect(owner.getByText("Revoked", { exact: true })).toBeVisible();

    let lookups = 0;
    friend.on("request", (r) => r.url().includes("/api/public-collect/get") && lookups++);
    await friend.reload();
    await expect(friend.getByRole("heading", { name: "This link doesn't work anymore" })).toBeVisible();
    await friend.waitForLoadState("networkidle");
    expect(lookups, "lookups of the dead link").toBe(1);

    await owner.getByRole("button", { name: "Sign out" }).click();
    await expect(owner.getByRole("heading", { name: "Sign in" })).toBeVisible();
    for (const p of ["/yearbooks", bookUrl(), bookUrl("/notes"), bookUrl("/book")]) {
      await owner.goto(p);
      await expect(owner.getByRole("heading", { name: "Sign in" })).toBeVisible();
      expect(new URL(owner.url()).pathname, p).toBe("/login");
    }
  });
});
