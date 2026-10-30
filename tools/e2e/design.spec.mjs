// The customisation journey end to end (T-147, docs/e2e.md "The customisation journey"): a student with a Memory Book and one approved friend note opens the
// customise screen, picks a theme and a font set, hides the profile page and replaces a cover text, saves, reloads, and prints; resets to the template; and
// switches to Navy Classic, which resets the design while the note survives.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
// Every step is a test.step with a hard limit; no fixed sleeps. Credentials: the registered fixed email code (DEV-SHORTCUT(otp), SMEM_ENV=test only).
// The note-link token is a bearer secret: only ever in `link`, scrubbed from every error leaving a step.
import { expect, request, test } from "@playwright/test";
import { mkdtempSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { inspect, savePdf, text } from "../print-check/lib.mjs";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 60_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Võ Thị Mai Anh", email: `e2e-design-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-7` };
const TITLE = `Lớp 11B2 – Lưu bút ${RUN}`;
const NOTE = { name: `Bạn Thân ${RUN}`, message: `Nhớ giữ liên lạc nhé ${RUN}!` };
const QUOTE = `Điều tuyệt nhất là có bạn ${RUN}`; // printed on the profile page only
const BELONGS = `Của riêng ${RUN}`.slice(0, 40); // cover.belongs allows 40 characters
// The memory-book book has four sheets (cover, profile, one note, back); the hidden profile leaves three.
const PAGES = { all: 4, noProfile: 3 };

let link = "";

async function step(title, body) {
  try {
    return await test.step(title, body, { timeout: STEP_MS });
  } catch (e) {
    if (!(e instanceof Error)) throw e;
    const mark = `STEP FAILED: ${title}\n`;
    const token = link.split("/").pop();
    const scrub = (s) => (token ? s.replaceAll(token, "<token>") : s);
    e.stack = e.stack ? mark + scrub(e.stack) : e.stack;
    e.message = mark + scrub(e.message);
    throw e;
  }
}

const norm = (s) => s.normalize("NFC").replace(/\s+/g, " ").trim().toLocaleLowerCase("vi");

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

test("customisation journey: pick a look, hide a page, save, reload, print, reset, change the template", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const owner = await (await browser.newContext({ locale: "en-US", baseURL: BASE })).newPage();
  const friend = await (await browser.newContext({ locale: "en-US", baseURL: BASE })).newPage(); // anonymous
  let bookId = "";
  const bookUrl = (tail = "") => `/yearbooks/${bookId}${tail}`;
  const api = (p, data) => owner.request.post(p, { data, headers: { Origin: BASE } });

  /** The groups of the customise screen. The template picker has a "Classic" too (Navy Classic), so a choice is looked up inside its group. */
  const group = (name) => owner.getByRole("group", { name, exact: true });
  const openCustomise = async () => {
    await owner.goto(bookUrl("/customise"));
    await expect(owner.getByRole("heading", { name: "Customise the design" })).toBeVisible();
    await expect(group("Colours")).toBeVisible();
  };
  /** The print route: waits for the ready signal and reads what the browser prints. */
  const printed = async () => {
    await owner.goto(bookUrl("/book"));
    await owner.locator("html[data-book-ready]").waitFor({ timeout: 45_000 });
    return owner.evaluate(() => {
      const sheets = [...document.querySelectorAll("section.bk-sheet")];
      const page = document.querySelector(".mb-page, .nc-page");
      return {
        ready: document.documentElement.dataset.bookReady,
        kinds: sheets.map((s) => s.dataset.page),
        themes: [...new Set([...document.querySelectorAll("[data-theme]")].map((e) => `${e.dataset.theme}/${e.dataset.fonts}`))],
        ink: page ? getComputedStyle(page).getPropertyValue("--mb-ink").trim() : "",
        cover: sheets.find((s) => s.dataset.page === "cover")?.textContent ?? "",
      };
    });
  };
  const pdfOf = async () => {
    const file = path.join(mkdtempSync(path.join(os.tmpdir(), "e2e-design-")), "book.pdf");
    await savePdf(owner, file);
    return { info: inspect(file), all: norm(text(file)) };
  };

  await step("1. register, create the yearbook and choose Memory Book", async () => {
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
    await owner.getByLabel("6-digit code").fill(OTP); // the sixth digit submits
    await expect(owner.getByText("Your email address is verified.")).toBeVisible();
    await owner.getByRole("link", { name: "Continue" }).click();
    await owner.goto("/yearbooks/new");
    await owner.getByLabel("Title").fill(TITLE);
    await owner.getByLabel("Yearbook language").selectOption("vi");
    await owner.getByLabel("Page size").selectOption("A5");
    await owner.getByRole("button", { name: "Create yearbook" }).click();
    await expect(owner.getByRole("heading", { level: 1, name: TITLE })).toBeVisible();
    bookId = new URL(owner.url()).pathname.split("/").pop();
    await owner.getByLabel("Full name").fill(OWNER.name);
    await owner.getByLabel("Favourite quote").fill(QUOTE);
    const profile = owner.waitForResponse((r) => r.url().includes("/api/yearbook/update-profile") && r.ok());
    await owner.getByRole("button", { name: "Save profile" }).click();
    await profile;
    const res = await api("/api/yearbook/update", { id: bookId, template_id: "memory-book" });
    expect(res.ok(), `template change ${res.status()}`).toBe(true);
  });

  await step("2. a friend sends a note and the student approves it", async () => {
    await owner.goto(bookUrl("/note-links"));
    await owner.getByLabel("Name (optional)").fill("Class chat");
    await owner.getByRole("button", { name: "Create link" }).click();
    await expect(owner.getByRole("heading", { name: "Your link is ready" })).toBeVisible();
    link = new URL(await owner.getByLabel("Link for Class chat").inputValue()).pathname;
    await owner.getByRole("button", { name: "Done" }).click();
    await friend.goto(link);
    await friend.getByLabel(/^Your name/).fill(NOTE.name);
    await friend.getByLabel(/^Your message/).fill(NOTE.message);
    const sent = friend.waitForResponse((r) => r.url().includes("/api/public-collect/submit-note") && r.ok());
    await friend.getByRole("button", { name: "Send my note" }).click();
    await sent;
    await owner.goto(bookUrl("/notes"));
    await expect(owner.getByText("1 waiting for you · 0 in your book")).toBeVisible(); // the list has loaded
    await owner.getByRole("button", { name: `Approve ${NOTE.name}` }).click();
    await owner.getByRole("button", { name: /^Approved/ }).click();
    await expect(owner.locator(".inbox-card")).toHaveCount(1);
    await expect(owner.locator(".inbox-card")).toContainText(NOTE.name);
  });

  await step("3. a book with no design prints the template's look (lavender, playful, all four pages)", async () => {
    const p = await printed();
    expect(norm(await owner.locator('section[data-page="profile"]').textContent()), "the quote is on the profile page").toContain(norm(QUOTE));
    expect(p.kinds).toEqual(["cover", "profile", "notes", "back"]);
    expect(p.themes).toEqual(["lavender/playful"]);
    expect(p.ink).toBe("#3b2f4a");
  });

  await step("4. customise: peach, the classic font set, the profile page hidden, a cover text; save", async () => {
    await openCustomise();
    await expect(group("Colours").getByRole("radio", { name: /Lavender/ })).toBeChecked(); // the default
    await group("Colours").getByRole("radio", { name: /Peach/ }).check();
    await group("Fonts").getByRole("radio", { name: /Classic/ }).check();
    await owner.getByRole("switch", { name: "Print “All about me”" }).uncheck();
    await owner.getByLabel("Cover: “this book belongs to”").fill(BELONGS);
    await expect(owner.getByText("You have changes that are not saved.")).toBeVisible();
    const saved = owner.waitForResponse((r) => r.url().includes("/api/book-design/update") && r.ok());
    await owner.getByRole("button", { name: "Save design" }).click();
    await saved;
    await expect(owner.getByText("Design saved.")).toBeVisible();
  });

  await step("5. after a reload the screen shows the same choices", async () => {
    await openCustomise();
    await expect(group("Colours").getByRole("radio", { name: /Peach/ })).toBeChecked();
    await expect(group("Fonts").getByRole("radio", { name: /Classic/ })).toBeChecked();
    await expect(owner.getByRole("switch", { name: "Print “All about me”" })).not.toBeChecked();
    await expect(owner.getByLabel("Cover: “this book belongs to”")).toHaveValue(BELONGS);
    await expect(owner.getByRole("button", { name: "Save design" })).toBeDisabled(); // nothing unsaved
  });

  await step("6. the print route applies the design: three pages, no profile, the cover text, the peach colours, the PDF agrees", async () => {
    const p = await printed();
    expect(p.ready, "the book's own page count").toBe(String(PAGES.noProfile));
    expect(p.kinds, "the hidden section is absent").toEqual(["cover", "notes", "back"]);
    expect(p.themes).toEqual(["peach/classic"]);
    expect(p.ink, "the theme's custom property is applied").toBe("#4a2c22");
    expect(norm(p.cover)).toContain(norm(TITLE));
    expect(norm(p.cover)).toContain(norm(BELONGS));
    const pdf = await pdfOf();
    expect(pdf.info.pages, "PDF pages").toBe(PAGES.noProfile);
    expect(pdf.info.allFontsEmbedded, "every font is embedded").toBe(true);
    for (const s of [TITLE, BELONGS, NOTE.name, NOTE.message]) expect(pdf.all, `the PDF contains "${s}"`).toContain(norm(s));
    expect(pdf.all, "the hidden profile page is not printed").not.toContain(norm(QUOTE));
  });

  await step("7. reset to the template: the default look and all four pages come back", async () => {
    await openCustomise();
    await owner.getByRole("button", { name: "Reset to template" }).click();
    await owner.getByRole("dialog", { name: "Reset the design?" }).getByRole("button", { name: "Reset the design" }).click();
    await expect(owner.getByText("The design is back to the template’s look.")).toBeVisible();
    await expect(group("Colours").getByRole("radio", { name: /Lavender/ })).toBeChecked();
    const p = await printed();
    expect(p.kinds).toEqual(["cover", "profile", "notes", "back"]);
    expect(p.themes).toEqual(["lavender/playful"]);
    expect(p.ink).toBe("#3b2f4a");
    expect(norm(p.cover)).not.toContain(norm(BELONGS));
    const pdf = await pdfOf();
    expect(pdf.info.pages).toBe(PAGES.all);
    expect(pdf.all, "the profile page is printed again").toContain(norm(QUOTE));
  });

  await step("8. switching to Navy Classic resets a saved design; the note and the book stay", async () => {
    await openCustomise();
    await group("Colours").getByRole("radio", { name: /Peach/ }).check();
    const saved = owner.waitForResponse((r) => r.url().includes("/api/book-design/update") && r.ok());
    await owner.getByRole("button", { name: "Save design" }).click();
    await saved;
    const switched = owner.waitForResponse((r) => r.url().includes("/api/yearbook/update") && r.ok());
    await owner.getByRole("radio", { name: /Navy Classic/ }).click(); // not .check(): the radio is controlled and follows the save
    await switched;
    await expect(owner.getByText("You changed the template, so the earlier design was reset.")).toBeVisible();
    await expect(group("Colours").getByRole("radio", { name: /Navy and gold/ })).toBeChecked();
    const stored = (await (await owner.request.get(`/api/book-design/get?yearbook_id=${bookId}`)).json()).data;
    expect(stored.options, "the design row is gone").toBeNull();
    expect(stored.template_id).toBe("navy-classic");
    const p = await printed();
    expect(p.themes).toEqual(["navy/classic"]);
    expect(p.kinds).toEqual(["cover", "profile", "notes", "back"]);
    expect(norm(await owner.locator(".bk-stage").textContent())).toContain(norm(NOTE.message)); // the note survived the change
  });
});
