// Text boxes in the layout editor end to end (T-161, docs/e2e.md "Text boxes in the editor"): add a box, type Vietnamese through the browser's real composition
// path (CDP Input.imeSetComposition, as the ADR 0004 spike did), the 500-code-point cap at compositionend (also at 499), plain-text paste, undo and redo,
// the 41st box, the clip warning, the style controls, the 16 px rule on a touch screen, and the edit view equal to the printed view on A5, A4 and Letter.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
// Real keyboards and devices (macOS Telex, iOS Safari, Firefox, Gboard) are NOT covered here: the manual-check table in docs/editor-text.md.
// Credentials: the registered fixed email code (DEV-SHORTCUT(otp), SMEM_ENV=test only). The measured numbers print as `T161-NUMBERS {...}` lines.
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 90_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Võ Thị Thanh Trúc", email: `e2e-text-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9` };
const TITLE = `Lớp 11B2 – Chữ ${RUN}`;
const PAPERS = ["A5", "A4", "Letter"];
const A5_W = 816; // design px
const A5_H = 1158;

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
const numbers = {};
test.afterAll(() => {
  for (const [k, v] of Object.entries(numbers)) console.log(`T161-NUMBERS ${JSON.stringify({ [k]: v })}`);
});

async function ownerPage(browser, viewport, extra = {}) {
  const ctx = await browser.newContext({ locale: "en-US", baseURL: BASE, viewport, ...extra });
  const page = await ctx.newPage();
  const login = await page.request.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password }, headers: { Origin: BASE } });
  expect(login.ok(), `login ${login.status()}`).toBe(true);
  return page;
}
const post = (page, path, data) => page.request.post(path, { data, headers: { Origin: BASE } });
async function saveLayout(page, items, paper) {
  if (paper) expect((await post(page, "/api/yearbook/update", { id: bookId, page_size: paper })).ok()).toBe(true);
  const got = await (await page.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json();
  const res = await post(page, "/api/book-layout/save", { yearbook_id: bookId, base_version: got.data.version, layout: { v: 1, pages: items.length ? { cover: { items } } : {} } });
  expect(res.ok(), `layout save ${res.status()}: ${await res.text()}`).toBe(true);
}
const storedItems = async (page) => {
  const got = await (await page.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json();
  return got.data.layout?.pages?.cover?.items ?? [];
};
async function openEditor(page) {
  await page.goto(`/yearbooks/${bookId}/editor`);
  await expect(page.getByRole("toolbar", { name: "Editor toolbar" })).toBeVisible();
  await expect(page.locator(".ed-pagebox .bk-sheet[data-page='cover']")).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
}
async function setZoom(page, pct) {
  const cur = () => page.locator(".ed-pagebox").evaluate((el) => Number(el.dataset.zoom));
  for (let i = 0; i < 8 && (await cur()) !== pct; i++) {
    await page.getByRole("button", { name: (await cur()) < pct ? "Zoom in" : "Zoom out" }).click();
  }
  expect(await cur()).toBe(pct);
}
/** A box in design px of the A5 page, as the stored units. */
const item = (id, left, top, w, h, text, extra = {}) => ({
  id,
  t: "text",
  text,
  x: Math.round((left * 1000) / A5_W),
  w: Math.round((w * 1000) / A5_W),
  h: Math.round((h * 1000) / A5_W),
  cy: Math.round(((top + h / 2) * 1000) / A5_H),
  r: 0,
  font: "body",
  size: 30,
  color: "ink",
  align: "left",
  bold: false,
  ...extra,
});
const field = (page) => page.getByRole("textbox", { name: "Text of the box", exact: true });
const panelField = (page) => page.getByRole("textbox", { name: "Text of the box (edit here)" });
const printed = (page, id) => page.locator(`.ed-pagebox [data-layout-item='${id}'] .bk-fit`);
const runes = (s) => [...s].length;

/** Types a word the way an IME does: the composition grows letter by letter (Telex "Vieejt" gives V Vi Viê Việ Việt), then it is committed. */
async function compose(cdp, steps, commit = steps.at(-1)) {
  for (const s of steps) await cdp.send("Input.imeSetComposition", { text: s, selectionStart: s.length, selectionEnd: s.length });
  await cdp.send("Input.insertText", { text: commit });
}
async function paste(page, plain, html) {
  await field(page).evaluate(
    (el, [p, h]) => {
      const dt = new DataTransfer();
      if (p !== null) dt.setData("text/plain", p);
      if (h !== null) dt.setData("text/html", h);
      el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: dt, bubbles: true, cancelable: true }));
    },
    [plain, html],
  );
}

