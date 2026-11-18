// Accessibility and i18n audit of the layout editor in a real browser (T-182, docs/editor-accessibility.md): the accessibility tree in the main states (en and vi), colour contrast,
// touch targets, 200 % zoom, reduced motion, Vietnamese text lengths at 768, 1024 and 1440 px, and the language switch in the middle of an edit.
//   make e2e        (same API and web server as journey.spec.mjs; scripts/e2e.sh)
import { readFileSync } from "node:fs";
import { expect, request, test } from "@playwright/test";

const BASE = process.env.E2E_BASE_URL;
const OTP = process.env.SMEM_DEV_FIXED_OTP; // DEV-SHORTCUT(otp): the API of this run was started with it (scripts/e2e.sh)
const STEP_MS = 120_000;
const RUN = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
const OWNER = { name: "Đặng Thị Bích Ngọc", email: `e2e-a11y-${RUN}@example.com`, password: `Mật-khẩu-${RUN}-9` };
const TITLE = `Lớp 11A2 – Kiểm tra ${RUN}`;
const VI_TEXT = "Chúc các bạn thật nhiều niềm vui";
const OUT = process.env.E2E_OUT ?? "out";

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

let bookId = "";

async function cleanUp() {
  if (!BASE) return;
  const api = await request.newContext({ baseURL: BASE, extraHTTPHeaders: { Origin: BASE } });
  try {
    const login = await api.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password } });
    if (!login.ok()) return;
    const list = await (await api.get("/api/yearbook/get-list")).json();
    for (const b of list.data?.items ?? []) await api.post("/api/yearbook/delete", { data: { id: b.id } });
  } catch (e) {
    console.warn(`clean-up failed (the run's data stays in the test database): ${e.message}`);
  } finally {
    await api.dispose();
  }
}

test.afterAll(cleanUp);
test.describe.configure({ mode: "serial" });

async function ownerPage(browser, { width = 1280, height = 900, lang = "en-US", touch = false, reducedMotion = "no-preference" } = {}) {
  const ctx = await browser.newContext({ locale: lang, baseURL: BASE, viewport: { width, height }, hasTouch: touch, isMobile: false, reducedMotion });
  const page = await ctx.newPage();
  const login = await page.request.post("/api/auth/login", { data: { email: OWNER.email, password: OWNER.password }, headers: { Origin: BASE } });
  expect(login.ok(), `login ${login.status()}`).toBe(true);
  return page;
}
async function openEditor(page, kind = "cover") {
  await page.goto(`/yearbooks/${bookId}/editor?page=${kind}`);
  await expect(page.locator(".ed")).toBeVisible();
  await expect(page.locator(`.ed-pagebox .bk-sheet[data-page='${kind}'], .ed-phone .bk-sheet[data-page='${kind}']`).first()).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
}

test("setup: register, create a Memory Book, add a text and a sticker through the API", async ({ browser }) => {
  test.skip(!BASE || !OTP, "run through `make e2e` (scripts/e2e.sh starts the API and the web server)");
  const owner = await (await browser.newContext({ locale: "en-US", baseURL: BASE, viewport: { width: 1280, height: 900 } })).newPage();
  await step("register and verify the email", async () => {
    await owner.goto("/register");
    await owner.getByLabel("Your name").fill(OWNER.name);
    await owner.getByLabel("Email").fill(OWNER.email);
    await owner.getByLabel("Password", { exact: true }).fill(OWNER.password);
    await owner.getByLabel("I agree to the Terms of Use and the Privacy Policy").check();
    const registered = owner.waitForResponse((r) => r.url().includes("/api/auth/register") && r.request().method() === "POST");
    await owner.getByRole("button", { name: "Create account" }).click();
    if ((await registered).status() === 429) throw new Error("registration is rate limited: run `make reset-limits SMEM_ENV=test` (make e2e does)");
    await owner.getByLabel("6-digit code").fill(OTP);
    await expect(owner.getByText("Your email address is verified.")).toBeVisible();
  });
  await step("create the book on Memory Book (A5, Vietnamese)", async () => {
    await owner.goto("/yearbooks/new");
    await owner.getByLabel("Title").fill(TITLE);
    await owner.getByLabel("Yearbook language").selectOption("vi");
    await owner.getByLabel("Page size").selectOption("A5");
    await owner.getByRole("button", { name: "Create yearbook" }).click();
    await expect(owner.getByRole("heading", { level: 1, name: TITLE })).toBeVisible();
    bookId = new URL(owner.url()).pathname.split("/").pop();
    const res = await owner.request.post("/api/yearbook/update", { data: { id: bookId, template_id: "memory-book" }, headers: { Origin: BASE } });
    expect(res.ok(), `template change ${res.status()}`).toBe(true);
  });
  await step("a Vietnamese text on the cover", async () => {
    await openEditor(owner);
    await owner.getByRole("toolbar").getByRole("button", { name: "Add text" }).click();
    await expect(owner.getByRole("textbox", { name: "Text of the box", exact: true })).toBeFocused();
    await owner.keyboard.press("Control+a");
    await owner.keyboard.type(VI_TEXT);
    await owner.keyboard.press("Escape");
    await expect(owner.locator(".ed-pagebox [data-layout-item] .bk-fit")).toHaveText(VI_TEXT);
    await owner.waitForResponse((r) => r.url().includes("/api/book-layout/save") && r.ok());
  });
});

