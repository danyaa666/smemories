// Sample PDFs of the designed templates (memory-book T-059, navy-classic T-060) for the owner's review (docs/templates/): invented people, synthetic photos, each under 2 MB.
//   python3 make-fixtures.py sample /tmp/photos && PRINT_CHECK_PHOTOS=/tmp/photos node make-samples.mjs [OUT_DIR]
// Same path as `make print-check`: the book screen in headless Chromium, page.pdf() with the page size from CSS and backgrounds on.
import { statSync } from "node:fs";
import path from "node:path";
import { chromium } from "playwright";
import { makeBook } from "./fixture.mjs";
import { open, ROOT, savePdf, serveBook, startServer } from "./lib.mjs";

const out = path.resolve(process.argv[2] ?? path.join(ROOT, "docs/templates"));
const SAMPLES = [
  ["memory-book-a5-vi.pdf", "A5", "vi", "memory-book"],
  ["memory-book-a4-en.pdf", "A4", "en", "memory-book"],
  ["memory-book-letter-vi.pdf", "Letter", "vi", "memory-book"],
  ["navy-classic-a5-vi.pdf", "A5", "vi", "navy-classic"],
  ["navy-classic-a4-en.pdf", "A4", "en", "navy-classic"],
  ["navy-classic-letter-vi.pdf", "Letter", "vi", "navy-classic"],
];
const stop = await startServer();
const browser = await chromium.launch();
try {
  for (const [file, size, language, template] of SAMPLES) {
    if (process.argv[3] && process.argv[3] !== template) continue; // node make-samples.mjs OUT_DIR [template]
    const page = await browser.newPage();
    const book = makeBook({ size, notes: 21, template, longNote: 4 }); // note 5 has the form's longest message (2000 characters)
    book.yearbook.language = language;
    await serveBook(page, book);
    await open(page);
    const { bytes } = await savePdf(page, path.join(out, file));
    console.log(file, (bytes / 1e6).toFixed(2), "MB");
    if (bytes >= 2e6) throw new Error(`${file} is over 2 MB`);
    await page.close();
  }
} finally {
  await browser.close();
  stop();
}
