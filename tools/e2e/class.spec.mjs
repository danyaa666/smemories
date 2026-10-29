// The class yearbook journey end to end (T-131, docs/e2e.md "The class journey"): an admin makes a class and an invite, two students join through the
// link and build their pages, each collects friend notes and approves one, the admin assembles and orders the book, adds an intro page and opens the
// print route. The privacy half: student A must not see student B's notes anywhere, the admin must never get a pending note.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
// Every step is a test.step with a hard limit; no fixed sleeps. Credentials: the registered fixed email code (DEV-SHORTCUT(otp), SMEM_ENV=test only).
// The invite token and the note-link tokens are bearer secrets: only ever in `secrets`, scrubbed from every error leaving a step.
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 90_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const user = (name, tag) => ({ name, email: `e2e-class-${tag}-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-${tag}9` });
const ADMIN = user("Cô Nguyễn Thu Hà", "admin");
const A = user("Trần Quốc Anh", "a");
const B = user("Lê Thị Bích Ngọc", "b");
const CLASS_NAME = `Lớp 12A1 ${RUN}`;
const INTRO = { title: `Lời chào ${RUN}`, body: `Chúc cả lớp ${RUN} một tương lai rực rỡ.` };
// One approved and one pending note per student. The markers are unique, so a leak is found by a plain text search.
const notes = (s) => ({
  approved: { name: `Bạn Duyệt ${s} ${RUN}`, message: `Lời nhắn đã duyệt cho ${s} ${RUN}` },
  pending: { name: `Bạn Chờ ${s} ${RUN}`, message: `Lời nhắn CHƯA duyệt cho ${s} ${RUN}` },
});
const NOTE = { a: notes("A"), b: notes("B") };

const secrets = []; // invite and note-link tokens
const created = { classId: "" };

async function step(title, body) {
  try {
    return await test.step(title, body, { timeout: STEP_MS });
  } catch (e) {
    if (!(e instanceof Error)) throw e;
    const mark = `STEP FAILED: ${title}\n`;
    const scrub = (s) => secrets.filter(Boolean).reduce((out, token) => out.replaceAll(token, "<token>"), s);
    e.stack = e.stack ? mark + scrub(e.stack) : e.stack;
    e.message = mark + scrub(e.message);
    throw e;
  }
}

/** Deletes what the run created (the accounts stay). Never fails the test. */
async function cleanUp() {
  if (!BASE) return;
  for (const [u, deleteClass] of [[ADMIN, true], [A, false], [B, false]]) {
    const api = await request.newContext({ baseURL: BASE, extraHTTPHeaders: { Origin: BASE } });
    try {
      const login = await api.post("/api/auth/login", { data: { email: u.email, password: u.password } });
      if (!login.ok()) continue; // the run failed before the account existed
      if (deleteClass && created.classId) await api.post("/api/class/delete", { data: { id: created.classId } });
      const list = await (await api.get("/api/yearbook/get-list")).json();
      for (const y of list.data?.items ?? []) await api.post("/api/yearbook/delete", { data: { id: y.id } });
    } catch (e) {
      console.warn(`clean-up failed (the run's data stays in the test database): ${e.message}`);
    } finally {
      await api.dispose();
    }
  }
}

test.afterAll(cleanUp);