/** Runs in the page: the contrast of every text of the editor chrome (the drawn page and its items are the book, not the editor) and of every bordered control. */
function contrastReport() {
  const cx = Object.assign(document.createElement("canvas"), { width: 1, height: 1 }).getContext("2d", { willReadFrequently: true });
  const rgba = (c) => {
    cx.clearRect(0, 0, 1, 1);
    cx.fillStyle = "#000";
    cx.fillStyle = c;
    cx.fillRect(0, 0, 1, 1);
    const d = cx.getImageData(0, 0, 1, 1).data;
    return [d[0], d[1], d[2], d[3] / 255];
  };
  const over = (top, bottom) => {
    const a = top[3] + bottom[3] * (1 - top[3]);
    return a === 0 ? [0, 0, 0, 0] : [0, 1, 2].map((i) => (top[i] * top[3] + bottom[i] * bottom[3] * (1 - top[3])) / a).concat(a);
  };
  const lum = ([r, g, b]) => {
    const f = (v) => ((v /= 255) <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);
    return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
  };
  const ratio = (a, b) => {
    const [x, y] = [lum(a), lum(b)].sort((m, n) => n - m);
    return (x + 0.05) / (y + 0.05);
  };
  const backdrop = (el) => {
    const chain = [];
    for (let e = el; e; e = e.parentElement) chain.unshift(e);
    let bg = [255, 255, 255, 1];
    let image = false;
    for (const e of chain) {
      const cs = getComputedStyle(e);
      if (cs.backgroundImage !== "none") image = true;
      bg = over(rgba(cs.backgroundColor), bg);
    }
    return { bg, image };
  };
  const opacityOf = (el) => {
    let o = 1;
    for (let e = el; e; e = e.parentElement) o *= Number(getComputedStyle(e).opacity);
    return o;
  };
  const out = [];
  const root = document.querySelector(".ed");
  const skip = (el) => el.closest(".ed-pagebox, .ed-thumb-view, .ed-tpl-thumb, .visually-hidden, [aria-hidden=true]");
  for (const el of root.querySelectorAll("*")) {
    if (skip(el)) continue;
    const own = [...el.childNodes].some((n) => n.nodeType === 3 && /\S/.test(n.textContent));
    const r = el.getBoundingClientRect();
    const cs = getComputedStyle(el);
    if (r.width === 0 || r.height === 0 || cs.visibility === "hidden") continue;
    const disabled = el.matches(":disabled, [aria-disabled=true]") || !!el.closest(":disabled");
    const label = `${el.tagName.toLowerCase()}.${String(el.className).split(" ")[0]} "${(el.textContent ?? "").trim().slice(0, 28)}"`;
    const { bg, image } = backdrop(el);
    if (own) {
      const fg = rgba(cs.color);
      fg[3] *= opacityOf(el);
      const size = parseFloat(cs.fontSize);
      const large = size >= 24 || (size >= 18.66 && Number(cs.fontWeight) >= 700);
      out.push({ kind: "text", label, disabled, large, image, ratio: ratio(over(fg, bg), bg), size });
    }
    if (el.matches("button, input, select, textarea") && parseFloat(cs.borderTopWidth) > 0 && !el.matches(".btn-primary, .ed-tpl-pick")) {
      const par = backdrop(el.parentElement);
      const bc = rgba(cs.borderTopColor);
      bc[3] *= opacityOf(el);
      const fill = over(rgba(cs.backgroundColor), par.bg); // a solid button is bounded by its fill as much as by its border
      out.push({ kind: "border", label, disabled, image: par.image, ratio: Math.max(ratio(over(bc, par.bg), par.bg), ratio(fill, par.bg)) });
    }
  }
  return out;
}