const SHORT = "Việt Nam";
const MEDIUM = "Tôi yêu những buổi chiều tan học, cùng các bạn đi dưới hàng phượng đỏ rực cả một góc trời.";
const LONG = "Mười hai năm đèn sách, những ngày mưa đường trơn trượt, những buổi trưa hè ve kêu râm ran, tiếng trống trường giòn giã mỗi sáng, cô giáo chủ nhiệm cười hiền, bạn bè chia nhau từng cái kẹo, từng trang vở, từng nỗi lo thi cử. ".repeat(2);
const FULL = [..."Việt Nam đường về nhà 😀 ".repeat(40)].slice(0, 500).join("");
const R499 = [...FULL].slice(0, 499).join("");

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

test("add a box, type Vietnamese through the real composition path, undo and redo", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1500, height: 1000 });
  await saveLayout(page, []);
  await openEditor(page);
  const cdp = await page.context().newCDPSession(page);
  await step("Add text: a box in the default style, selected, open, its text selected", async () => {
    await page.getByRole("toolbar").getByRole("button", { name: "Add text" }).click();
    await expect(field(page)).toBeFocused();
    await expect(field(page)).toHaveValue("Your text");
    const sel = await field(page).evaluate((el) => [el.selectionStart, el.selectionEnd]);
    expect(sel).toEqual([0, 9]);
    await expect(page.getByLabel("Size", { exact: true })).toHaveValue("52");
    await expect(page.getByLabel("Font")).toHaveValue("body");
  });
  await step("key by key: Việt, Nam, đường (3 compositions) lose no letter", async () => {
    await page.evaluate(() => {
      const ta = document.querySelector("textarea.ed-textarea");
      window.__ime = [];
      for (const t of ["compositionstart", "compositionupdate", "compositionend"]) ta.addEventListener(t, () => window.__ime.push(t));
    });
    await compose(cdp, ["V", "Vi", "Viê", "Việ", "Việt"]);
    await page.keyboard.type(" ");
    await compose(cdp, ["N", "Na", "Nam"]);
    await page.keyboard.type(" ");
    await compose(cdp, ["đ", "đu", "đư", "đươ", "đươn", "đương", "đường"]);
    await expect(field(page)).toHaveValue("Việt Nam đường");
    const ev = await page.evaluate(() => window.__ime);
    numbers.ime = { value: "Việt Nam đường", compositionstart: ev.filter((e) => e === "compositionstart").length, compositionupdate: ev.filter((e) => e === "compositionupdate").length, compositionend: ev.filter((e) => e === "compositionend").length };
    expect(numbers.ime.compositionstart).toBe(3);
    await expect(printed(page, (await storedOrDrawnId(page)))).toHaveText("Việt Nam đường");
  });
  await step("Escape keeps the text; undo is one step for the whole typing; redo brings it back; it is saved", async () => {
    await page.keyboard.press("Escape");
    await expect(field(page)).toHaveCount(0);
    const id = await storedOrDrawnId(page);
    await expect(printed(page, id)).toHaveText("Việt Nam đường");
    await page.getByRole("button", { name: "Undo" }).click();
    await expect(printed(page, id)).toHaveText("Your text");
    await page.getByRole("button", { name: "Redo" }).click();
    await expect(printed(page, id)).toHaveText("Việt Nam đường");
    await expect.poll(async () => (await storedItems(page))[0]?.text, { timeout: 15_000 }).toBe("Việt Nam đường");
    const saved = (await storedItems(page))[0];
    expect(saved).toMatchObject({ t: "text", size: 52, x: 200, w: 600, cy: 500 });
    await page.reload();
    await expect(printed(page, saved.id)).toHaveText("Việt Nam đường");
  });
});

