// Automated print check of the book renderer (T-058, AC6): the book screen is opened in headless Chromium with a fixture book,
// printed with page.pdf() (what "Save as PDF" does), and the PDF itself is inspected with poppler.
//   make print-check        (or: cd tools/print-check && npm ci && npx playwright install chromium && npm test)
// Needs pdfinfo, pdffonts, pdfimages and pdftotext (poppler) on the PATH.
import assert from "node:assert/strict";
import { mkdtempSync, readFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, before, describe, test } from "node:test";
import { chromium } from "playwright";
import { CLASS_BOOK_CASES, DESIGN_TITLE, HIDDEN_PHOTOS, classBookPages, designOf, makeBook, makeClassBook, makeDesignClassBook, photoId, SIGNATURES, studentName } from "./fixture.mjs";
import { BASE, fitBoxes, inkIn, inspect, open, openClassBook, raster, savePdf, serveBook, signatureBoxes, startServer, text, words } from "./lib.mjs";

const squash = (x) => x.replace(/\s+/g, "");
const PT = { A5: [419.53, 595.28], A4: [595.28, 841.89], Letter: [612, 792] }; // exact sizes in points
const tmp = mkdtempSync(path.join(os.tmpdir(), "print-check-"));
let stop, browser;
before(async () => {
  stop = await startServer();
  browser = await chromium.launch();
});
after(async () => {
  await browser?.close();
  stop?.();
});

async function render(book, name) {
  const page = await browser.newPage();
  const hosts = new Set();
  const urls = [];
  page.on("request", (r) => {
    hosts.add(new URL(r.url()).host);
    urls.push(r.url());
  });
  const seen = await serveBook(page, book);
  const opened = await open(page);
  const screenText = await page.locator("main").innerText();
  const file = path.join(tmp, `${name}.pdf`);
  const saved = await savePdf(page, file);
  const shrunk = await page.locator("[data-fit=shrunk]").count();
  await page.close();
  return { opened, saved, hosts, urls, seen, shrunk, screenText, file, info: inspect(file), t: text(file) };
}

// T-121: the image queue (T-116) left the photos of pages never scrolled to without a src, and "ready" did not wait for them.
// The book says it is ready only when every photo on the pages has loaded, with nothing scrolled and no print event.
describe("every photo has loaded when the book says it is ready", () => {
  for (const [template, notes, photos] of [["navy-classic", 40, 1], ["memory-book", 40, 30], [null, 84, 30]]) {
    test(`${template ?? "basic"}, ${notes} notes`, async () => {
      const page = await browser.newPage();
      const seen = await serveBook(page, makeBook({ size: "A5", notes, template }));
      await open(page);
      const imgs = await page.evaluate(() =>
        [...document.querySelectorAll("img.bk-photo")].map((i) => ({ src: i.hasAttribute("src"), w: i.naturalWidth })),
      );
      assert.equal(imgs.filter((i) => i.src && i.w > 0).length, imgs.length, "every photo on the pages has loaded at ready");
      assert.ok(imgs.length >= photos, `${imgs.length} photos on the pages`);
      assert.ok(seen.media <= 2 * photos, `${seen.media} media requests`);
      await page.close();
    });
  }
});

describe("24-page book, 30 photos, 84 notes", () => {
  for (const size of ["A5", "A4", "Letter"]) {
    test(`${size}: pages, size, fonts, text, photos`, async () => {
      const r = await render(makeBook({ size }), `book-${size}`);
      console.log(`# ${size}: ready ${r.opened.readyMs} ms, pdf ${r.saved.pdfMs} ms, ${(r.saved.bytes / 1e6).toFixed(1)} MB, ${r.file}`);
      assert.equal(r.opened.readyMs < 30000, true, `ready in ${r.opened.readyMs} ms`);
      assert.equal(r.saved.pdfMs < 20000, true, `pdf in ${r.saved.pdfMs} ms`);
      assert.equal(r.info.pages, 24, "cover + profile + 21 notes pages + back");
      for (const [i, want] of PT[size].entries())
        assert.ok(Math.abs(r.info.sizePt[i] - want) < 1, `${size} page size ${r.info.sizePt} vs ${PT[size]}`);
      assert.ok(r.info.allFontsEmbedded, "every font is embedded");
      assert.equal(r.info.photos, 30, "30 photos");
      assert.ok(r.info.ppiMin >= 150, `photos at least 150 ppi, got ${r.info.ppiMin}`);
      assert.equal(r.seen.bookData, 1, "the book comes from one request");
      assert.deepEqual([...r.hosts], [new URL(BASE).host], "no request to any other host (no Google Fonts)");
      assert.ok(r.shrunk >= 1, "the long note was shrunk to fit");
      // Vietnamese text is extractable and exact (NFC); emoji survive as text
      for (const s of ["Nguyễn Thị Hồng Ánh", "Đặng Ngọc Bích Trâm", "Huỳnh Ngọc Diệp", "Chuyến dã ngoại ở Đà Lạt", "Lời nhắn từ bạn bè", "Học, học nữa, học mãi"])
        assert.ok(squash(r.t).includes(squash(s)), `extracted text contains "${s}"`); // pdftotext on the CI runner puts spaces inside letter-spaced words ("L Ạ I"), so spaces are ignored
      assert.ok(r.t.includes("😄"), "emoji are text");
      // the screen controls are not printed
      assert.ok(!r.t.includes("Xem trước và in") && !r.t.includes("Print or save"), "no app chrome in the PDF");
    });
  }
});

// The designed template of T-059: one friend per page, so 21 notes make cover + profile + 21 + back = 24 pages with 30 photos.
describe("memory-book template: 24 pages, 30 photos, 21 notes", () => {
  for (const size of ["A5", "A4", "Letter"]) {
    test(`${size}: pages, size, fonts, text, photos, no external request`, async () => {
      const r = await render(makeBook({ size, notes: 21, template: "memory-book", longNote: 4 }), `memory-${size}`);
      console.log(`# memory-book ${size}: ready ${r.opened.readyMs} ms, pdf ${r.saved.pdfMs} ms, ${(r.saved.bytes / 1e6).toFixed(2)} MB, ${r.file}`);
      assert.equal(r.info.pages, 24);
      for (const [i, want] of PT[size].entries())
        assert.ok(Math.abs(r.info.sizePt[i] - want) < 1, `${size} page size ${r.info.sizePt} vs ${PT[size]}`);
      assert.ok(r.info.allFontsEmbedded, "every font is embedded");
      const names = r.info.fonts.map((f) => f.name).join(" ");
      assert.match(names, /Baloo2/, "the title font is Baloo 2");
      assert.match(names, /PatrickHand/, "the handwriting font is Patrick Hand");
      assert.ok(!/Fredoka|Gaegu/.test(names), "the fonts of the canvas that lack Vietnamese are not used");
      assert.equal(r.info.photos, 30, "30 photos");
      assert.ok(r.info.ppiMin >= 150, `photos at least 150 ppi, got ${r.info.ppiMin}`);
      assert.equal(r.seen.bookData, 1, "the book comes from one request");
      assert.deepEqual([...r.hosts], [new URL(BASE).host], "no request to any other host");
      assert.ok(r.saved.bytes < 4e6, `the PDF stays small (the dotted paper is one picture), got ${r.saved.bytes}`);
      for (const s of ["Nguyễn Thị Hồng Ánh", "Đặng Ngọc Bích Trâm", "Từ một người bạn", "Giữ liên lạc nhé", "Hẹn ngày gặp lại", "Chuyến dã ngoại ở Đà Lạt"])
        assert.ok(squash(r.t).includes(squash(s)), `extracted text contains "${s}"`); // pdftotext on the CI runner puts spaces inside letter-spaced words ("L Ạ I"), so spaces are ignored
      assert.ok(r.t.includes("😄"), "emoji are text");
      assert.ok(!r.t.includes("Xem trước và in") && !r.t.includes("Nguyễn Thị Hồng Ánh, "), "no app chrome and no list of shrunk notes in the PDF");
    });
  }

  for (const size of ["A5", "A4", "Letter"]) {
    test(`${size}: long Vietnamese names and emoji everywhere: nothing is cut off and no text leaves its box`, async () => {
      const page = await browser.newPage();
      await serveBook(page, makeBook({ size, notes: 3, template: "memory-book", stress: true }));
      await open(page);
      const res = await page.evaluate(() => {
        const sheetOf = (e) => e.closest(".bk-sheet").getBoundingClientRect();
        const boxes = [...document.querySelectorAll(".bk-fit")];
        return {
          n: boxes.length,
          overflow: boxes.filter((e) => e.dataset.fit === "overflow").map((e) => e.className),
          outside: boxes
            .filter((e) => {
              const r = e.getBoundingClientRect(), s = sheetOf(e);
              return r.left < s.left - 1 || r.right > s.right + 1 || r.top < s.top - 1 || r.bottom > s.bottom + 1;
            })
            .map((e) => e.className),
          wide: boxes.filter((e) => e.scrollWidth > e.clientWidth + 1).map((e) => e.className),
        };
      });
      assert.ok(res.n >= 30, `${res.n} text boxes`);
      assert.deepEqual(res.overflow, [], "no text is cut off");
      assert.deepEqual(res.outside, [], "every text box is inside its page");
      assert.deepEqual(res.wide, [], "no line runs out of its box");
      await page.close();
    });
  }

  test("the 2000-character note is listed on the print screen with its author, its size in points and a link to the inbox", async () => {
    const page = await browser.newPage();
    await serveBook(page, makeBook({ size: "A5", notes: 21, template: "memory-book", longNote: 4 }));
    await open(page);
    const box = page.locator("details.bk-fits");
    await box.waitFor();
    const items = await box.locator("li").allInnerTexts();
    const mine = items.find((t) => t.startsWith("Lê Hoàng Phương Uyên,")); // note 5 of the fixture: 2000 characters
    assert.ok(mine, `Lê Hoàng Phương Uyên is listed: ${items.join(" | ")}`);
    assert.match(mine, /^Lê Hoàng Phương Uyên, \d+(\.5)? pt/);
    const link = box.locator("li", { hasText: "Lê Hoàng Phương Uyên" }).locator("a");
    assert.match(await link.getAttribute("href"), /^\/yearbooks\/\w+\/notes\?note=01J9Z3K6V8Q4M7N2P5N/);
    await page.close();
  });

  test("Vietnamese is set in the embedded lookalike fonts only, no letter falls back (Chromium reports the fonts it used)", async () => {
    const PANGRAM = "Tôi yêu những người bạn thân thiết: ưu ái, đằm thắm, nghĩ ngợi, quả ổi, ngũ cốc, ĐẶNG HUỲNH ẶẲẴ ỲỶỸỴ";
    const page = await browser.newPage();
    await serveBook(page, makeBook({ size: "A5", notes: 1, template: "memory-book" }));
    await open(page);
    await page.evaluate((s) => {
      document.body.insertAdjacentHTML("beforeend", `<div class="mb-page" style="position:fixed;left:0;top:0;width:900px;height:200px"><div id="a" class="disp" style="font-size:30px">${s}</div><div id="b" class="cute" style="font-size:30px">${s}</div><div id="c" style="font-size:30px;font-weight:700">${s}</div></div>`);
      return document.fonts.ready;
    }, PANGRAM);
    await page.waitForTimeout(300);
    const cdp = await page.context().newCDPSession(page);
    await cdp.send("DOM.enable");
    await cdp.send("CSS.enable");
    const { root } = await cdp.send("DOM.getDocument");
    for (const [id, family] of [["a", "Baloo 2"], ["b", "Patrick Hand"], ["c", "Nunito"]]) {
      const { nodeId } = await cdp.send("DOM.querySelector", { nodeId: root.nodeId, selector: `#${id}` });
      const { fonts } = await cdp.send("CSS.getPlatformFontsForNode", { nodeId });
      assert.equal(fonts.length, 1, `${id}: one font only (no fallback), got ${JSON.stringify(fonts)}`);
      assert.ok(fonts[0].familyName.startsWith(family), `${id}: ${family}, got ${fonts[0].familyName}`);
      assert.equal(fonts[0].isCustomFont, true);
      assert.ok(fonts[0].glyphCount >= PANGRAM.length - 2, `${id}: every character is drawn by the font (${fonts[0].glyphCount} of ${PANGRAM.length})`);
    }
    await page.close();
  });
});