/** The words of the app: `T("editor.undo")` in the language of the page, read from the same files the app uses (so a renamed button does not break the audit, a missing one does). */
const words = (lang) => {
  const json = JSON.parse(readFileSync(new URL(`../../web/src/locales/${lang}.json`, import.meta.url), "utf8"));
  return (key, vars = {}) => {
    const v = key.split(".").reduce((o, k) => o?.[k], json);
    if (typeof v !== "string") throw new Error(`no text for ${key} in ${lang}.json`);
    return v.replace(/\{\{(\w+)\}\}/g, (_, n) => String(vars[n]));
  };
};
const toolbarButton = (page, name) => page.getByRole("toolbar").getByRole("button", { name, exact: true });

/** One screen state of the editor, entered from a freshly opened cover; `T` gives the words of the page's language. */
const STATES = {
  idle: async () => {},
  selected: async (page) => page.locator(".ed-hit").first().click(),
  editing: async (page, T) => {
    await toolbarButton(page, T("editor.addText")).click();
    await expect(page.locator(".ed-textarea")).toBeFocused();
  },
  palette: async (page, T) => {
    await toolbarButton(page, T("editor.palette.toggle")).click();
    await expect(page.getByRole("complementary", { name: T("editor.palette.title") })).toBeVisible();
  },
  templates: async (page, T) => {
    await toolbarButton(page, T("editor.templates.button")).click();
    await expect(page.getByRole("radiogroup", { name: T("editor.templates.list") })).toBeVisible();
  },
  "template preview": async (page, T) => {
    await STATES.templates(page, T);
    await page.getByRole("radio").first().click();
    await expect(page.getByRole("button", { name: T("editor.templates.back") })).toBeVisible();
  },
  history: async (page, T) => {
    await toolbarButton(page, T("editor.history.button")).click();
    await expect(page.getByRole("complementary", { name: T("editor.history.title") })).toBeVisible();
    await expect(page.getByText(T("editor.history.loading"))).toHaveCount(0);
  },
  help: async (page, T) => {
    await toolbarButton(page, T("editor.helpButton")).click();
    await expect(page.getByRole("region", { name: T("editor.helpTitle") })).toBeVisible();
  },
  dialog: async (page, T) => {
    await toolbarButton(page, T("editor.resetAll")).click();
    await expect(page.getByRole("dialog", { name: T("editor.resetTitle") })).toBeVisible();
  },
  "save template dialog": async (page, T) => {
    await STATES.templates(page, T);
    await page.getByRole("button", { name: T("editor.templates.save.button"), exact: true }).click();
    await expect(page.getByRole("dialog", { name: T("editor.templates.save.title") })).toBeVisible();
  },
  error: async (page, T) => {
    await page.route("**/api/book-layout/save", (r) => r.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ status: "error", code: "internal" }) }));
    await STATES.editing(page, T);
    await page.keyboard.press("Escape");
    await page.getByRole("button", { name: T("editor.done"), exact: true }).click();
    await expect(page.getByRole("alert").filter({ hasText: T("editor.doneFailed") })).toBeVisible();
  },
};
const NAMED_ROLES = "button|link|textbox|spinbutton|radio|checkbox|combobox|tab|slider|switch|searchbox|option|menuitem";