/** The id of the only item on the cover, as drawn on the stage. */
const storedOrDrawnId = (page) => page.locator(".ed-pagebox [data-layout-item]").first().getAttribute("data-layout-item");

test("composition at 499 code points finishes the word, then is cut to 500 at compositionend", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1500, height: 1000 });
  await saveLayout(page, [item("cap499item1", 60, 300, 700, 400, R499)]);
  await openEditor(page);
  const cdp = await page.context().newCDPSession(page);
  await page.getByRole("button", { name: /^Text: / }).click();
  await page.keyboard.press("Enter");
  await expect(field(page)).toBeFocused();
  expect(runes(await field(page).inputValue())).toBe(499);
  for (const s of ["đ", "đư", "đườ"]) await cdp.send("Input.imeSetComposition", { text: s, selectionStart: s.length, selectionEnd: s.length });
  const during = runes(await field(page).inputValue());
  await cdp.send("Input.insertText", { text: "đườ" });
  await expect.poll(async () => runes(await field(page).inputValue())).toBe(500);
  const after = await field(page).inputValue();
  numbers.cap499 = { during_composition: during, after_compositionend: runes(after) };
  expect(during).toBe(502);
  expect(after.endsWith("đ")).toBe(true);
  await page.keyboard.press("Escape");
  await expect.poll(async () => runes((await storedItems(page))[0]?.text ?? ""), { timeout: 15_000 }).toBe(500);
});

test("paste is plain text, 600 emoji give exactly 500 code points, and Ctrl/Cmd+Z in the field is the field's own undo", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1500, height: 1000 });
  await saveLayout(page, [item("pasteitem01", 60, 300, 700, 400, "x")]);
  await openEditor(page);
  await page.getByRole("button", { name: /^Text: / }).click();
  await page.keyboard.press("Enter");
  await field(page).selectText();
  await step("HTML on the clipboard inserts no markup", async () => {
    await paste(page, "bold text\r\nsecond\tline", '<b style="font-size:99px">bold</b> <a href="https://example.com">link</a>');
    await expect(field(page)).toHaveValue("bold text\nsecond line");
    await expect(page.locator(".ed-pagebox [data-layout-item='pasteitem01'] b, .ed-pagebox [data-layout-item='pasteitem01'] a")).toHaveCount(0);
    expect(await printed(page, "pasteitem01").innerHTML()).not.toContain("<");
    await paste(page, null, "<i>only html</i>");
    await expect(field(page)).toHaveValue("bold text\nsecond line"); // nothing plain on the clipboard: nothing inserted
  });
  await step("600 pasted emoji: exactly 500 code points", async () => {
    await field(page).selectText();
    await paste(page, "😀".repeat(600), null);
    await expect.poll(async () => runes(await field(page).inputValue())).toBe(500);
    numbers.paste = { pasted_code_points: 600, kept: runes(await field(page).inputValue()), utf16_units_kept: (await field(page).inputValue()).length };
  });
  await step("Ctrl/Cmd+Z inside the field undoes the typing in the field only", async () => {
    await field(page).selectText();
    await page.keyboard.type("hello");
    await expect(field(page)).toHaveValue("hello");
    const before = await storedItems(page);
    await page.keyboard.press("ControlOrMeta+z");
    await expect(field(page)).not.toHaveValue("hello");
    await expect(field(page)).toBeFocused();
    await expect(page.locator(".ed-pagebox [data-layout-item='pasteitem01']")).toHaveCount(1);
    expect(before.length).toBe(1);
  });
  await step("an Enter inside the field is a line break, not a shortcut", async () => {
    await field(page).selectText();
    await page.keyboard.type("a");
    await page.keyboard.press("Enter");
    await page.keyboard.type("b");
    await expect(field(page)).toHaveValue("a\nb");
  });
});

