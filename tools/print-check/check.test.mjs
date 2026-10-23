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
import { makeBook, photoId, SIGNATURES } from "./fixture.mjs";
import { BASE, fitBoxes, inkIn, inspect, open, raster, savePdf, serveBook, signatureBoxes, startServer, text, words } from "./lib.mjs";

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
async function fitsOf(book) {
  const page = await browser.newPage();
  await serveBook(page, book);
  await open(page);
  const r = await page.evaluate(() =>
    [...document.querySelectorAll(".bk-fit")].map((e) => ({ cls: e.className, fit: e.dataset.fit, px: parseFloat(getComputedStyle(e).fontSize), base: parseFloat(e.style.fontSize) })),
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
          if (x.fit === "overflow" || x.px <= x.base * 0.5 + 1.5) return;
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