/** Runs in the page: what a width, a language or a text zoom breaks in the chrome of the editor (the drawn page is the book's). Returns the list of problems. */
function layoutProblems() {
  const out = [];
  const root = document.querySelector(".dialog") ?? document.querySelector(".ed"); // a modal dialog covers the rest of the screen: only it counts
  const skip = (el) => el.closest(".ed-pagebox, .ed-thumb-view, .ed-tpl-thumb, .visually-hidden, .ed-handle, .ed-chrome, .ed-overlay, .bk-sheet");
  const name = (el) => `${el.tagName.toLowerCase()}.${String(el.className).split(" ")[0]} "${(el.getAttribute("aria-label") || el.textContent || "").trim().slice(0, 24)}"`;
  if (document.documentElement.scrollWidth > window.innerWidth + 1) out.push(`the page scrolls sideways (${document.documentElement.scrollWidth} > ${window.innerWidth})`);
  const boxes = [];
  /** The part of the element that is on screen: cut by every scrolling or clipping ancestor (a sheet that scrolls has controls under its edge, not on top of other controls). */
  const shown = (el) => {
    let { left, top, right, bottom } = el.getBoundingClientRect();
    for (let p = el.parentElement; p; p = p.parentElement) {
      const cs = getComputedStyle(p);
      if (cs.overflowX === "visible" && cs.overflowY === "visible") continue;
      const q = p.getBoundingClientRect();
      left = Math.max(left, q.left);
      top = Math.max(top, q.top);
      right = Math.min(right, q.right);
      bottom = Math.min(bottom, q.bottom);
    }
    return { left, top, right, bottom };
  };
  for (const el of root.querySelectorAll("*")) {
    if (skip(el)) continue;
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) continue;
    const cs = getComputedStyle(el);
    if (el.matches("button, a, input, select, textarea, [role=radio], [role=tab]") && (cs.overflowX !== "visible" || cs.textOverflow === "ellipsis") && el.scrollWidth > el.clientWidth + 1)
      out.push(`clipped: ${name(el)} (${el.scrollWidth} > ${el.clientWidth})`);
    if (el.matches("button, a, input, select, textarea") && r.right > window.innerWidth + 1) out.push(`beyond the right edge: ${name(el)}`);
    if (el.matches("button, a, input, select, textarea, [role=radio], [role=tab]") && cs.position !== "absolute" && cs.position !== "fixed") boxes.push({ el, r: shown(el) });
  }
  for (let i = 0; i < boxes.length; i++)
    for (let j = i + 1; j < boxes.length; j++) {
      const a = boxes[i];
      const b = boxes[j];
      if (a.el.contains(b.el) || b.el.contains(a.el)) continue;
      const w = Math.min(a.r.right, b.r.right) - Math.max(a.r.left, b.r.left);
      const h = Math.min(a.r.bottom, b.r.bottom) - Math.max(a.r.top, b.r.top);
      if (w > 2 && h > 2) out.push(`overlap: ${name(a.el)} and ${name(b.el)}`);
    }
  return out;
}

async function enter(browser, state, { lang = "en-US", ...view } = {}) {
  const page = await ownerPage(browser, { lang, ...view });
  const T = words(lang.startsWith("vi") ? "vi" : "en");
  await openEditor(page);
  await STATES[state](page, T);
  return { page, T };
}

test("accessibility tree: every control has a name, dialogs and messages have their roles, in English and Vietnamese (AC2)", async ({ browser }, info) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  test.setTimeout(240_000);
  for (const lang of ["en-US", "vi-VN"])
    for (const state of Object.keys(STATES)) {
      const { page, T } = await enter(browser, state, { lang });
      const snap = (await page.locator(".ed").ariaSnapshot()).replace(/- '([a-z]+ ".*")'/g, "- $1"); // names with a colon are quoted twice in the snapshot
      await info.attach(`aria-${lang}-${state}.yml`, { body: snap, contentType: "text/plain" });
      const unnamed = snap.split("\n").filter((l) => new RegExp(`^\\s*- (${NAMED_ROLES})(?= *(\\[|:|$))`).test(l));
      expect(unnamed, `${lang} ${state}: controls without an accessible name`).toEqual([]);
      expect(snap, `${lang} ${state}: the toolbar`).toContain(`- toolbar "${T("editor.toolbar")}"`);
      expect(snap, `${lang} ${state}: the page strip`).toContain(`- navigation "${T("editor.strip")}"`);
      expect(snap, `${lang} ${state}: the stage`).toMatch(/- region "[^"]+"/);
      if (state === "selected") {
        expect(snap).toMatch(/- complementary "[^"]+":\n\s+- heading "[^"]+" \[level=2\]/); // the panel is a landmark with a heading
        expect(snap).toMatch(/- button "[^"]+" \[pressed\]/); // the selected item says so
      }
      if (state === "templates" || state === "history") expect(snap).toMatch(/- complementary "[^"]+"/);
      if (state === "dialog") {
        const dlg = page.getByRole("dialog");
        await expect(dlg).toHaveAttribute("aria-modal", "true");
        await expect(dlg.getByRole("heading", { level: 2 })).toHaveText(T("editor.resetTitle"));
        await expect(dlg.getByRole("button")).toHaveText([T("editor.resetCancel"), T("editor.resetConfirm")]);
      }
      if (state === "error") {
        await expect(page.getByRole("alert").first()).toBeVisible();
        await expect(page.locator("[role=status]").filter({ hasText: T("editor.save.failing") })).toHaveCount(1); // the save status, polite
      }
      if (state === "editing") expect(snap).toMatch(/- textbox "[^"]+"/);
      await page.context().close();
    }
});

