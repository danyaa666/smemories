// Print check of the shipped page templates of Navy Classic (T-178): every template, on A5, A4 and Letter, with the sample book (a photo, "Nguyễn Thị Phương Anh", nine notes of which the
// second has 2000 characters, signatures on the notes). The same parity method as "layout layer: the printed page is the page on screen" in check.test.mjs (print against screen below
// the standing threshold, each placed item within 2 px; the helpers below are copies of that file's, to be shared once the Memory Book cases of T-177 are merged), and the things a decoration
// must never do: cover a text of the page, a signature or the template's own ornaments, cross the inner rule of the gold frame, or push a text out of its box.
//   cd tools/print-check && node --test --test-name-pattern="navy" templates-navy.test.mjs        (or make print-check)
import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { after, before, describe, test } from "node:test";
import { chromium } from "playwright";
import { makeBook } from "./fixture.mjs";
import { inspect, open, raster, savePdf, serveBook, startServer, text } from "./lib.mjs";
import { KINDS, NAME_LONG, PDF_PAGE, covers, layoutOf, sampleBook, shippedTemplates } from "./navy-templates.mjs";

const squash = (x) => x.replace(/\s+/g, "");
const PT = { A5: [419.53, 595.28], A4: [595.28, 841.89], Letter: [612, 792] }; // exact sizes in points
const tmp = mkdtempSync(path.join(os.tmpdir(), "print-check-navy-"));
let stop, browser;
before(async () => {
  stop = await startServer();
  browser = await chromium.launch();
});
after(async () => {
  await browser?.close();
  stop?.();
});

