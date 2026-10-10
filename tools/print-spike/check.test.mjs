// Automated check of spike T-054 (AC3): headless Chromium page.pdf() of the prototype, then page count, page size,
// embedded fonts, text extraction (Vietnamese) and photo resolution are asserted on the PDF itself with poppler.
//   cd tools/print-spike && npm install && npx playwright install chromium && npm test      (needs pdfinfo, pdffonts, pdfimages, pdftotext)
import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, before, describe, test } from "node:test";
import { chromium } from "playwright";
import { inspect, open, savePdf, startServer, text } from "./lib.mjs";

const PT = { a5: [419.53, 595.28], a4: [595.28, 841.89], letter: [612, 792] }; // exact sizes in points
const tmp = mkdtempSync(path.join(os.tmpdir(), "print-spike-test-"));
let stop, browser;
before(async () => {
  stop = await startServer();
  browser = await chromium.launch();
});
after(async () => {
  await browser?.close();
  stop?.();
});

async function render(design, size, lang = "vi") {
  const page = await browser.newPage();
  const hosts = new Set();
  page.on("request", (r) => hosts.add(new URL(r.url()).host));
  const opened = await open(page, design, size, lang);
  const file = path.join(tmp, `${design}-${size}-${lang}.pdf`);
  await savePdf(page, file);
  await page.close();
  return { opened, hosts, file, info: inspect(file), t: text(file) };
}

describe("24-page memory book, 30 photos", () => {
  for (const size of ["a5", "a4", "letter"]) {
    test(`${size}: pages, size, fonts, text, photos`, async () => {
      const { opened, hosts, info, t } = await render("memory", size);
      assert.equal(info.pages, 24);
      assert.equal(opened.pages, 24);
      for (const [i, want] of PT[size].entries())
        assert.ok(
          Math.abs(info.sizePt[i] - want) < 1,
          `${size} page size ${info.sizePt} vs ${PT[size]}`,
        );
      assert.ok(info.allFontsEmbedded, "every font is embedded");
      assert.equal(info.photos, 30, "30 photos");
      assert.ok(
        info.ppiMin >= 300,
        `photos at least 300 ppi, got ${info.ppiMin}`,
      );
      assert.deepEqual(
        [...hosts],
        ["localhost:5199"],
        "no request to any other host (no Google Fonts)",
      );
      // The one deliberately huge note (1,496 characters) does not fit its box even at 50 % of the font size: it is clipped and flagged.
      assert.equal(opened.fit.overflow, 1, "only the stress note overflows");
      // Vietnamese text is extractable and exact (NFC), emoji survive as text
      for (const s of [
        "Nguyễn Thị Hồng Ánh",
        "Đặng Ngọc Bích Trâm",
        "Huỳnh Ngọc Diệp",
        "Chuyến dã ngoại ở Đà Lạt",
      ])
        assert.ok(t.includes(s), `extracted text contains "${s}"`);
      // Titles in Fredoka fall back to Nunito for the diacritics Fredoka lacks; poppler then inserts a space at the font
      // switch ("ngườ i"), so these are compared without whitespace. Copy and search in a PDF reader are affected alike (ADR 0003).
      const squash = (x) => x.replace(/\s+/g, "");
      for (const s of ["Từ một người bạn", "Giữ liên lạc nhé"])
        assert.ok(
          squash(t).includes(squash(s)),
          `extracted text contains "${s}" (ignoring spaces)`,
        );
      assert.ok(t.includes("😄"));
    });
  }
});

test("navy classic: cover, letter and paginated autographs on A5", async () => {
  const { info, t } = await render("classic", "a5");
  assert.equal(info.pages, 4);
  assert.ok(info.allFontsEmbedded);
  for (const s of [
    "Gửi các bạn khóa 2026",
    "Lưu bút",
    "Tạm biệt khóa 2026",
    "NGUYỄN THỊ HỒNG ÁNH",
  ])
    assert.ok(t.includes(s), s);
});