test("class journey: admin, invite, two students, assemble, print, and nobody sees what they must not", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const page = async () => (await (await browser.newContext({ locale: "en-US", baseURL: BASE })).newPage());
  const admin = await page();
  const a = await page();
  const b = await page();
  const friend = await page(); // anonymous: no cookies
  let invitePath = "";
  let classId = "";
  const book = { a: "", b: "" }; // yearbook ids

  /** Sign-up with the Terms box, then the fixed development code. `from` is a path the app should return to ("Continue"). */
  const signUp = async (p, u, from) => {
    if (from) {
      await p.goto(from);
      await p.locator("main").getByRole("link", { name: "Create account" }).click();
    } else {
      await p.goto("/register");
    }
    await p.getByLabel("Your name").fill(u.name);
    await p.getByLabel("Email").fill(u.email);
    await p.getByLabel("Password", { exact: true }).fill(u.password);
    await p.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
    const registered = p.waitForResponse((r) => r.url().includes("/api/auth/register") && r.request().method() === "POST");
    await p.getByRole("button", { name: "Create account" }).click();
    if ((await registered).status() === 429) {
      throw new Error("registration is rate limited (5 per hour per address): run `make reset-limits SMEM_ENV=test` (make e2e does), or point SMEM_REDIS_URL at a fresh Redis db, then run again");
    }
    await expect(p.getByRole("heading", { name: "Verify your email" })).toBeVisible();
    await p.getByLabel("6-digit code").fill(OTP); // the sixth digit submits
    await expect(p.getByText("Your email address is verified.")).toBeVisible();
    await p.getByRole("link", { name: "Continue" }).click();
  };

  await step("1. the admin signs up and creates the class", async () => {
    await signUp(admin, ADMIN);
    await admin.goto("/classes/new");
    await admin.getByLabel("Class name").fill(CLASS_NAME);
    await admin.getByLabel("School").fill("Trường THPT Chuyên Lê Hồng Phong");
    await admin.getByRole("button", { name: "Create class" }).click();
    await expect(admin.getByRole("heading", { level: 1, name: CLASS_NAME })).toBeVisible();
    classId = new URL(admin.url()).pathname.split("/").pop();
    created.classId = classId;
    await expect(admin.getByText("Your role:")).toContainText("Admin");
  });

  await step("2. the admin makes an invite link and reads it once", async () => {
    await admin.getByRole("button", { name: "Create invite link" }).click();
    const shown = await admin.getByLabel("Invite link", { exact: true }).inputValue();
    secrets.push(shown); // a secret before anything reads, parses or asserts on it
    invitePath = new URL(shown).pathname;
    secrets.push(invitePath, invitePath.split("/").pop());
    // `toBe(true)`, not `toMatch`: a failure must not print the path or the token
    expect(/^\/join\/[\w-]{43}$/.test(invitePath), "the invite link has the shape /join/<43 characters>").toBe(true);
    await admin.getByRole("button", { name: "Done, hide the link" }).click();
    await expect(admin.getByLabel("Invite link", { exact: true })).toHaveCount(0); // shown once, then gone
  });

  /** A student joins through the link (signing up on the way), creates the page and fills the profile; returns the yearbook id. */
  const join = async (p, u) => {
    await signUp(p, u, invitePath);
    await expect(p.getByRole("heading", { name: `Join ${CLASS_NAME}` })).toBeVisible();
    await p.getByRole("button", { name: "Join this class" }).click();
    await expect(p.getByRole("heading", { name: `You joined ${CLASS_NAME}` })).toBeVisible();
    await p.getByRole("button", { name: "Create my page" }).click();
    await expect(p).toHaveURL(/\/yearbooks\/[0-9A-Z]{26}$/);
    const id = new URL(p.url()).pathname.split("/").pop();
    await p.getByLabel("Full name").fill(u.name);
    await p.getByLabel("Favourite quote").fill(`Câu nói của ${u.name} ${RUN}`);
    const saved = p.waitForResponse((r) => r.url().includes("/api/yearbook/update-profile") && r.ok());
    await p.getByRole("button", { name: "Save profile" }).click();
    await saved;
    return id;
  };

  await step("3. student A joins through the link and builds a page", async () => {
    book.a = await join(a, A);
  });
  await step("4. student B joins through the link and builds a page", async () => {
    book.b = await join(b, B);
  });

  /** The student makes a note link; the friend (anonymous) sends the two notes; the student approves the first and leaves the second pending. */
  const collect = async (p, id, n) => {
    await p.goto(`/yearbooks/${id}/note-links`);
    await p.getByLabel("Name (optional)").fill("Class chat");
    await p.getByRole("button", { name: "Create link" }).click();
    await expect(p.getByRole("heading", { name: "Your link is ready" })).toBeVisible();
    const shown = await p.getByLabel("Link for Class chat").inputValue();
    secrets.push(shown);
    const link = new URL(shown).pathname;
    secrets.push(link, link.split("/").pop());
    await p.getByRole("button", { name: "Done" }).click();
    for (const note of [n.approved, n.pending]) {
      await friend.goto(link);
      await friend.getByLabel(/^Your name/).fill(note.name);
      await friend.getByLabel(/^Your message/).fill(note.message);
      const sent = friend.waitForResponse((r) => r.url().includes("/api/public-collect/submit-note") && r.ok());
      await friend.getByRole("button", { name: "Send my note" }).click();
      await sent;
      await expect(friend.getByRole("heading", { name: `Sent! Thank you, ${note.name}.` })).toBeVisible();
    }
    await p.goto(`/yearbooks/${id}/notes`);
    await expect(p.getByText("2 waiting for you · 0 in your book")).toBeVisible();
    await p.getByRole("button", { name: `Approve ${n.approved.name}` }).click();
    await expect(p.getByText("1 waiting for you · 1 in your book")).toBeVisible();
  };

  await step("5. each student collects two friend notes and approves one", async () => {
    await collect(a, book.a, NOTE.a);
    await collect(b, book.b, NOTE.b);
  });

  await step("6. the admin sees both pages in the roster, adds an intro page and puts B before A", async () => {
    await admin.goto(`/classes/${classId}`);
    for (const u of [A, B]) await expect(admin.getByText(u.name).first()).toBeVisible();
    await admin.getByRole("link", { name: "Assemble the class book" }).click();
    await expect(admin.getByRole("heading", { name: `Class book of ${CLASS_NAME}` })).toBeVisible();
    await admin.getByRole("button", { name: "Add a class page" }).click();
    await admin.getByLabel("Title").fill(INTRO.title);
    await admin.getByLabel("Text", { exact: true }).fill(INTRO.body);
    const made = admin.waitForResponse((r) => r.url().includes("/api/class-page/create") && r.ok());
    await admin.getByRole("button", { name: "Save page" }).click();
    await made;
    await expect(admin.getByText(INTRO.title).first()).toBeVisible();
    await admin.getByRole("button", { name: `Move ${B.name} up` }).click();
    const ordered = admin.waitForResponse((r) => r.url().includes("/api/class-book/update-order") && r.ok());
    await admin.getByRole("button", { name: "Save order" }).click();
    await ordered;
    await admin.reload(); // the order is the server's, not the screen's
    const students = admin.locator('ol[aria-labelledby="students"] li');
    await expect(students).toHaveCount(3); // the admin is a member too (no page)
    const rows = await students.allInnerTexts();
    expect(rows.findIndex((r) => r.includes(B.name)), "B is listed").toBeGreaterThanOrEqual(0);
    expect(rows.findIndex((r) => r.includes(B.name))).toBeLessThan(rows.findIndex((r) => r.includes(A.name)));
  });

  await step("7. the print route has the intro page and both students' pages and approved notes, in order, and no pending note", async () => {
    await admin.goto(`/classes/${classId}/book`);
    await admin.waitForFunction(() => document.documentElement.dataset.bookReady); // fonts and photos settled
    const text = await admin.locator(".bk-stage").textContent();
    for (const s of [INTRO.title, INTRO.body, A.name, B.name, NOTE.a.approved.name, NOTE.a.approved.message, NOTE.b.approved.name, NOTE.b.approved.message]) {
      expect(text, `the class book holds "${s}"`).toContain(s);
    }
    expect(text.indexOf(INTRO.title)).toBeLessThan(text.indexOf(B.name)); // class pages at the front
    expect(text.indexOf(B.name)).toBeLessThan(text.indexOf(A.name)); // the admin's order
    for (const n of [NOTE.a.pending, NOTE.b.pending]) {
      expect(text, "a pending note reached the admin's printout").not.toContain(n.name);
      expect(text, "a pending note reached the admin's printout").not.toContain(n.message);
    }
  });

  await step("8. the admin's class-book answer holds the approved notes only, and the admin still has no right over a student's book", async () => {
    const res = await admin.request.get(`/api/class-book/get?class_id=${classId}`);
    expect(res.status()).toBe(200);
    const json = JSON.stringify(await res.json());
    for (const s of [NOTE.a.approved.message, NOTE.b.approved.message]) expect(json).toContain(s);
    for (const n of [NOTE.a.pending, NOTE.b.pending]) {
      expect(json, "a pending note is in class-book/get").not.toContain(n.message);
      expect(json).not.toContain(n.name);
    }
    for (const s of secrets) expect(json, "a token is in class-book/get").not.toContain(s);
    for (const email of [A.email, B.email]) expect(json, "an email is in class-book/get").not.toContain(email);
    for (const id of [book.a, book.b]) {
      expect((await admin.request.get(`/api/yearbook/get?id=${id}`)).status()).toBe(404);
      expect((await admin.request.get(`/api/note/get-list?yearbook_id=${id}`)).status()).toBe(404);
      expect((await admin.request.get(`/api/book-data/get?yearbook_id=${id}`)).status()).toBe(404);
    }
  });

  await step("9. student A can not see student B's notes anywhere: screens, direct API calls, the class book", async () => {
    const everything = [NOTE.b.approved, NOTE.b.pending].flatMap((n) => [n.name, n.message]).concat(B.name, B.email);
    const none = async (label, text) => {
      for (const s of everything) expect(text, `${label} shows "${s}" of student B`).not.toContain(s);
    };
    // The screens: the class, the admin's screens (a student gets the not-found screen), B's own book address.
    for (const path of [`/classes/${classId}`, `/classes/${classId}/assemble`, `/classes/${classId}/book`, `/yearbooks/${book.b}`, `/yearbooks/${book.b}/notes`, `/yearbooks`]) {
      await a.goto(path);
      await a.waitForLoadState("networkidle");
      await none(`the screen ${path.replace(classId, "<class>").replace(book.b, "<B>")}`, await a.locator("body").innerText());
    }
    await expect(a.locator(".bk-stage")).toHaveCount(0); // no print route for a student
    await a.goto(`/yearbooks/${book.b}/notes`);
    await expect(a.locator(".inbox-card")).toHaveCount(0);
    // Direct API calls with A's session.
    const probes = [
      [`/api/yearbook/get?id=${book.b}`, 404],
      [`/api/note/get-list?yearbook_id=${book.b}`, 404],
      [`/api/note/get-list?yearbook_id=${book.b}&status=pending`, 404],
      [`/api/book-data/get?yearbook_id=${book.b}`, 404],
      [`/api/note-collection/get-list?yearbook_id=${book.b}`, 404],
      [`/api/class-book/get?class_id=${classId}`, 403], // the whole book is the admin's
      [`/api/class-member/get-list?class_id=${classId}`, 403], // and so is the roster
      [`/api/class-page/get-list?class_id=${classId}`, 403],
    ];
    for (const [url, status] of probes) {
      const res = await a.request.get(url);
      expect(res.status(), `GET ${url.replace(book.b, "<B>").replace(classId, "<class>")}`).toBe(status);
      await none(`the answer of ${url.split("?")[0]}`, await res.text());
    }
    // What A does have: the class (own role, own row) and own notes.
    const mine = await a.request.get(`/api/class/get?id=${classId}`);
    expect(mine.status()).toBe(200);
    expect((await mine.json()).data.my_role).toBe("student");
    await none("class/get", await mine.text());
    const own = await a.request.get(`/api/note/get-list?yearbook_id=${book.a}`);
    expect(own.status()).toBe(200);
    expect(await own.text()).toContain(NOTE.a.approved.message);
  });
});