test("the 41st box on a page is refused with a message, nothing is added", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1500, height: 1000 });
  await saveLayout(page, Array.from({ length: 40 }, (_, i) => item(`fill${String(i).padStart(4, "0")}`, 20 + (i % 8) * 90, 40 + Math.floor(i / 8) * 150, 80, 60, `n${i}`, { size: 12 })));
  await openEditor(page);
  await page.getByRole("toolbar").getByRole("button", { name: "Add text" }).click();
  await expect(page.getByRole("alert")).toContainText("This page already has 40 items");
  await expect(page.locator(".ed-pagebox [data-layout-item]")).toHaveCount(40);
  await expect(field(page)).toHaveCount(0);
});

test("the clip warning appears for text that does not fit at 50 % and goes away when the text is shorter or the box larger", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1500, height: 1000 });
  await saveLayout(page, [item("clipitem001", 60, 300, 220, 70, R499)]);
  await openEditor(page);
  const stageWarn = page.locator(".ed-stage .ed-warn");
  const panelWarn = page.getByTestId("text-cut-panel");
  await step("selected: the warning shows beside the stage and in the panel", async () => {
    await page.getByRole("button", { name: /^Text: / }).click();
    await expect(printed(page, "clipitem001")).toHaveAttribute("data-fit", "overflow");
    await expect(stageWarn).toContainText("Text is cut");
    await expect(panelWarn).toContainText("Text is cut");
    await page.keyboard.press("Enter");
    await expect(field(page)).toBeFocused();
    await expect(stageWarn).toBeVisible(); // still shown while the field is open
    const css = await field(page).evaluate((el) => ({ overflow: getComputedStyle(el).overflow, caret: getComputedStyle(el).caretColor }));
    expect(css.overflow).toBe("hidden");
    expect(css.caret).not.toBe("rgba(0, 0, 0, 0)");
    await page.keyboard.press("Escape");
  });
  await step("a larger box removes it", async () => {
    await page.getByRole("spinbutton", { name: "Width" }).fill("90");
    await page.getByRole("spinbutton", { name: "Width" }).press("Enter");
    await page.getByRole("spinbutton", { name: "Height" }).fill("40");
    await page.getByRole("spinbutton", { name: "Height" }).press("Enter");
    await expect(printed(page, "clipitem001")).not.toHaveAttribute("data-fit", "overflow");
    await expect(stageWarn).toHaveCount(0);
    await expect(panelWarn).toHaveCount(0);
    await page.getByRole("button", { name: "Undo" }).click();
    await page.getByRole("button", { name: "Undo" }).click();
    await expect(stageWarn).toContainText("Text is cut"); // back to the small box
  });
  await step("shorter text removes it", async () => {
    await panelField(page).fill("Ngắn thôi");
    await expect(stageWarn).toHaveCount(0);
    await expect(printed(page, "clipitem001")).toHaveText("Ngắn thôi");
  });
});