// T-142: the colour themes and font sets of the memory-book (epic E09). Every theme x font set on A5 and A4 with the Vietnamese fixture: 24 pages, none blank,
// the fonts of that set (and of no other), only those font files requested, nothing cut off with long names everywhere, and no request leaves the origin.
// The default design (lavender, playful) is the book with no design: the cases above run it without one.
describe("memory-book themes x font sets (T-142)", () => {
  // The app's own screen also uses a family named Be Vietnam Pro (its toolbar), so that family's files are requested whatever the book's set is; Cormorant Garamond is the book's alone.
  const SETS = {
    playful: { has: /Baloo2.*PatrickHand|PatrickHand.*Baloo2/s, hasNot: /CormorantGaramond|BeVietnamPro/, files: /Baloo2|PatrickHand|Nunito/, filesNot: /CormorantGaramond/ },
    classic: { has: /CormorantGaramond.*BeVietnamPro|BeVietnamPro.*CormorantGaramond/s, hasNot: /Baloo2|PatrickHand|Nunito/, files: /CormorantGaramond|BeVietnamPro/, filesNot: /Baloo2|PatrickHand|Nunito/ },
  };
  for (const theme of ["lavender", "peach", "mint"])
    for (const fonts of ["playful", "classic"])
      for (const size of ["A5", "A4"]) {
        const set = SETS[fonts];
        test(`${theme} + ${fonts}, ${size}: 24 pages, none blank, that set's fonts only, self-hosted`, async () => {
          const r = await render(makeBook({ size, notes: 21, template: "memory-book", longNote: 4, design: designOf(theme, fonts) }), `memory-${theme}-${fonts}-${size}`);
          assert.equal(r.info.pages, 24);
          for (const [i, want] of PT[size].entries()) assert.ok(Math.abs(r.info.sizePt[i] - want) < 1, `${size} page size ${r.info.sizePt}`);
          assert.ok(r.info.allFontsEmbedded, "every font is embedded");
          const names = r.info.fonts.map((f) => f.name).join(" ");
          assert.match(names, set.has, `the fonts of ${fonts}: ${names}`);
          assert.doesNotMatch(names, set.hasNot, `no font of the other set: ${names}`);
          const files = r.urls.map((u) => new URL(u).pathname).filter((u) => /\.woff2/.test(u)).join(" ");
          assert.match(files, set.files, "the active set's font files are requested");
          assert.doesNotMatch(files, set.filesNot, `the font files of the other set are not requested: ${files}`);
          assert.deepEqual([...r.hosts], [new URL(BASE).host], "no request to any other host");
          const pages = r.t.split("\f").filter((_, i, a) => i < a.length - 1 || a[i].trim());
          assert.equal(pages.length, 24);
          assert.ok(pages.every((p) => p.trim().length > 20), "no blank page");
          for (const s of ["Nguyễn Thị Hồng Ánh", "Đặng Ngọc Bích Trâm", "Từ một người bạn", "Chuyến dã ngoại ở Đà Lạt"])
            assert.ok(squash(r.t).includes(squash(s)), `extracted text contains "${s}"`);
          assert.ok(r.t.includes("😄"), "emoji are text");
        });

        test(`${theme} + ${fonts}, ${size}: long Vietnamese names and emoji everywhere: nothing is cut off and no text leaves its box`, async () => {
          const page = await browser.newPage();
          await serveBook(page, makeBook({ size, notes: 3, template: "memory-book", stress: true, design: designOf(theme, fonts) }));
          await open(page);
          const res = await page.evaluate(([wantTheme, wantFonts]) => {
            const sheetOf = (e) => e.closest(".bk-sheet").getBoundingClientRect();
            const boxes = [...document.querySelectorAll(".bk-fit")];
            return {
              n: boxes.length,
              fontsAtReady: {
                status: document.fonts.status,
                loading: [...document.fonts].filter((f) => f.status === "loading").map((f) => f.family),
                set: [...document.fonts].some((f) => f.status === "loaded" && f.family.replace(/"/g, "") === (wantFonts === "classic" ? "Cormorant Garamond" : "Baloo 2")),
              },
              themed: [...document.querySelectorAll(".mb-page")].every((e) => e.dataset.theme === wantTheme && e.dataset.fonts === wantFonts),
              overflow: boxes.filter((e) => e.dataset.fit === "overflow").map((e) => e.className),
              outside: boxes
                .filter((e) => {
                  const r = e.getBoundingClientRect(), s = sheetOf(e);
                  return r.left < s.left - 1 || r.right > s.right + 1 || r.top < s.top - 1 || r.bottom > s.bottom + 1;
                })
                .map((e) => e.className),
              wide: boxes.filter((e) => e.scrollWidth > e.clientWidth + 1).map((e) => e.className),
            };
          }, [theme, fonts]);
          assert.ok(res.n >= 30, `${res.n} text boxes`);
          assert.deepEqual(res.fontsAtReady, { status: "loaded", loading: [], set: true }, "at ready the fonts of the set have loaded and none is still loading");
          assert.ok(res.themed, "every page carries the theme and the font set");
          assert.deepEqual(res.overflow, [], "no text is cut off");
          assert.deepEqual(res.outside, [], "every text box is inside its page");
          assert.deepEqual(res.wide, [], "no line runs out of its box");
          await page.close();
        });
      }

  test("a design this build does not know draws the defaults", async () => {
    const page = await browser.newPage();
    await serveBook(page, makeBook({ size: "A5", notes: 1, template: "memory-book", design: designOf("neon", "comic") }));
    await open(page);
    const set = await page.evaluate(() => [...new Set([...document.querySelectorAll(".mb-page")].map((e) => `${e.dataset.theme}/${e.dataset.fonts}`))]);
    assert.deepEqual(set, ["lavender/playful"]);
    await page.close();
  });
});

// T-143: sections, texts and images of the memory-book (epic E09). Hidden sections leave no page; the order follows the design; a replaced text at its
// maximum length (read from the manifest) fits or shrinks; the plain pattern leaves the cover without the dotted paper.
describe("memory-book sections, texts and images (T-143)", () => {
  const manifest = JSON.parse(readFileSync(path.join(path.dirname(new URL(import.meta.url).pathname), "../../internal/templates/embed/memory-book.json"), "utf8")).design;
  /** Vietnamese and emoji, exactly `max` characters (runes, as the server counts them). */
  const filler = (marker, max) => [...`${marker} Chúc bạn thật nhiều sức khỏe 😄 Hẹn gặp lại ${"Ấ ".repeat(40)}`].slice(0, max).join("");
  const MAX_TEXTS = Object.fromEntries(manifest.texts.map((t) => [t.id, filler(`<${t.id}>`.slice(0, 6), t.max)]));
  // what each page of the book says: the cover, the profile, a friend's page and the back
  const KIND = [["cover", /cuốn lưu bút này là của/], ["profile", /Giới thiệu về mình/], ["notes", /Từ một người bạn/], ["back", /Giữ liên lạc nhé/]];
  const kinds = (t) => t.split("\f").filter((_, i, a) => i < a.length - 1 || a[i].trim()).map((p) => KIND.find(([, re]) => re.test(p))?.[0] ?? "?");
  const NOTES = 3;
  const CASES = {
    "profile hidden": { design: { hidden: ["profile"] }, want: ["cover", "notes", "notes", "notes", "back"] },
    "back hidden": { design: { hidden: ["back"] }, want: ["cover", "profile", "notes", "notes", "notes"] },
    "profile and back hidden: cover and notes only": { design: { hidden: ["profile", "back"] }, want: ["cover", "notes", "notes", "notes"] },
    "notes before profile": { design: { order: ["cover", "notes", "profile", "back"] }, want: ["cover", "notes", "notes", "notes", "profile", "back"] },
    "notes before profile, back hidden": { design: { order: ["cover", "notes", "profile", "back"], hidden: ["back"] }, want: ["cover", "notes", "notes", "notes", "profile"] },
  };
  for (const size of ["A5", "A4"])
    for (const [name, c] of Object.entries(CASES))
      test(`${name}, ${size}: the pages that print, in order, none blank, every photo loaded at ready`, async () => {
        const page = await browser.newPage();
        const book = makeBook({ size, notes: NOTES, template: "memory-book", design: designOf("lavender", "playful", c.design) });
        const seen = await serveBook(page, book);
        await open(page);
        assert.equal(await page.evaluate(() => document.documentElement.dataset.bookReady), String(c.want.length), "the page count of the ready signal");
        const imgs = await page.evaluate(() => [...document.querySelectorAll("img.bk-photo")].map((i) => ({ src: i.hasAttribute("src"), w: i.naturalWidth })));
        assert.ok(imgs.length > 0 && imgs.every((i) => i.src && i.w > 0), `every photo on the pages has loaded at ready (${imgs.length})`);
        const file = path.join(tmp, `sections-${size}-${name.replace(/\W+/g, "-")}.pdf`);
        await savePdf(page, file);
        const info = inspect(file);
        assert.equal(info.pages, c.want.length);
        for (const [i, want] of PT[size].entries()) assert.ok(Math.abs(info.sizePt[i] - want) < 1, `${size} page size ${info.sizePt}`);
        const t = text(file);
        assert.deepEqual(kinds(t), c.want, "the kind of each page");
        assert.ok(t.split("\f").filter((p) => p.trim()).every((p) => p.trim().length > 20), "no blank page");
        // the owner's portrait is on the profile page only: a hidden profile does not even fetch it
        const hidden = (c.design.hidden ?? []).includes("profile");
        assert.equal(seen.media <= 2 * (hidden ? 1 + NOTES : 2 + NOTES), true, `${seen.media} media requests`);
        const own = await page.locator("[data-fit=overflow]").count();
        assert.equal(own, 0, "no text is cut off");
        await page.close();
      });

  for (const size of ["A5", "A4"])
    for (const fonts of ["playful", "classic"]) {
      test(`every text at its maximum length (${fonts}), ${size}: all pages, fits or shrinks, nothing leaves its page, no HTML`, async () => {
        const texts = { ...MAX_TEXTS, "profile.subtitle": `<script>alert("x")</script>${MAX_TEXTS["profile.subtitle"]}`.slice(0, 50) };
        const page = await browser.newPage();
        const book = makeBook({ size, notes: NOTES, template: "memory-book", stress: true, design: designOf("peach", fonts, { texts }) });
        await serveBook(page, book);
        await open(page);
        const res = await page.evaluate(() => {
          const sheetOf = (e) => e.closest(".bk-sheet").getBoundingClientRect();
          const boxes = [...document.querySelectorAll(".bk-fit")];
          return {
            scripts: document.querySelectorAll("main script, .bk-sheet script").length,
            overflow: boxes.filter((e) => e.dataset.fit === "overflow").map((e) => e.className),
            shrunk: boxes.filter((e) => e.dataset.fit === "shrunk").length,
            outside: boxes.filter((e) => { const r = e.getBoundingClientRect(), s = sheetOf(e); return r.left < s.left - 1 || r.right > s.right + 1 || r.top < s.top - 1 || r.bottom > s.bottom + 1; }).map((e) => e.className),
          };
        });
        assert.equal(res.scripts, 0, "a <script> text is text, not markup");
        assert.deepEqual(res.overflow, [], "no text is cut off");
        assert.deepEqual(res.outside, [], "every text box is inside its page");
        assert.ok(res.shrunk > 0, "long texts shrink");
        const file = path.join(tmp, `texts-${fonts}-${size}.pdf`);
        await savePdf(page, file);
        const t = squash(text(file));
        assert.equal(inspect(file).pages, 2 + 1 + NOTES);
        for (const id of ["cover.belongs", "profile.heading", "notes.heading", "back.heading", "back.subtitle", "back.pact"]) assert.ok(t.includes(squash(MAX_TEXTS[id])), `the PDF has the text ${id}`);
        assert.ok(t.includes(squash('<script>alert("x")</script>')), "a <script> string prints literally");
        await page.close();
      });
    }

  for (const size of ["A5", "A4"])
    test(`pattern plain, ${size}: the cover has no dotted paper, the other pages keep it`, async () => {
      const page = await browser.newPage();
      await serveBook(page, makeBook({ size, notes: 2, template: "memory-book", design: designOf("lavender", "playful", { images: { "cover.pattern": "plain" } }) }));
      await open(page);
      const bg = await page.evaluate(() => [...document.querySelectorAll(".mb-page")].map((e) => getComputedStyle(e).backgroundImage));
      assert.equal(bg[0], "none", "the cover");
      assert.ok(bg.slice(1).every((b) => /memory-dots/.test(b)), "the other pages");
      await page.close();
    });

  test("an image choice this build does not know draws the dotted paper", async () => {
    const page = await browser.newPage();
    await serveBook(page, makeBook({ size: "A5", notes: 1, template: "memory-book", design: designOf("lavender", "playful", { images: { "cover.pattern": "stripes" } }) }));
    await open(page);
    assert.match(await page.evaluate(() => getComputedStyle(document.querySelector(".mb-page")).backgroundImage), /memory-dots/);
    await page.close();
  });

  // T-143 carry-over from T-142 QA: Cormorant's old-style figures drew the digit 1 as a capital I ("Lớp I2AI" for 12A1).
  test("classic: a class name with a 1 reads 12A1 (lining figures), the cover title at its maximum length fits", async () => {
    const page = await browser.newPage();
    const book = makeBook({ size: "A5", notes: 1, template: "memory-book", design: designOf("lavender", "classic") });
    book.yearbook.class_name = "12A1";
    book.yearbook.title = [..."Lớp 12A1 11A11 - THPT Chu Văn An 2011 ".repeat(4)].slice(0, 120).join("");
    await serveBook(page, book);
    await open(page);
    // the figures as the book draws them, with lining figures forced and with old-style ones forced, as pictures: the book must draw the first, and the two must differ
    const probe = async (variant) => {
      await page.evaluate((v) => {
        const e = document.createElement("span");
        e.id = "probe";
        e.className = "disp";
        e.style.cssText = `position:fixed;left:0;top:0;z-index:99;background:#fff;font-size:100px;white-space:nowrap;${v ? `font-variant-numeric:${v}` : ""}`;
        e.textContent = "1234567890 12A1";
        document.querySelector(".mb-page").append(e);
      }, variant);
      const png = await page.locator("#probe").screenshot();
      await page.evaluate(() => document.getElementById("probe").remove());
      return png;
    };
    const res = { fit: await page.evaluate(() => document.querySelector(".mb-cover-title").dataset.fit), book: await probe(""), lining: await probe("lining-nums"), old: await probe("oldstyle-nums") };
    assert.notEqual(Buffer.compare(res.lining, res.old), 0, "the font has lining figures that differ from its old-style ones");
    assert.equal(Buffer.compare(res.book, res.lining), 0, "the book draws lining figures");
    assert.notEqual(res.fit, "overflow", "the title at 120 characters is not cut off");
    const file = path.join(tmp, "classic-lining.pdf");
    await savePdf(page, file);
    assert.ok(squash(text(file)).includes("12A1"), "12A1 is in the PDF text of the cover");
    await page.close();
  });
});

// The second designed template (T-060): the navy and gold canvas, four friends per autograph page: 84 notes make cover + profile + 21 + back = 24 pages.
describe("navy-classic template: 24 pages, 84 notes", () => {
  for (const size of ["A5", "A4", "Letter"]) {
    test(`${size}: pages, size, fonts, text, photo, no external request`, async () => {
      const r = await render(makeBook({ size, notes: 84, template: "navy-classic", longNote: 4 }), `navy-${size}`);
      console.log(`# navy-classic ${size}: ready ${r.opened.readyMs} ms, pdf ${r.saved.pdfMs} ms, ${(r.saved.bytes / 1e6).toFixed(2)} MB, ${r.file}`);
      assert.equal(r.info.pages, 24);
      for (const [i, want] of PT[size].entries())
        assert.ok(Math.abs(r.info.sizePt[i] - want) < 1, `${size} page size ${r.info.sizePt} vs ${PT[size]}`);
      assert.ok(r.info.allFontsEmbedded, "every font is embedded");
      const names = r.info.fonts.map((f) => f.name).join(" ");
      assert.match(names, /CormorantGaramond/, "the serif is Cormorant Garamond");
      assert.match(names, /BeVietnamPro/, "the capitals are Be Vietnam Pro");
      assert.ok(!/Jost/.test(names), "Jost (no usable Vietnamese) is not used");
      assert.equal(r.info.photos, 1, "the design shows the owner's portrait only (not the cover photo, not the friends' photos)");
      assert.equal(r.seen.media, 1, "and the book fetches only that photo");
      assert.ok(r.info.ppiMin >= 150, `the portrait at least 150 ppi, got ${r.info.ppiMin}`);
      assert.equal(r.seen.bookData, 1, "the book comes from one request");
      assert.deepEqual([...r.hosts], [new URL(BASE).host], "no request to any other host");
      assert.ok(r.saved.bytes < 4e6, `the PDF stays small, got ${r.saved.bytes}`);
      // text-transform makes the small labels capitals in the PDF
      for (const s of ["Nguyễn Thị Hồng Ánh", "Đặng Ngọc Bích Trâm", "Giới thiệu về mình", "Lưu bút", "Tạm biệt, khóa 2026", "KHÓA", "ĐỂ LẠI ĐÔI LỜI", "Chuyến dã ngoại ở Đà Lạt", "Học, học nữa, học mãi"])
        assert.ok(squash(r.t).includes(squash(s)), `extracted text contains "${s}"`); // pdftotext on the CI runner puts spaces inside letter-spaced words ("L Ạ I"), so spaces are ignored
      assert.ok(r.t.includes("😄"), "emoji are text");
      assert.ok(!r.t.includes("Xem trước và in"), "no app chrome in the PDF");
    });
  }

  for (const size of ["A5", "A4", "Letter"]) {
    test(`${size}: long Vietnamese names and emoji everywhere: nothing is cut off and no text leaves its box`, async () => {
      const page = await browser.newPage();
      await serveBook(page, makeBook({ size, notes: 8, template: "navy-classic", stress: true }));
      await open(page);
      const res = await page.evaluate(() => {
        const sheetOf = (e) => e.closest(".bk-sheet").getBoundingClientRect();
        const boxes = [...document.querySelectorAll(".bk-fit")];
        return {
          n: boxes.length,
          overflow: boxes.filter((e) => e.dataset.fit === "overflow").map((e) => e.className),
          outside: boxes
            .filter((e) => {
              const r = e.getBoundingClientRect(), s = sheetOf(e);
              return r.left < s.left - 1 || r.right > s.right + 1 || r.top < s.top - 1 || r.bottom > s.bottom + 1;
            })
            .map((e) => e.className),
          wide: boxes.filter((e) => e.scrollWidth > e.clientWidth + 1).map((e) => e.className),
        };
      });
      assert.ok(res.n >= 30, `${res.n} text boxes`);
      assert.deepEqual(res.overflow, [], "no text is cut off");
      assert.deepEqual(res.outside, [], "every text box is inside its page");
      assert.deepEqual(res.wide, [], "no line runs out of its box");
      await page.close();
    });
  }

  test("the oval takes portrait and landscape photos alike (cover fit) and is a real oval", async () => {
    for (const [what, id] of [["portrait", 1], ["landscape", 2]]) {
      const page = await browser.newPage();
      const book = makeBook({ size: "A5", notes: 1, template: "navy-classic" });
      book.profile.photo_media_id = photoId(id);
      await serveBook(page, book);
      await open(page);
      const o = await page.evaluate(() => {
        const img = document.querySelector("img.nc-oval");
        img.scrollIntoView({ block: "center" }); // elementFromPoint only sees what is on screen
        const r = img.getBoundingClientRect(), cs = getComputedStyle(img);
        const hit = (x, y) => document.elementFromPoint(x, y) === img;
        return {
          natural: [img.naturalWidth, img.naturalHeight], ratio: r.width / r.height, fit: cs.objectFit, radius: cs.borderTopLeftRadius,
          centre: hit(r.left + r.width / 2, r.top + r.height / 2), corner: hit(r.left + 2, r.top + 2),
        };
      });
      assert.ok(what === "portrait" ? o.natural[1] > o.natural[0] : o.natural[0] > o.natural[1], `${what} photo is ${o.natural}`);
      assert.ok(Math.abs(o.ratio - 200 / 260) < 0.01, `${what}: the oval keeps the canvas's 200 x 260 shape, got ${o.ratio}`);
      assert.equal(o.fit, "cover", `${what}: the photo covers the oval`);
      assert.equal(o.radius, "50%");
      assert.ok(o.centre && !o.corner, `${what}: the centre is the photo, the corner of its box is not`);
      await page.close();
    }
  });

  test("the 2000-character note is listed on the print screen with its author, its size in points and a link to the inbox", async () => {
    const page = await browser.newPage();
    await serveBook(page, makeBook({ size: "A5", notes: 21, template: "navy-classic", longNote: 4 }));
    await open(page);
    const box = page.locator("details.bk-fits");
    await box.waitFor();
    const items = await box.locator("li").allInnerTexts();
    const mine = items.find((t) => t.startsWith("Lê Hoàng Phương Uyên,")); // note 5 of the fixture: 2000 characters
    assert.ok(mine, `Lê Hoàng Phương Uyên is listed: ${items.join(" | ")}`);
    assert.match(mine, /^Lê Hoàng Phương Uyên, \d+(\.5)? pt/);
    const link = box.locator("li", { hasText: "Lê Hoàng Phương Uyên" }).locator("a");
    assert.match(await link.getAttribute("href"), /^\/yearbooks\/\w+\/notes\?note=01J9Z3K6V8Q4M7N2P5N/);
    await page.close();
  });

  test("Vietnamese is set in the embedded fonts only, no letter falls back (Chromium reports the fonts it used)", async () => {
    const PANGRAM = "Tôi yêu những người bạn thân thiết: ưu ái, đằm thắm, nghĩ ngợi, quả ổi, ngũ cốc, ĐẶNG HUỲNH ẶẲẴ ỲỶỸỴ";
    const page = await browser.newPage();
    await serveBook(page, makeBook({ size: "A5", notes: 1, template: "navy-classic" }));
    await open(page);
    await page.evaluate((s) => {
      document.body.insertAdjacentHTML("beforeend", `<div class="nc-page" style="position:fixed;left:0;top:0;width:900px;height:200px"><div id="a" class="serif" style="font-size:30px">${s}</div><div id="b" class="serif" style="font-size:30px;font-style:italic;font-weight:600">${s}</div><div id="c" class="caps" style="font-size:20px">${s}</div></div>`);
      return document.fonts.ready;
    }, PANGRAM);
    await page.waitForTimeout(300);
    const cdp = await page.context().newCDPSession(page);
    await cdp.send("DOM.enable");
    await cdp.send("CSS.enable");
    const { root } = await cdp.send("DOM.getDocument");
    for (const [id, family] of [["a", "Cormorant Garamond"], ["b", "Cormorant Garamond"], ["c", "Be Vietnam Pro"]]) {
      const { nodeId } = await cdp.send("DOM.querySelector", { nodeId: root.nodeId, selector: `#${id}` });
      const { fonts } = await cdp.send("CSS.getPlatformFontsForNode", { nodeId });
      assert.equal(fonts.length, 1, `${id}: one font only (no fallback), got ${JSON.stringify(fonts)}`);
      assert.ok(fonts[0].familyName.startsWith(family), `${id}: ${family}, got ${fonts[0].familyName}`);
      assert.equal(fonts[0].isCustomFont, true);
      assert.ok(fonts[0].glyphCount >= PANGRAM.length - 2, `${id}: every character is drawn by the font (${fonts[0].glyphCount} of ${PANGRAM.length})`);
    }
    await page.close();
  });
});

// T-145: the colour themes and font sets of the navy-classic (epic E09). Every theme x font set on A5 and A4 with the Vietnamese fixture. The default design
// (navy, classic) is the book with no design: the cases above run it without one.
describe("navy-classic themes x font sets, sections and texts (T-145)", () => {
  const manifest = JSON.parse(readFileSync(path.join(path.dirname(new URL(import.meta.url).pathname), "../../internal/templates/embed/navy-classic.json"), "utf8")).design;
  // The app's own screen also uses a family named Be Vietnam Pro (its toolbar), so that family's files are requested whatever the book's set is; Nunito and Cormorant Garamond are the book's alone.
  const SETS = {
    classic: { has: /CormorantGaramond.*BeVietnamPro|BeVietnamPro.*CormorantGaramond/s, hasNot: /Nunito/, files: /CormorantGaramond|BeVietnamPro/, filesNot: /Nunito/, family: "Cormorant Garamond" },
    modern: { has: /Nunito/, hasNot: /CormorantGaramond|BeVietnamPro/, files: /Nunito/, filesNot: /CormorantGaramond/, family: "Nunito" },
  };
  const NOTES = 21; // 6 notes pages of four
  for (const theme of ["navy", "burgundy", "forest"])
    for (const fonts of ["classic", "modern"])
      for (const size of ["A5", "A4"]) {
        const set = SETS[fonts];
        test(`${theme} + ${fonts}, ${size}: 9 pages, none blank, that set's fonts only, self-hosted`, async () => {
          const r = await render(makeBook({ size, notes: NOTES, template: "navy-classic", longNote: 4, design: designOf(theme, fonts) }), `navy-${theme}-${fonts}-${size}`);
          assert.equal(r.info.pages, 3 + Math.ceil(NOTES / 4));
          for (const [i, want] of PT[size].entries()) assert.ok(Math.abs(r.info.sizePt[i] - want) < 1, `${size} page size ${r.info.sizePt}`);
          assert.ok(r.info.allFontsEmbedded, "every font is embedded");
          const names = r.info.fonts.map((f) => f.name).join(" ");
          assert.match(names, set.has, `the fonts of ${fonts}: ${names}`);
          assert.doesNotMatch(names, set.hasNot, `no font of the other set: ${names}`);
          const files = r.urls.map((u) => new URL(u).pathname).filter((u) => /\.woff2/.test(u)).join(" ");
          assert.match(files, set.files, "the active set's font files are requested");
          assert.doesNotMatch(files, set.filesNot, `the font files of the other set are not requested: ${files}`);
          assert.deepEqual([...r.hosts], [new URL(BASE).host], "no request to any other host");
          const pages = r.t.split("\f").filter((_, i, a) => i < a.length - 1 || a[i].trim());
          assert.equal(pages.length, r.info.pages);
          assert.ok(pages.every((p) => p.trim().length > 20), "no blank page");
          for (const s of ["Nguyễn Thị Hồng Ánh", "Đặng Ngọc Bích Trâm", "Giới thiệu về mình", "Lưu bút", "Chuyến dã ngoại ở Đà Lạt"]) assert.ok(squash(r.t).includes(squash(s)), `extracted text contains "${s}"`);
          assert.ok(r.t.includes("😄"), "emoji are text");
        });

        test(`${theme} + ${fonts}, ${size}: long Vietnamese names and emoji everywhere: nothing is cut off and no text leaves its box`, async () => {
          const page = await browser.newPage();
          await serveBook(page, makeBook({ size, notes: 8, template: "navy-classic", stress: true, design: designOf(theme, fonts) }));
          await open(page);
          const res = await page.evaluate(([wantTheme, wantFonts, family]) => {
            const sheetOf = (e) => e.closest(".bk-sheet").getBoundingClientRect();
            const boxes = [...document.querySelectorAll(".bk-fit")];
            return {
              n: boxes.length,
              fontsAtReady: {
                status: document.fonts.status,
                loading: [...document.fonts].filter((f) => f.status === "loading").map((f) => f.family),
                set: [...document.fonts].some((f) => f.status === "loaded" && f.family.replace(/"/g, "") === family),
              },
              themed: [...document.querySelectorAll(".nc-page")].every((e) => e.dataset.theme === wantTheme && e.dataset.fonts === wantFonts),
              overflow: boxes.filter((e) => e.dataset.fit === "overflow").map((e) => e.className),
              outside: boxes
                .filter((e) => {
                  const r = e.getBoundingClientRect(), s = sheetOf(e);
                  return r.left < s.left - 1 || r.right > s.right + 1 || r.top < s.top - 1 || r.bottom > s.bottom + 1;
                })
                .map((e) => e.className),
              wide: boxes.filter((e) => e.scrollWidth > e.clientWidth + 1).map((e) => e.className),
            };
          }, [theme, fonts, set.family]);
          assert.ok(res.n >= 30, `${res.n} text boxes`);
          assert.deepEqual(res.fontsAtReady, { status: "loaded", loading: [], set: true }, "at ready the fonts of the set have loaded and none is still loading");
          assert.ok(res.themed, "every page carries the theme and the font set");
          assert.deepEqual(res.overflow, [], "no text is cut off");
          assert.deepEqual(res.outside, [], "every text box is inside its page");
          assert.deepEqual(res.wide, [], "no line runs out of its box");
          await page.close();
        });

        // The default set keeps the signature cases above; every other combination runs them too (T-097 rules: the page count does not change, only messages may be cut off,
        // and no text box meets a signature box).
        if (theme !== "navy" || fonts !== "classic")
          test(`${theme} + ${fonts}, ${size}: signatures keep the page count and never meet a text`, async () => {
            const notes = 8;
            const page = await browser.newPage();
            await serveBook(page, makeBook({ size, notes, template: "navy-classic", longNote: 4, signatures: true, design: designOf(theme, fonts) }));
            await open(page);
            const cut = await page.locator("[data-fit=overflow]").evaluateAll((es) => es.map((e) => e.className));
            assert.ok(cut.every((c) => /\bnc-msg\b/.test(c)), `only messages may be cut off, got ${cut.join(" | ")}`);
            const sheets = await signatureBoxes(page);
            const file = path.join(tmp, `navy-sig-${theme}-${fonts}-${size}.pdf`);
            await savePdf(page, file);
            await page.close();
            assert.equal(inspect(file).pages, 3 + Math.ceil(notes / 4));
            const signed = sheets.flatMap((s, p) => s.sigs.map((sig) => ({ p, sig, texts: s.texts })));
            assert.equal(signed.length, 6, "3 of every 4 fixture notes are signed");
            for (const { p, sig, texts } of signed)
              for (const t of texts) {
                const apart = t.x1 <= sig.x0 + 0.001 || t.x0 >= sig.x1 - 0.001 || t.y1 <= sig.y0 + 0.001 || t.y0 >= sig.y1 - 0.001;
                assert.ok(apart, `page ${p + 1}: the text box "${t.text.slice(0, 20)}" overlaps the signature box`);
              }
            const pdfWords = words(file);
            for (const { p, sig } of signed) {
              const { w, h, words: ws } = pdfWords[p];
              for (const x of ws) {
                const cx = (x.x0 + x.x1) / 2 / w, cy = (x.y0 + x.y1) / 2 / h;
                assert.ok(!(cx > sig.x0 && cx < sig.x1 && cy > sig.y0 && cy < sig.y1), `page ${p + 1}: the word "${x.t}" lies in the signature box`);
              }
            }
          });
      }

  test("a design this build does not know draws the defaults", async () => {
    const page = await browser.newPage();
    await serveBook(page, makeBook({ size: "A5", notes: 1, template: "navy-classic", design: designOf("neon", "comic") }));
    await open(page);
    const set = await page.evaluate(() => [...new Set([...document.querySelectorAll(".nc-page")].map((e) => `${e.dataset.theme}/${e.dataset.fonts}`))]);
    assert.deepEqual(set, ["navy/classic"]);
    await page.close();
  });

  // Sections: hidden sections leave no page and the order follows the design. Every page carries a marker text of its kind (letter-spaced capitals come out of pdftotext with spaces: squash).
  const KIND = [["notes", /ĐỂLẠIĐÔILỜI/], ["profile", /Giớithiệuvềmình/], ["back", /Cảmơnbạnđãlàmộtphần/], ["cover", /Khóa/]];
  const kinds = (t) => t.split("\f").filter((_, i, a) => i < a.length - 1 || a[i].trim()).map((p) => KIND.find(([, re]) => re.test(squash(p)))?.[0] ?? "?");
  const SECTION_NOTES = 5; // two notes pages
  const CASES = {
    "profile hidden": { design: { hidden: ["profile"] }, want: ["cover", "notes", "notes", "back"] },
    "back hidden": { design: { hidden: ["back"] }, want: ["cover", "profile", "notes", "notes"] },
    "profile and back hidden: cover and notes only": { design: { hidden: ["profile", "back"] }, want: ["cover", "notes", "notes"] },
    "notes before profile": { design: { order: ["cover", "notes", "profile", "back"] }, want: ["cover", "notes", "notes", "profile", "back"] },
    "notes before profile, back hidden": { design: { order: ["cover", "notes", "profile", "back"], hidden: ["back"] }, want: ["cover", "notes", "notes", "profile"] },
  };
  for (const size of ["A5", "A4"])
    for (const [name, c] of Object.entries(CASES))
      test(`${name}, ${size}: the pages that print, in order, none blank`, async () => {
        const page = await browser.newPage();
        const seen = await serveBook(page, makeBook({ size, notes: SECTION_NOTES, template: "navy-classic", design: designOf("burgundy", "modern", c.design) }));
        await open(page);
        assert.equal(await page.evaluate(() => document.documentElement.dataset.bookReady), String(c.want.length), "the page count of the ready signal");
        const file = path.join(tmp, `navy-sections-${size}-${name.replace(/\W+/g, "-")}.pdf`);
        await savePdf(page, file);
        const info = inspect(file);
        assert.equal(info.pages, c.want.length);
        for (const [i, want] of PT[size].entries()) assert.ok(Math.abs(info.sizePt[i] - want) < 1, `${size} page size ${info.sizePt}`);
        const t = text(file);
        assert.deepEqual(kinds(t), c.want, "the kind of each page");
        assert.ok(t.split("\f").filter((p) => p.trim()).every((p) => p.trim().length > 20), "no blank page");
        // the owner's portrait is on the profile page only: a hidden profile does not even fetch it
        assert.equal(seen.media, (c.design.hidden ?? []).includes("profile") ? 0 : 1, `${seen.media} media requests`);
        assert.equal(await page.locator("[data-fit=overflow]").count(), 0, "no text is cut off");
        await page.close();
      });

  // Every text at its maximum length, read from the manifest.
  const filler = (marker, max) => [...`${marker} Chúc bạn thật nhiều sức khỏe 😄 Hẹn gặp lại ${"Ấ ".repeat(60)}`].slice(0, max).join("");
  const MAX_TEXTS = Object.fromEntries(manifest.texts.map((t) => [t.id, filler(`<${t.id.slice(0, 1)}>`, t.max)]));
  for (const size of ["A5", "A4"])
    for (const fonts of ["classic", "modern"])
      test(`every text at its maximum length (${fonts}), ${size}: all pages, fits or shrinks, nothing leaves its page, no HTML`, async () => {
        const texts = { ...MAX_TEXTS, "back.thanks": `<script>alert("x")</script>${MAX_TEXTS["back.thanks"]}`.slice(0, 80) };
        const page = await browser.newPage();
        await serveBook(page, makeBook({ size, notes: 3, template: "navy-classic", stress: true, design: designOf("forest", fonts, { texts }) }));
        await open(page);
        const res = await page.evaluate(() => {
          const sheetOf = (e) => e.closest(".bk-sheet").getBoundingClientRect();
          const boxes = [...document.querySelectorAll(".bk-fit")];
          return {
            scripts: document.querySelectorAll("main script, .bk-sheet script").length,
            overflow: boxes.filter((e) => e.dataset.fit === "overflow").map((e) => e.className),
            shrunk: boxes.filter((e) => e.dataset.fit === "shrunk").length,
            outside: boxes.filter((e) => { const r = e.getBoundingClientRect(), s = sheetOf(e); return r.left < s.left - 1 || r.right > s.right + 1 || r.top < s.top - 1 || r.bottom > s.bottom + 1; }).map((e) => e.className),
            replaced: [...document.querySelectorAll(".nc-eyebrow-fit, .nc-title-fit, .nc-thanks-fit")].length,
          };
        });
        assert.equal(res.scripts, 0, "a <script> text is text, not markup");
        assert.equal(res.replaced, 5, "the five replaced texts are drawn in their fit boxes");
        assert.deepEqual(res.overflow, [], "no text is cut off");
        assert.deepEqual(res.outside, [], "every text box is inside its page");
        assert.ok(res.shrunk > 0, "long texts shrink");
        const file = path.join(tmp, `navy-texts-${fonts}-${size}.pdf`);
        await savePdf(page, file);
        const t = squash(text(file));
        assert.equal(inspect(file).pages, 3 + 1);
        for (const id of ["profile.eyebrow", "profile.heading", "notes.eyebrow", "notes.heading"]) assert.ok(t.toLowerCase().includes(squash(MAX_TEXTS[id]).toLowerCase()), `the PDF has the text ${id}`); // the eyebrows print as capitals
        assert.ok(t.includes(squash('<script>alert("x")</script>')), "a <script> string prints literally");
        await page.close();
      });
});

// A note that is still too long at the smallest font is cut off at its box. In the printed PDF (not only on screen) no word of it may lie
// outside that box: it must not run over the footer or off the page (T-059 QA). Two notes: 400 short lines, and 2000 characters in one paragraph.
for (const template of ["memory-book", "navy-classic", "basic"]) {
  for (const size of ["A5", "A4", "Letter"]) {
    test(`${template} ${size}: a cut-off note is clipped inside its box in the PDF`, async () => {
      const page = await browser.newPage();
      await serveBook(page, makeBook({ size, notes: 2, template: template === "basic" ? null : template, cutOff: true }));
      await open(page);
      assert.ok((await page.locator("[data-fit=overflow]").count()) >= 1, "the 400-line message is cut off at the smallest font");
      const boxes = await fitBoxes(page);
      const file = path.join(tmp, `cutoff-${template}-${size}.pdf`);
      await savePdf(page, file);
      await page.close();
      const pdf = words(file);
      assert.equal(pdf.length, boxes.length, "one PDF page per sheet");
      // pdftotext on the CI runner may cut a word before its last letter ("Dòn g"), hence the shortened forms
      for (const [marker, mark] of [["Dòng", new Set(["Dòng", "Dòn"])], ["Một đoạn", new Set(["đoạn", "dài", "xuống", "xuốn"])]]) {
        let seen = 0;
        for (const [p, { w, h, words: ws }] of pdf.entries()) {
          const box = boxes[p].find((b) => b.text.includes(marker));
          if (!box) continue;
          for (const x of ws.filter((x) => mark.has(x.t))) {
            seen++;
            const cx = (x.x0 + x.x1) / 2 / w, cy = (x.y0 + x.y1) / 2 / h; // the centre of the word's box: a last line cut in half still counts as inside
            assert.ok(cx >= box.x0 && cx <= box.x1 && cy >= box.y0 && cy <= box.y1, `${marker}: "${x.t}" at (${cx.toFixed(3)}, ${cy.toFixed(3)}) of page ${p + 1} lies outside its box x ${box.x0.toFixed(3)}-${box.x1.toFixed(3)}, y ${box.y0.toFixed(3)}-${box.y1.toFixed(3)}`);
          }
        }
        assert.ok(seen > 0, `${marker}: its words are in the PDF; words of its pages: ${JSON.stringify(pdf.filter((_, p) => boxes[p].some((b) => b.text.includes(marker))).map((pg) => pg.words.slice(0, 40).map((x) => x.t).join(" ")))}`);
      }
    });
  }
}

// The owner's own profile text is never lost in print (T-060 QA): every owner field at its validated maximum, with and without the portrait.
// The page-2 PDF must contain the first word of each text and the name and the sign line, inside the page and inside the text's own box.
for (const template of ["navy-classic", "memory-book", "basic"]) {
  for (const size of ["A5", "A4", "Letter"]) {
    for (const portrait of [true, false]) {
      test(`${template} ${size}${portrait ? "" : ", no portrait"}: profile fields at their maximum length are all in the PDF, inside the page and their boxes`, async () => {
        const page = await browser.newPage();
        await serveBook(page, makeBook({ size, notes: 1, template: template === "basic" ? null : template, profileMax: true, portrait }));
        await open(page);
        const boxes = (await fitBoxes(page))[1]; // the profile sheet
        const file = path.join(tmp, `profile-${template}-${size}-${portrait}.pdf`);
        await savePdf(page, file);
        await page.close();
        const { w, h, words: ws } = words(file)[1];
        const want = template === "navy-classic" ? ["Hmark", "Pmark", "Qmark", "Zname", "Knick"] : ["Hmark", "Pmark", "Qmark", "Zname"]; // Knick: the nickname opens the sign line of navy-classic
        for (const marker of want) {
          // a letter-spaced capital line comes out of pdftotext one letter per word ("K N I C K"): join the letters
          const hits = ws.flatMap((x, i) => {
            for (let j = i, joined = ""; j < ws.length && joined.length < marker.length; j++) {
              joined += ws[j].t.toLowerCase();
              if (joined === marker.toLowerCase()) return [{ x0: x.x0, y0: x.y0, x1: ws[j].x1, y1: ws[j].y1 }];
              if (!marker.toLowerCase().startsWith(joined)) break;
            }
            return [];
          });
          assert.ok(hits.length > 0, `${marker} is missing from the profile page of the PDF`);
          const box = boxes.find((b) => b.text.toLowerCase().includes(marker.toLowerCase()));
          for (const x of hits) {
            const cx = (x.x0 + x.x1) / 2 / w, cy = (x.y0 + x.y1) / 2 / h;
            assert.ok(cx > 0 && cx < 1 && cy > 0 && cy < 1, `${marker} at (${cx.toFixed(3)}, ${cy.toFixed(3)}) is off the page`);
            if (box) assert.ok(cx >= box.x0 && cx <= box.x1 && cy >= box.y0 && cy <= box.y1, `${marker} at (${cx.toFixed(3)}, ${cy.toFixed(3)}) lies outside its box x ${box.x0.toFixed(3)}-${box.x1.toFixed(3)}, y ${box.y0.toFixed(3)}-${box.y1.toFixed(3)}`);
          }
        }
      });
    }
  }
}

// Signatures in the two designed templates (T-097): a maximal, a tiny, no and a normal signature in turn, and the 2000-character note with a maximal one.
// In the print layout the signature box never meets a text box, the PDF keeps its page count and size, the message text is still extractable and none of
// its words lies in the signature box, the page has ink inside the box (also with "background graphics" off), and the box has the size the spec gives.
const MM = { A5: [148, 210], A4: [210, 297], Letter: [215.9, 279.4] };
const NOTES_PER_PAGE = { "memory-book": 1, "navy-classic": 4 };
const SIG_MM = { "memory-book": [40, 16], "navy-classic": [24, 9] }; // at most, on A5; the design px scale with the page width like the other measures
for (const template of ["memory-book", "navy-classic"]) {
  for (const size of ["A5", "A4", "Letter"]) {
    test(`${template} ${size}: signatures do not change the page count, never overlap text and print as ink`, async () => {
      const notes = 8;
      const book = makeBook({ size, notes, template, longNote: 4, signatures: true });
      const page = await browser.newPage();
      await serveBook(page, book);
      await open(page);
      // The two longest messages (1300 and 2000 characters) may stay cut off at the font floor, as they can without a signature (the print screen
      // lists them); a name or any other line must not be.
      const cut = await page.locator("[data-fit=overflow]").evaluateAll((es) => es.map((e) => e.className));
      assert.ok(cut.every((c) => /\b(mb-lines|nc-msg)\b/.test(c)), `only messages may be cut off, got ${cut.join(" | ")}`);
      const sheets = await signatureBoxes(page);
      const file = path.join(tmp, `sig-${template}-${size}.pdf`);
      await savePdf(page, file);
      const noBg = path.join(tmp, `sig-${template}-${size}-nobg.pdf`);
      await page.pdf({ path: noBg, preferCSSPageSize: true, printBackground: false });
      await page.close();

      const info = inspect(file);
      assert.equal(info.pages, 3 + Math.ceil(notes / NOTES_PER_PAGE[template]), "the signatures add no page");
      for (const [i, want] of PT[size].entries()) assert.ok(Math.abs(info.sizePt[i] - want) < 1, `${size} page size ${info.sizePt} vs ${PT[size]}`);
      assert.equal(sheets.length, info.pages);
      const signed = sheets.flatMap((s, p) => s.sigs.map((sig) => ({ p, sig, texts: s.texts })));
      assert.equal(signed.length, book.notes.filter((n) => n.signature).length, "one drawn signature per signed note");
      assert.equal(signed.length, 6, "3 of every 4 fixture notes are signed");

      const pdfWords = words(file);
      const [wmm, hmm] = MM[size];
      for (const { p, sig, texts } of signed) {
        // the size: the template's box (scaled with the page width), never larger than the spec's maximum on A5 scaled to this page
        const wm = (sig.x1 - sig.x0) * wmm, hm = (sig.y1 - sig.y0) * hmm;
        assert.ok(Math.abs(wm / hm - 2.5) < 0.05, `the box keeps the 5:2 drawing space, got ${wm.toFixed(1)} x ${hm.toFixed(1)} mm`);
        const scale = wmm / MM.A5[0];
        assert.ok(wm <= SIG_MM[template][0] * scale + 0.5 && hm <= SIG_MM[template][1] * scale + 0.5, `page ${p + 1}: ${wm.toFixed(1)} x ${hm.toFixed(1)} mm exceeds the maximum of ${SIG_MM[template].join(" x ")} mm on A5 scaled to ${size}`);
        assert.ok(sig.x0 >= 0 && sig.y0 >= 0 && sig.x1 <= 1 && sig.y1 <= 1, "inside its page");
        // a text box is apart from the signature, or (memory-book: the signature floats inside its message box, the text wraps beside it) contains it;
        // that no word lies in the signature box is the check on the PDF words below
        for (const t of texts) {
          const apart = t.x1 <= sig.x0 + 0.001 || t.x0 >= sig.x1 - 0.001 || t.y1 <= sig.y0 + 0.001 || t.y0 >= sig.y1 - 0.001;
          const holds = template === "memory-book" && t.x0 <= sig.x0 + 0.001 && t.x1 >= sig.x1 - 0.001 && t.y0 <= sig.y0 + 0.001 && t.y1 >= sig.y1 - 0.001;
          assert.ok(apart || holds, `page ${p + 1}: the text box "${t.text.slice(0, 20)}" overlaps the signature box`);
        }
        // the words of the PDF
        const { w, h, words: ws } = pdfWords[p];
        for (const x of ws) {
          const cx = (x.x0 + x.x1) / 2 / w, cy = (x.y0 + x.y1) / 2 / h;
          assert.ok(!(cx > sig.x0 && cx < sig.x1 && cy > sig.y0 && cy < sig.y1), `page ${p + 1}: the word "${x.t}" lies in the signature box`);
        }
        // ink: the page has dark pixels inside the box, with backgrounds and without
        assert.ok(inkIn(raster(file, p + 1), sig) >= 3, `page ${p + 1}: no ink in the signature box`);
        assert.ok(inkIn(raster(noBg, p + 1), sig) >= 3, `page ${p + 1}: no ink in the signature box with background graphics off`);
      }
      // the maximal signature on the 2000-character note (note 5: the 5th note is signed with the maximal one, the 1st of its cycle)
      const r = text(file);
      for (const s of ["Nguyễn Thị Hồng Ánh", "Chúc bạn thi đỗ", "Mình nhớ những buổi"]) assert.ok(r.includes(s), `extracted text contains "${s}"`);
      assert.ok(r.includes("😄"), "emoji are text");
    });
  }
}

// T-097 rework: a signature must not push ordinary text to the font floor.
// Memory-book: a message that fits above the 50 % floor without a signature still fits with one, and its font is at most 1 px (two 0.5 px fit steps) smaller.
// The lengths are ordinary (a few lines to a full page); the fixture is the default friend and the heavy one (the four optional fields at 200 characters).
const SENTENCE = "Chuyến dã ngoại ở Đà Lạt là kỷ niệm đẹp nhất của mình, cảm ơn bạn vì đã luôn ở bên cạnh nhé. ";
const sentence = (n) => SENTENCE.repeat(Math.ceil(n / SENTENCE.length)).slice(0, n).trim();
// the template's own font sizes (the fit overwrites the inline size): 20 px in the pastel boxes (.mb-lines), 18 px for the one-line fields
const BASE_PX = (x) => (x.cls.includes("mb-lines") ? 20 : 18);
async function fitsOf(book) {
  const page = await browser.newPage();
  await serveBook(page, book);
  await open(page);
  const r = await page.evaluate(() =>
    [...document.querySelectorAll(".bk-fit")].map((e) => ({ cls: e.className, fit: e.dataset.fit, px: parseFloat(getComputedStyle(e).fontSize) })),
  );
  await page.close();
  return r;
}
for (const size of ["A5", "A4", "Letter"]) {
  for (const heavy of [false, true]) {
    test(`memory-book ${size}${heavy ? " (four long optional fields)" : ""}: a signature keeps ordinary messages off the font floor`, async () => {
      const lengths = heavy ? [100, 300, 500, 700] : [100, 300, 600, 900];
      const fits = (L, sign) => {
        const book = makeBook({ size, notes: 1, template: "memory-book" });
        const n = book.notes[0];
        n.photo_ids = [];
        if (heavy) for (const f of ["how_we_met", "first_impression", "best_memory", "wish"]) n.answers[f] = sentence(200);
        n.answers.message = sentence(L);
        n.signature = sign ? SIGNATURES.maximal : null;
        return fitsOf(book);
      };
      for (const L of lengths) {
        const [p, s] = [await fits(L, false), await fits(L, true)];
        assert.equal(s.length, p.length, "the same text boxes with and without a signature");
        // a box that is clear of the font floor (1.5 px) without a signature is not cut off with one (a box already at the floor without one, in the
        // heaviest fixture, is not the signature's doing and its fit varies a little with the machine's text rendering)
        p.forEach((x, i) => {
          if (x.fit === "overflow" || x.px <= BASE_PX(x) * 0.5 + 1.5) return;
          assert.notEqual(s[i].fit, "overflow", `${L} characters: a signature cuts off ${s[i].cls} (${x.px} px without one)`);
        });
        const msg = (r) => r.findLast((x) => x.cls.includes("mb-lines")).px;
        assert.ok(msg(p) - msg(s) <= 1, `${L} characters: the message font drops from ${msg(p)} to ${msg(s)} px with a signature`);
      }
    });
  }
}

// Navy-classic: names up to the form's limit (60 characters) wrap beside the signature instead of being cut off.
for (const size of ["A5", "A4", "Letter"]) {
  test(`navy-classic ${size}: a 60-character name is not cut off beside a signature`, async () => {
    const book = makeBook({ size, notes: 4, template: "navy-classic", signatures: true });
    for (const n of book.notes) n.answers.name = "Nguyễn Thị Hồng Ánh Tuyết Mai Lan Phương Thảo Vy Quỳnh Giang".slice(0, 60);
    assert.equal(book.notes[0].answers.name.length, 60);
    const r = await fitsOf(book);
    const names = r.filter((x) => x.cls.includes("nc-signature") && !x.cls.includes("nc-sign-line"));
    assert.ok(names.length >= 4, "the four names are on the page");
    assert.deepEqual(names.filter((x) => x.fit === "overflow"), [], "no name is cut off");
    assert.ok(names.every((x) => x.px >= 9), "no name is set below 9 px");
  });
}

test("a book cut at 300 notes warns on screen and prints only the book", async () => {
  const r = await render(makeBook({ notes: 5, truncatedOf: 350 }), "truncated");
  assert.match(r.screenText, /Only 5 of 350 approved notes are included/);
  assert.equal(r.info.pages, 3 + 2);
  assert.ok(!r.t.includes("approved notes are included"), "the warning is not in the PDF");
});

test("a book with no notes still prints cover, profile and back", async () => {
  const r = await render(makeBook({ notes: 0 }), "empty");
  assert.equal(r.info.pages, 3);
});

// T-130: the class book. 30 students in three designs, 29 profile photos and the notes' photos, front and back class pages, printed on A5 and on A4.
// The routes serve one class-book answer; the personal book above is untouched.
describe("class book: 30 students, 77 pages", () => {
  for (const size of ["A5", "A4"]) {
    test(`${size}: every page there, in order, none blank, every photo loaded at ready, nothing cut off but what the screen lists`, async () => {
      const cb = makeClassBook();
      const want = classBookPages(cb);
      assert.ok(want <= 120, `the fixture is within the 120 page budget (${want})`);
      const page = await browser.newPage();
      const seen = await serveBook(page, null, cb);
      const opened = await openClassBook(page, size);
      assert.equal(opened.pages, want, "the ready signal carries the page count");

      // every photo of the pages has loaded when the book says it is ready (nothing scrolled, no print event)
      const imgs = await page.evaluate(() => [...document.querySelectorAll("img.bk-photo")].map((i) => ({ src: i.hasAttribute("src"), w: i.naturalWidth })));
      assert.ok(imgs.length >= 29, `${imgs.length} photos on the pages`);
      assert.equal(imgs.filter((i) => i.src && i.w > 0).length, imgs.length, "every photo has loaded at ready");
      assert.ok(seen.media <= 60, `${seen.media} media requests for 30 files`);
      assert.equal(seen.classBook, 1, "the class book comes from one request");
      assert.equal(await page.locator("button", { hasText: /Print or save as PDF/ }).isEnabled(), true, "Print is enabled at ready");

      // the screen lists exactly who is cut: the 41-note student, the note too long for its box, and the student with no page
      const ui = await page.locator("div.bk-ui").innerText();
      assert.ok(ui.includes(`Ready to print: ${want} pages.`), "the page count is on the screen");
      for (const i of [CLASS_BOOK_CASES.limit, CLASS_BOOK_CASES.tooLong, CLASS_BOOK_CASES.noPage]) assert.ok(ui.includes(studentName(i)), `${studentName(i)} is in the summary`);
      assert.match(ui, new RegExp(`${studentName(CLASS_BOOK_CASES.limit)}: Only 40 of 41 approved notes`));
      assert.match(ui, new RegExp(`${studentName(CLASS_BOOK_CASES.tooLong)}: Notes too long for their space and cut off: 1`));
      assert.equal(await page.locator("[data-fit=overflow]").count(), 1, "one text box is cut off, the one in the summary");
      const outside = await page.evaluate(() =>
        [...document.querySelectorAll(".bk-fit")].filter((e) => {
          const r = e.getBoundingClientRect(), s = e.closest(".bk-sheet").getBoundingClientRect();
          return r.left < s.left - 1 || r.right > s.right + 1 || r.top < s.top - 1 || r.bottom > s.bottom + 1;
        }).length,
      );
      assert.equal(outside, 0, "every text box is inside its page");

      const file = path.join(tmp, `class-${size}.pdf`);
      const saved = await savePdf(page, file);
      await page.close();
      console.log(`# class book ${size}: ${want} pages, ready ${opened.readyMs} ms, pdf ${saved.pdfMs} ms, ${(saved.bytes / 1e6).toFixed(1)} MB`);
      const info = inspect(file);
      assert.equal(info.pages, want, "the PDF has every page");
      for (const [i, w] of PT[size].entries()) assert.ok(Math.abs(info.sizePt[i] - w) < 1, `${size} page size ${info.sizePt} vs ${PT[size]}`);
      assert.ok(info.allFontsEmbedded, "every font is embedded");
      assert.ok(info.photos >= 29, `${info.photos} photos in the PDF`);

      // no blank page, and the order: cover, front page, students in the order set, back page, back cover
      const pdf = words(file);
      pdf.forEach((p, n) => assert.ok(p.words.length >= 3, `page ${n + 1} is not blank (${p.words.length} words)`));
      const pages = text(file).split("\f").slice(0, want).map((t) => squash(t).toLowerCase());
      const at = (needle, from = 0) => pages.findIndex((t, n) => n >= from && t.includes(squash(needle).toLowerCase()));
      assert.equal(at("Lớp 12A1 Chu Văn An"), 0, "the cover is first");
      assert.equal(at("Lời chào của cả lớp"), 1, "the front class page follows");
      assert.equal(at("Thầy cô chủ nhiệm"), want - 2, "the back class page is second to last");
      assert.equal(at("Cảm ơn bạn đã là một phần"), want - 1, "the back cover is last");
      let from = 2;
      for (const [i, m] of cb.members.entries()) {
        if (!m.linked) {
          assert.equal(at(studentName(i)), -1, "a student without a page is not printed");
          continue;
        }
        const n = at(studentName(i), from);
        assert.ok(n >= from && n < want - 2, `${studentName(i)} is in the PDF after the previous student (page ${n + 1})`);
        from = n;
      }
    });
  }
});

// T-146: the class book applies each student's own design to their pages only. Six students with different themes, font sets, hidden and moved sections on A5 and A4:
// the page count and order, no blank page, the cover and class pages in the class look, the hidden profile's photo never fetched, photos eager (loaded at ready) and at most 16 in flight.
describe("class book with each student's own design: 6 students (T-146)", () => {
  for (const size of ["A5", "A4"]) {
    test(`${size}: pages, order, looks, no blank page, photos eager and at most 16 in flight`, async () => {
      const cb = makeDesignClassBook();
      const want = classBookPages(cb);
      const page = await browser.newPage();
      const seen = await serveBook(page, null, cb, { mediaDelayMs: 40 });
      const opened = await openClassBook(page, size);
      assert.equal(opened.pages, want, "the ready signal carries the page count");

      const imgs = await page.evaluate(() => [...document.querySelectorAll("img.bk-photo")].map((i) => ({ src: i.hasAttribute("src"), w: i.naturalWidth })));
      assert.equal(imgs.filter((i) => i.src && i.w > 0).length, imgs.length, "every photo has loaded at ready (eager)");
      assert.ok(seen.maxInFlight <= 16, `${seen.maxInFlight} photos in flight`);
      for (const id of HIDDEN_PHOTOS) assert.ok(!seen.ids.has(id), "the photo of a hidden profile is not fetched");

      // each student's pages carry their own theme and fonts; the class cover, class pages and back cover carry none
      const looks = await page.evaluate(() => [...document.querySelectorAll("[data-theme]")].map((e) => `${e.dataset.theme}/${e.dataset.fonts}`));
      // pages per look: student 0 two notes pages (profile hidden), 1 one notes page (profile hidden), 3 profile + one notes page, 5 profile + two notes pages
      const perLook = looks.reduce((a, l) => ({ ...a, [l]: (a[l] ?? 0) + 1 }), {});
      assert.deepEqual(perLook, { "lavender/classic": 2, "burgundy/modern": 1, "forest/classic": 2, "mint/playful": 3 }, "each student's pages have their own look");
      const classLook = await page.evaluate(() => [...document.querySelectorAll('[data-page^="class-"] [data-theme], [data-page^="class-"][data-theme]')].length);
      assert.equal(classLook, 0, "the class cover and class pages keep the class look");
      assert.equal(await page.locator("[data-fit=overflow]").count(), 0, "nothing is cut off");

      const file = path.join(tmp, `class-designs-${size}.pdf`);
      await savePdf(page, file);
      await page.close();
      const info = inspect(file);
      assert.equal(info.pages, want, "the PDF has every page");
      for (const [i, w] of PT[size].entries()) assert.ok(Math.abs(info.sizePt[i] - w) < 1, `${size} page size ${info.sizePt} vs ${PT[size]}`);
      assert.ok(info.allFontsEmbedded, "every font is embedded");
      const pdf = words(file);
      pdf.forEach((p, n) => assert.ok(p.words.length >= 3, `page ${n + 1} is not blank (${p.words.length} words)`));
      const pages = text(file).split("\f").slice(0, want).map((t) => squash(t).toLowerCase());
      const at = (needle, from = 0) => pages.findIndex((t, n) => n >= from && t.includes(squash(needle).toLowerCase()));
      assert.equal(at("Lớp 12A1 Chu Văn An"), 0, "the cover is first");
      assert.equal(at("Cảm ơn bạn đã là một phần"), want - 1, "the back cover is last");
      // the students in order; the profile quote is on the pages of the four students whose design shows the profile
      let from = 2;
      for (let i = 0; i < cb.members.length; i++) {
        const hidden = cb.members[i].design?.sections.hidden.includes("profile");
        const n = hidden ? at("Chúc mừng", from) : at(studentName(i), from);
        if (!hidden) assert.ok(n >= from, `${studentName(i)} is in the PDF after the previous student`);
        if (!hidden) from = n;
      }
      const quote = pages.filter((t) => t.includes(squash("Hãy sống như thể hôm nay là ngày cuối cùng.").toLowerCase())).length;
      assert.equal(quote, 4, "the profile page is printed for the four students who show it, not for the two who hide it");
      assert.ok(pages.some((t) => t.includes(squash(DESIGN_TITLE).toLowerCase())), "the student's own title is printed");
    });
  }
});
