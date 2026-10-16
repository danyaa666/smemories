// Automated print check of the book renderer (T-058, AC6): the book screen is opened in headless Chromium with a fixture book,
// printed with page.pdf() (what "Save as PDF" does), and the PDF itself is inspected with poppler.
//   make print-check        (or: cd tools/print-check && npm ci && npx playwright install chromium && npm test)
// Needs pdfinfo, pdffonts, pdfimages and pdftotext (poppler) on the PATH.
import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, before, describe, test } from "node:test";
import { chromium } from "playwright";
import { makeBook } from "./fixture.mjs";
import { BASE, inspect, open, savePdf, serveBook, startServer, text } from "./lib.mjs";

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
  page.on("request", (r) => hosts.add(new URL(r.url()).host));
  const seen = await serveBook(page, book);
  const opened = await open(page);
  const screenText = await page.locator("main").innerText();
  const file = path.join(tmp, `${name}.pdf`);
  const saved = await savePdf(page, file);
  const shrunk = await page.locator("[data-fit=shrunk]").count();
  await page.close();
  return { opened, saved, hosts, seen, shrunk, screenText, file, info: inspect(file), t: text(file) };
}

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
        assert.ok(r.t.includes(s), `extracted text contains "${s}"`);
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
        assert.ok(r.t.includes(s), `extracted text contains "${s}"`);
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
