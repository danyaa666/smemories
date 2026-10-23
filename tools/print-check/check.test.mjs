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
import { makeBook, photoId } from "./fixture.mjs";
import { BASE, fitBoxes, inspect, open, savePdf, serveBook, startServer, text, words } from "./lib.mjs";

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
      for (const [marker, mark] of [["Dòng", new Set(["Dòng", "Dòn"])], ["Một đoạn", new Set(["đoạn", "dài", "xuống", "xuốn"])]] // pdftotext on the CI runner may cut a word before its last letter ("Dòn g")) {
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