test("contrast: text 4.5:1, large text and bordered controls 3:1, in every state of the editor (AC4)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  test.setTimeout(240_000);
  const findings = [];
  for (const state of Object.keys(STATES)) {
    const { page } = await enter(browser, state);
    const rep = await page.evaluate(contrastReport);
    expect(rep.length).toBeGreaterThan(30);
    for (const x of rep) {
      const need = x.kind === "text" && !x.large ? 4.5 : 3;
      if (x.ratio >= need || x.image) continue;
      // a disabled control is exempt in WCAG 1.4.3 and 1.4.11, and the page strip's buttons are the page drawings (their frame is the page itself)
      if (x.disabled || /ed-thumb/.test(x.label)) continue;
      findings.push(`${state}: ${x.kind} ${x.ratio.toFixed(2)} < ${need}: ${x.label}`);
    }
    await page.context().close();
  }
  expect(findings).toEqual([]);
});

test("touch targets: every button, link and field of the editor chrome is at least 44 px on a touch tablet (768 and 1024 px) (AC4)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  test.setTimeout(240_000);
  const small = [];
  for (const width of [768, 1024])
    for (const state of ["idle", "selected", "palette", "templates", "template preview", "history", "dialog", "save template dialog", "help"]) {
      const { page } = await enter(browser, state, { width, height: 1000, touch: true });
      expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches), "the context emulates a touch screen").toBe(true);
      const rows = await page.evaluate(() => {
        const out = [];
        // the item boxes of the page and the resize handles are the gestures' business (a handle's hit area is 44 px: editor-gestures.spec.mjs), the thumbnails are pictures
        for (const el of document.querySelectorAll(".ed button, .ed a[href], .ed input, .ed select, .ed textarea, .ed [role=radio], .ed [role=tab]")) {
          if (el.closest(".ed-pagebox, .ed-strip, .visually-hidden") || el.type === "file") continue;
          const r = (el.type === "checkbox" ? (el.closest("label") ?? el) : el).getBoundingClientRect(); // the row of a check box is its target
          if (r.width === 0 || r.height === 0) continue;
          if (r.width < 44 || r.height < 44) out.push(`${Math.round(r.width)}x${Math.round(r.height)} ${el.tagName.toLowerCase()} "${(el.getAttribute("aria-label") || el.textContent || el.className).trim().slice(0, 30)}"`);
        }
        return out;
      });
      for (const r of rows) small.push(`${width} ${state}: ${r}`);
      await page.context().close();
    }
  expect([...new Set(small)]).toEqual([]);
});

test("reduced motion: no animation and no transition when the system asks for none (AC4)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser, { reducedMotion: "reduce" });
  await openEditor(page);
  const r = await page.evaluate(() => {
    const probe = (cls) => {
      const el = Object.assign(document.createElement("div"), { className: cls });
      document.querySelector(".ed").append(el);
      const cs = getComputedStyle(el);
      const v = { animation: cs.animationName, transition: cs.transitionDuration };
      el.remove();
      return v;
    };
    return { skeleton: probe("skeleton"), button: probe("btn btn-primary"), running: document.getAnimations().length };
  });
  expect(r.skeleton.animation).toBe("none");
  expect(r.button.transition.split(",").every((d) => parseFloat(d) === 0)).toBe(true);
  expect(r.running).toBe(0);
});