test("the style controls reach the page and are saved; a size outside the manifest range is refused", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1500, height: 1000 });
  await saveLayout(page, [item("styleitem01", 60, 300, 600, 200, "Kiểu chữ")]);
  await openEditor(page);
  await page.getByRole("button", { name: /^Text: / }).click();
  await page.getByLabel("Font").selectOption("display");
  await page.getByRole("radio", { name: "Accent" }).click();
  await page.getByRole("button", { name: "Centre" }).click();
  await page.getByRole("checkbox", { name: "Bold" }).click();
  await expect(page.getByRole("checkbox", { name: "Bold" })).toBeChecked();
  const size = page.getByLabel("Size", { exact: true });
  await size.fill("97");
  await size.press("Enter");
  await expect(page.getByRole("alert").filter({ hasText: "Between 8 and 96" })).toBeVisible();
  await size.fill("40");
  await size.press("Enter");
  const css = await printed(page, "styleitem01").evaluate((el) => ({ size: el.style.fontSize, align: getComputedStyle(el).textAlign, weight: getComputedStyle(el).fontWeight }));
  expect(css).toEqual({ size: "40px", align: "center", weight: "700" });
  await expect.poll(async () => (await storedItems(page))[0], { timeout: 15_000 }).toMatchObject({ font: "display", color: "accent", align: "center", bold: true, size: 40 });
  await page.reload();
  await page.getByRole("button", { name: /^Text: / }).click();
  await expect(page.getByLabel("Font")).toHaveValue("display");
  await expect(page.getByRole("radio", { name: "Accent" })).toHaveAttribute("aria-checked", "true");
});

test("a text with only white space is removed when edit mode closes, and one undo brings it back", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1500, height: 1000 });
  await saveLayout(page, []);
  await openEditor(page);
  await page.getByRole("toolbar").getByRole("button", { name: "Add text" }).click();
  await page.keyboard.type("   ");
  await expect(page.locator(".ed-pagebox [data-layout-item]")).toHaveCount(1);
  await page.keyboard.press("Escape");
  await expect(page.locator(".ed-pagebox [data-layout-item]")).toHaveCount(0);
  await page.getByRole("button", { name: "Undo" }).click();
  await expect(page.locator(".ed-pagebox [data-layout-item]")).toHaveCount(1);
});

test("on a touch screen the field is never below 16 px: a smaller text is scaled with a transform", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 900, height: 1300 }, { hasTouch: true, isMobile: true });
  await saveLayout(page, [item("touchitem01", 60, 300, 600, 200, "Chữ nhỏ trên điện thoại", { size: 10 })]);
  await openEditor(page);
  expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
  await page.getByRole("button", { name: /^Text: / }).tap();
  await page.getByRole("button", { name: /^Text: / }).tap(); // a second tap on the selected text opens the field
  await expect(field(page)).toBeFocused();
  const css = await field(page).evaluate((el) => {
    const s = getComputedStyle(el);
    return { font: parseFloat(s.fontSize), transform: s.transform, screenFont: parseFloat(s.fontSize) * new DOMMatrix(s.transform).a * (el.closest(".bk-sheet").getBoundingClientRect().width / 816) };
  });
  numbers.touch = css;
  expect(css.font).toBeGreaterThanOrEqual(16);
  expect(css.transform).not.toBe("none");
  await page.keyboard.press("Escape");
});