describe("navy-classic page templates print as drawn (T-178)", () => {
  const DPI = 192;
  /** An 8-bit grey picture of a PNG screenshot, decoded by the browser itself (no image library in this tool). */
  async function grey(png) {
    const p = await browser.newPage();
    const r = await p.evaluate(async (b64) => {
      const img = new Image();
      img.src = `data:image/png;base64,${b64}`;
      await img.decode();
      const c = document.createElement("canvas");
      c.width = img.width;
      c.height = img.height;
      const x = c.getContext("2d");
      x.drawImage(img, 0, 0);
      const d = x.getImageData(0, 0, c.width, c.height).data;
      let out = "";
      for (let i = 0; i < d.length; i += 4 * 8192) {
        const chunk = [];
        for (let j = i; j < Math.min(d.length, i + 4 * 8192); j += 4) chunk.push(Math.round(0.299 * d[j] + 0.587 * d[j + 1] + 0.114 * d[j + 2]));
        out += String.fromCharCode(...chunk);
      }
      return { w: c.width, h: c.height, data: btoa(out) };
    }, png.toString("base64"));
    await p.close();
    return { w: r.w, h: r.h, px: Buffer.from(r.data, "base64") };
  }

  /** The share of pixels that differ by more than 48/255 from every pixel within 1 px of the same place in the other picture (a 1-px tolerant difference). */
  function difference(a, b) {
    const w = Math.min(a.w, b.w), h = Math.min(a.h, b.h);
    let bad = 0;
    const bands = new Array(20).fill(0);
    for (let y = 0; y < h; y++)
      for (let x = 0; x < w; x++) {
        const v = a.px[y * a.w + x];
        if (Math.abs(v - b.px[y * b.w + x]) <= 48) continue;
        let near = false;
        for (let dy = -1; dy <= 1 && !near; dy++)
          for (let dx = -1; dx <= 1 && !near; dx++) {
            const xx = x + dx, yy = y + dy;
            if (xx >= 0 && yy >= 0 && xx < w && yy < h && Math.abs(v - b.px[yy * b.w + xx]) <= 48) near = true;
          }
        if (!near) { bad++; bands[Math.floor((y / h) * 20)]++; }
      }
    if (process.env.PRINT_CHECK_BANDS) console.log(`# bad pixels per 5 % band: ${bands.join(" ")}`);
    return bad / (w * h);
  }

  /** Opens the book, prints it, and shows the sheets at paper size and 2x pixels, the way the PDF is rasterised. */
  async function printAndShow(book, size, name) {
    const page = await browser.newPage({ viewport: { width: 1400, height: 1900 }, deviceScaleFactor: 2 });
    const seen = await serveBook(page, book);
    const urls = [];
    page.on("request", (r) => urls.push(r.url()));
    await open(page);
    const file = path.join(tmp, `${name}.pdf`);
    await savePdf(page, file);
    await page.addStyleTag({ content: ".bk-ui { display: none !important } .bk-stage { --s: 1 !important; padding: 0 !important }" });
    return { page, file, seen, urls, size };
  }

  /** One sheet at paper size, taken at the top left corner of the window (no fractional offset from the sheets before it), like the first pixel of the PDF's raster. */
  async function sheetPicture(page, label, nth = 0) {
    const sheet = page.locator(`.bk-sheet[data-page="${label}"]`).nth(nth);
    const box = await sheet.evaluate((e) => {
      e.dataset.shot = "1";
      e.style.cssText += ";position: fixed; left: 0; top: 0; z-index: 99999; box-shadow: none";
      const r = e.getBoundingClientRect();
      return { width: r.width, height: r.height };
    });
    const png = await page.screenshot({ clip: { x: 0, y: 0, ...box }, animations: "disabled", caret: "hide" });
    await sheet.evaluate((e) => {
      delete e.dataset.shot;
      e.style.position = e.style.left = e.style.top = e.style.zIndex = e.style.boxShadow = "";
    });
    return grey(png);
  }


  const templates = shippedTemplates();
  const plainCache = {};
  test("navy: four or more templates per page kind", () => {
    for (const k of KINDS) assert.ok(templates.filter((t) => t.kind === k).length >= 4, `${k}: four or more templates`);
  });

  for (const size of ["A5", "A4", "Letter"])
    for (const tp of templates) {
      test(`navy ${size} ${tp.id}: prints as drawn (difference < 0.5 %, each item within 2 px), covers no text, signature or ornament, stays inside the frame, long names and notes still fit`, async () => {
        const mk = (stress = false) => (stress ? makeBook({ size, notes: 9, template: "navy-classic", stress: true, signatures: true }) : sampleBook(makeBook, size, { signatures: true }));
        const book = mk();
        book.layout = layoutOf({ [tp.kind]: tp.page });
        const pdfPages = PDF_PAGE[tp.kind];
        if (!plainCache[size]) {
          const plain = await printAndShow(mk(), size, `navy-tpl-base-${size}`);
          const info = inspect(plain.file);
          const diffs = {};
          for (const k of KINDS) for (const [i, n] of PDF_PAGE[k].entries()) diffs[`${k}${i}`] = difference(raster(plain.file, n, DPI), await sheetPicture(plain.page, k, i));
          const overflow = await plain.page.locator("[data-fit=overflow]").count();
          await plain.page.close();
          plainCache[size] = { pages: info.pages, diffs, overflow };
        }
        const base = plainCache[size];
        const r = await printAndShow(book, size, `navy-tpl-${tp.id}-${size}`);
        const info = inspect(r.file);
        assert.equal(info.pages, base.pages, "the same page count as the book without the template");
        assert.equal(info.pages, 6, "cover, profile, three notes pages, back");
        for (const [i, want] of PT[size].entries()) assert.ok(Math.abs(info.sizePt[i] - want) < 1, `${size} page size ${info.sizePt} vs ${PT[size]}`);
        assert.ok(info.allFontsEmbedded, "every font is embedded");
        assert.ok(squash(text(r.file)).includes(squash(NAME_LONG)), "the long Vietnamese name is in the PDF");
        assert.equal(await r.page.locator("[data-fit=overflow]").count(), base.overflow, "the template cuts no text that the plain page does not cut");
        assert.equal(await r.page.locator(`.bk-sheet[data-page="${tp.kind}"]`).count(), pdfPages.length, `${tp.kind} pages`);
        for (const [i, pdfPage] of pdfPages.entries()) {
          const pdfPic = raster(r.file, pdfPage, DPI);
          const scr = await sheetPicture(r.page, tp.kind, i);
          assert.ok(Math.abs(pdfPic.w - scr.w) <= 2 && Math.abs(pdfPic.h - scr.h) <= 2, `${pdfPic.w}x${pdfPic.h} against ${scr.w}x${scr.h}`);
          const diff = difference(pdfPic, scr);
          const b0 = base.diffs[`${tp.kind}${i}`];
          console.log(`# navy ${size} ${tp.id}#${i}: difference ${(diff * 100).toFixed(3)} % (the same page without the template: ${(b0 * 100).toFixed(3)} %)`);
          assert.ok(diff - b0 < 0.005, `${size} ${tp.id}: ${(diff * 100).toFixed(3)} % of the pixels differ, ${(b0 * 100).toFixed(3)} % without the template (it may add 0.5 %)`);
        }
        // each placed item sits where it is on screen: its box as a fraction of its sheet in the print layout against the screen layout, within 2 px of the 192 dpi picture
        const fractions = () => r.page.evaluate((kind) => [...document.querySelectorAll(`.bk-sheet[data-page="${kind}"]`)].map((sheet) => {
          const s = sheet.getBoundingClientRect();
          return [...sheet.querySelectorAll("[data-layout-item]")].map((e) => { const q = e.getBoundingClientRect(); return [e.dataset.layoutItem, (q.left - s.left) / s.width, (q.top - s.top) / s.height, (q.right - s.left) / s.width, (q.bottom - s.top) / s.height]; });
        }), tp.kind);
        const onScreen = await fractions();
        await r.page.emulateMedia({ media: "print" });
        const inPrint = await fractions();
        const pxW = (PT[size][0] / 72) * DPI, pxH = (PT[size][1] / 72) * DPI;
        assert.equal(inPrint[0].length, tp.page.items.length, "every item is drawn");
        for (const [n, sheetItems] of onScreen.entries())
          for (const [j, it] of sheetItems.entries()) {
            const q = inPrint[n][j];
            assert.equal(q[0], it[0]);
            for (const [k, scale] of [[1, pxW], [2, pxH], [3, pxW], [4, pxH]]) assert.ok(Math.abs(q[k] - it[k]) * scale <= 2, `${size} ${tp.id} item ${it[0]}: edge ${k} moves ${(Math.abs(q[k] - it[k]) * scale).toFixed(1)} px between screen and print (192 dpi)`);
          }
        await r.page.emulateMedia({ media: null });
        const hits = await r.page.locator(`.bk-sheet[data-page="${tp.kind}"]`).evaluateAll((els, src) => els.flatMap((e) => new Function(`return (${src})`)()(e).bad), covers.toString());
        assert.deepEqual(hits, [], "no decoration covers a text, a signature or an ornament of the page, or crosses the inner rule of the frame");
        await r.page.close();

        // long names and emoji everywhere (the stress book): the template adds no overflow and leaves no text outside its box
        const sbook = mk(true);
        sbook.layout = layoutOf({ [tp.kind]: tp.page });
        const p2 = await browser.newPage({ viewport: { width: 1400, height: 1900 } });
        await serveBook(p2, sbook);
        await open(p2);
        const withT = await p2.locator("[data-fit=overflow]").count();
        const outside = await p2.evaluate(() => [...document.querySelectorAll(".bk-fit")].filter((e) => { const x = e.getBoundingClientRect(), s = e.closest(".bk-sheet").getBoundingClientRect(); return x.left < s.left - 1 || x.right > s.right + 1 || x.top < s.top - 1 || x.bottom > s.bottom + 1; }).length);
        await p2.close();
        const p3 = await browser.newPage({ viewport: { width: 1400, height: 1900 } });
        await serveBook(p3, mk(true));
        await open(p3);
        const without = await p3.locator("[data-fit=overflow]").count();
        await p3.close();
        assert.equal(withT, without, "with the stress book the template cuts no more text than the plain page");
        assert.equal(outside, 0, "no text box leaves its page");
      });
    }
});