test("200 % browser zoom at 1280 px (a 640 px page: the phone editor) and 200 % text size: no clipped, overlapping or sideways-scrolling control (AC4)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  test.setTimeout(240_000);
  const problems = [];
  await step("browser zoom 200 %: the 1280 px window is 640 CSS px wide", async () => {
    for (const state of ["idle", "selected"]) {
      const { page } = await enter(browser, state, { width: 640, height: 450 });
      await expect(page.locator(".ed-phone")).toBeVisible();
      problems.push(...(await page.evaluate(layoutProblems)).map((p) => `zoom 200%, ${state}: ${p}`));
      for (const name of ["Done", "Undo"]) {
        const box = await page.getByRole("button", { name, exact: true }).boundingBox();
        if (!box || box.x + box.width > 640) problems.push(`zoom 200%, ${state}: "${name}" is not on screen`);
      }
      await page.context().close();
    }
  });
  await step("text size 200 % at 1280 px (every font size of the editor doubled, the way a text-only zoom does)", async () => {
    for (const state of ["idle", "selected", "templates", "history", "dialog"]) {
      const { page } = await enter(browser, state, { width: 1280, height: 900 });
      await page.evaluate(() => {
        const els = [...document.querySelectorAll(".ed, .ed *")].filter((e) => !e.closest(".ed-pagebox, .ed-thumb-view, .ed-tpl-thumb, svg"));
        const sizes = els.map((e) => parseFloat(getComputedStyle(e).fontSize));
        els.forEach((e, i) => (e.style.fontSize = `${sizes[i] * 2}px`));
      });
      problems.push(...(await page.evaluate(layoutProblems)).map((p) => `text 200%, ${state}: ${p}`));
      await page.context().close();
    }
  });
  expect(problems).toEqual([]);
});

test("Vietnamese texts do not clip or break the toolbar, panels and buttons at 768, 1024 and 1440 px (AC4)", async ({ browser }, info) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  test.setTimeout(300_000);
  const problems = [];
  for (const width of [768, 1024, 1440])
    for (const state of ["idle", "selected", "editing", "palette", "templates", "history", "dialog", "help", "error"]) {
      const { page } = await enter(browser, state, { lang: "vi-VN", width, height: 900 });
      await expect(page.locator("html")).toHaveAttribute("lang", "vi");
      problems.push(...(await page.evaluate(layoutProblems)).map((p) => `${width} ${state}: ${p}`));
      if (state === "idle" || state === "templates") await info.attach(`vi-${width}-${state}.png`, { body: await page.screenshot({ fullPage: true }), contentType: "image/png" });
      await page.context().close();
    }
  expect([...new Set(problems)]).toEqual([]);
});

test("switching the language in the middle of an edit keeps the document and the selection (AC4)", async ({ browser }) => {
  test.skip(!BASE || !OTP || !bookId, "needs the setup test");
  const page = await ownerPage(browser);
  await openEditor(page);
  const stored = async () => JSON.stringify((await (await page.request.get(`/api/book-layout/get?yearbook_id=${bookId}`)).json()).data);
  await page.getByRole("button", { name: "Cover title" }).click();
  const id = await page.locator(".ed-box.is-selected").getAttribute("data-target");
  const before = await stored();
  await expect(page.getByRole("complementary", { name: "Properties" }).getByRole("heading", { level: 2 })).toHaveText("Cover title");
  await page.getByRole("button", { name: "Tiếng Việt" }).click();
  await expect(page.locator("html")).toHaveAttribute("lang", "vi");
  await expect(page.getByRole("toolbar", { name: "Thanh công cụ trình soạn" }).or(page.getByRole("toolbar"))).toBeVisible();
  expect(await page.locator(".ed-box.is-selected").getAttribute("data-target"), "the same item is still selected").toBe(id);
  const heading = page.locator(".ed-panel h2");
  await expect(heading).not.toHaveText("Cover title"); // the panel is in Vietnamese now
  expect(await stored(), "the saved layout did not change").toBe(before);
  await page.getByRole("button", { name: "English" }).click();
  await expect(heading).toHaveText("Cover title");
  expect(await page.locator(".ed-box.is-selected").getAttribute("data-target")).toBe(id);
  await expect(page.locator(".ed-pagebox [data-layout-item] .bk-fit")).toHaveText(VI_TEXT); // the typed text stays
});