test("the edit view equals the printed view: short, medium, long, 500 runes and clipped text, rotated 0 and -4 degrees, on A5, A4 and Letter", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { width: 1500, height: 1500 });
  const cases = [
    ["short", item("parityShort", 48, 40, 440, 80, SHORT)],
    ["medium", item("parityMedium", 48, 160, 440, 150, MEDIUM)],
    ["long", item("parityLong", 48, 340, 440, 220, LONG)],
    ["500 runes", item("parityFull", 48, 600, 440, 200, FULL)],
    ["clipped", item("parityClip", 48, 850, 220, 70, R499)],
  ];
  // Compares two PNGs in the browser (no image library is needed): pixels that differ at all, and by more than 48 of 255 in some channel.
  const diff = (a, b) =>
    page.evaluate(
      async ([x, y]) => {
        const load = (b64) =>
          new Promise((res) => {
            const im = new Image();
            im.onload = () => res(im);
            im.src = `data:image/png;base64,${b64}`;
          });
        const [ia, ib] = await Promise.all([load(x), load(y)]);
        if (ia.width !== ib.width || ia.height !== ib.height) return { total: -1 };
        const px = (im) => {
          const c = document.createElement("canvas");
          c.width = im.width;
          c.height = im.height;
          const g = c.getContext("2d");
          g.drawImage(im, 0, 0);
          return g.getImageData(0, 0, im.width, im.height).data;
        };
        const da = px(ia);
        const db = px(ib);
        let any = 0;
        let big = 0;
        for (let i = 0; i < da.length; i += 4) {
          const d = Math.max(Math.abs(da[i] - db[i]), Math.abs(da[i + 1] - db[i + 1]), Math.abs(da[i + 2] - db[i + 2]));
          if (d > 0) any++;
          if (d > 48) big++;
        }
        return { total: ia.width * ia.height, any, big };
      },
      [a.toString("base64"), b.toString("base64")],
    );
  const rows = [];
  for (const paper of PAPERS) {
    for (const r of [0, -4]) {
      await saveLayout(page, cases.map(([, it]) => ({ ...it, r })), paper);
      await openEditor(page);
      await setZoom(page, 100);
      // no selection chrome, no caret in the pictures; the text on the page is all that differs between the two
      await page.addStyleTag({ content: ".ed-overlay { opacity: 0 !important } .ed-textarea { caret-color: transparent !important } .ed-scroll { scroll-behavior: auto !important }" });
      for (const [name, it] of cases) {
        const hit = page.locator(`[data-target='${it.id}'] .ed-hit`);
        const wrap = page.locator(`.ed-pagebox [data-layout-item='${it.id}']`);
        await wrap.scrollIntoViewIfNeeded();
        const clip = await wrap.evaluate((el) => {
          const b = el.getBoundingClientRect();
          return { x: Math.floor(b.left) - 2, y: Math.floor(b.top) - 2, width: Math.ceil(b.width) + 4, height: Math.ceil(b.height) + 4 };
        });
        await hit.focus();
        await page.keyboard.press("Enter");
        await expect(field(page)).toBeFocused();
        const fit = await printed(page, it.id).evaluate((el) => ({ state: el.dataset.fit, px: parseFloat(el.style.fontSize) }));
        const editPx = await field(page).evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
        await page.evaluate(() => new Promise((res) => requestAnimationFrame(() => requestAnimationFrame(res))));
        const edit = await page.screenshot({ clip });
        await page.keyboard.press("Escape");
        await expect(field(page)).toHaveCount(0);
        await page.evaluate(() => new Promise((res) => requestAnimationFrame(() => requestAnimationFrame(res))));
        const print = await page.screenshot({ clip });
        const d = await diff(edit, print);
        expect(d.total, "same picture size").toBeGreaterThan(0);
        rows.push({ paper, rotation: r, text: name, fit: fit.state, print_px: fit.px, edit_px: editPx, changed_px: d.any, over48_px: d.big, total_px: d.total, pct: Math.round((d.any / d.total) * 10000) / 100 });
      }
    }
  }
  numbers.parity = rows;
  const worst = Math.max(...rows.map((r) => r.pct));
  numbers.parity_summary = { cases: rows.length, worst_pct: worst, zero_cases: rows.filter((r) => r.changed_px === 0).length, over_0_2_pct: rows.filter((r) => r.pct > 0.2).length, max_over48_px: Math.max(...rows.map((r) => r.over48_px)) };
  for (const r of rows) {
    expect(r.edit_px, `${r.paper} ${r.rotation} ${r.text}: the field takes the size the fit rule chose`).toBeCloseTo(r.print_px, 1);
    // glyph anti-aliasing only: no pixel differs by more than 48 of 255 in any channel, and at most 0.6 % of the box differs at all (about half of the cases are exactly 0)
    expect(r.over48_px, `${r.paper} ${r.rotation} ${r.text}: pixels that differ by more than 48/255`).toBe(0);
    expect(r.pct, `${r.paper} ${r.rotation} ${r.text}: changed pixels, in percent of the box`).toBeLessThanOrEqual(0.6);
  }
  expect(rows.find((r) => r.text === "clipped").fit).toBe("overflow");
  expect(rows.find((r) => r.text === "500 runes").fit).not.toBe("overflow");
});
