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
