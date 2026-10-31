// Question 3: Vietnamese input, plain paste, rune limit, line breaks and "editor shows what prints", for contenteditable and textarea.
//   node scripts/text.mjs (needs `npm run dev`) -> results/text.json
import { writeFileSync } from "node:fs";
import { BASE, diffPng, launch, round } from "./lib.mjs";

const TELEX_VIET = ["V", "Vi", "Vie", "Viê", "Việ", "Việt"]; // typed V i e e j t
const TELEX_NAM = ["N", "Na", "Nam"];
const VNI_DUONG = ["đ", "đu", "đư", "đươ", "đươn", "đương", "đường"]; // d9 u7 o7 n g 2
const SHORT = "Việt Nam";
const MEDIUM = "Tôi yêu những buổi chiều tan học, cùng các bạn đi dưới hàng phượng đỏ rực cả một góc trời.";
const LONG = "Mười hai năm đèn sách, những ngày mưa đường trơn trượt, những buổi trưa hè ve kêu râm ran, tiếng trống trường giòn giã mỗi sáng, cô giáo chủ nhiệm cười hiền, bạn bè chia nhau từng cái kẹo, từng trang vở, từng nỗi lo thi cử. ".repeat(2);
const FULL = Array.from("Việt Nam đường về nhà 😀 ".repeat(40)).slice(0, 500).join("");

async function compose(page, cdp, steps, commit) {
  for (const s of steps) await cdp.send("Input.imeSetComposition", { text: s, selectionStart: s.length, selectionEnd: s.length });
  await cdp.send("Input.insertText", { text: commit });
}
const sel = (tech) => `[data-edit=${tech}]`;
const out = {};
for (const tech of ["ce", "ta"]) {
  const r = (out[tech] = {});
  // IME proof
  {
    const { browser, ctx, page } = await launch();
    await page.goto(`${BASE}/?view=text&tech=${tech}&mode=edit&text=`);
    await page.waitForSelector(sel(tech)); await page.waitForTimeout(400);
    const cdp = await ctx.newCDPSession(page);
    await compose(page, cdp, TELEX_VIET, "Việt"); await page.keyboard.type(" "); await compose(page, cdp, TELEX_NAM, "Nam"); await page.keyboard.type(" ");
    await compose(page, cdp, VNI_DUONG, "đường");
    const text = await page.evaluate(() => window.__text());
    const ev = await page.evaluate(() => window.__events());
    r.ime = { typed: "Viet(Telex) Nam(Telex) duong(VNI-like steps)", value: text, ok: text === "Việt Nam đường", compositionstart: ev.filter((e) => e.startsWith("compositionstart")).length, compositionupdate: ev.filter((e) => e.startsWith("compositionupdate")).length, compositionend: ev.filter((e) => e.startsWith("compositionend")).length };
    // caret in the middle: put the caret after "Việt", compose "đẹp " there
    await page.evaluate((t) => { const el = document.querySelector(t); el.focus(); if (el.setSelectionRange) el.setSelectionRange(4, 4); else { const rg = document.createRange(); rg.setStart(el.firstChild, 4); rg.collapse(true); getSelection().removeAllRanges(); getSelection().addRange(rg); } }, sel(tech));
    await compose(page, cdp, ["đ", "đẹ", "đẹp"], "đẹp");
    r.ime_caret_in_middle = await page.evaluate(() => window.__text());
    // Enter makes a line break
    await page.keyboard.press("Enter");
    r.enter_value_has_newline = (await page.evaluate(() => window.__text())).includes("\n");
    await browser.close();
  }
  // paste as plain text + the rune limit
  {
    const { browser, page } = await launch();
    await page.goto(`${BASE}/?view=text&tech=${tech}&mode=edit&text=`);
    await page.waitForSelector(sel(tech)); await page.waitForTimeout(400);
    const paste = (plain, html) => page.evaluate(([s, p, h]) => {
      const dt = new DataTransfer(); dt.setData("text/plain", p); if (h) dt.setData("text/html", h);
      document.querySelector(s).focus();
      document.querySelector(s).dispatchEvent(new ClipboardEvent("paste", { clipboardData: dt, bubbles: true, cancelable: true }));
    }, [sel(tech), plain, html]);
    await paste("dòng một\r\ndòng hai", "<b style='font-size:99px'>HTML</b>");
    r.paste_plain = await page.evaluate(() => window.__text());
    r.paste_no_markup_in_dom = await page.evaluate((s) => !document.querySelector(s).innerHTML.includes("<b"), sel(tech));
    await paste("😀".repeat(600));
    const v = await page.evaluate(() => window.__text());
    r.paste_600_emoji_runes_kept = Array.from(v).length;
    r.paste_600_emoji_utf16_units = v.length;
    await browser.close();
  }
  // parity: the editor box against the printed (view) box at three texts and two rotations
  r.parity = {};
  for (const [name, text] of Object.entries({ short: SHORT, medium: MEDIUM, long_shrinks: LONG, full_500_runes: FULL })) for (const rot of [0, -4]) {
    const { browser, page } = await launch({ deviceScaleFactor: 2 });
    const q = (mode) => `${BASE}/?view=text&tech=${tech}&mode=${mode}&r=${rot}&text=${encodeURIComponent(text)}`;
    await page.goto(q("view")); await page.waitForSelector("[data-box]"); await page.evaluate(() => document.fonts.ready); await page.waitForTimeout(500);
    const view = await page.locator("[data-box]").screenshot();
    const viewInfo = await page.evaluate(() => { const e = document.querySelector("[data-print-text]"); return { state: e.dataset.fit, px: getComputedStyle(e).fontSize, clipped: e.scrollHeight > e.clientHeight + 1 }; });
    await page.goto(q("edit")); await page.waitForSelector(sel(tech)); await page.evaluate(() => document.fonts.ready); await page.waitForTimeout(500);
    await page.addStyleTag({ content: "[data-edit]{outline:none !important}" });
    const edit = await page.locator("[data-box]").screenshot();
    const editPx = await page.evaluate((s) => getComputedStyle(document.querySelector(s)).fontSize, sel(tech));
    r.parity[`${name}@${rot}deg`] = { ...diffPng(view, edit), print_font_px: viewInfo.px, editor_font_px: editPx, print_fit_state: viewInfo.state, print_clipped_strict: viewInfo.clipped };
    await browser.close();
  }
  // mid-composition cap: 499 runes + a 3-rune composition is not cut until it is committed
  {
    const { browser, ctx, page } = await launch();
    await page.goto(`${BASE}/?view=text&tech=${tech}&mode=edit&text=${encodeURIComponent("a".repeat(499))}`);
    await page.waitForSelector(sel(tech)); await page.waitForTimeout(400);
    const cdp = await ctx.newCDPSession(page);
    await page.evaluate((s) => { const e = document.querySelector(s); e.focus(); if (e.setSelectionRange) e.setSelectionRange(499, 499); else { const rg = document.createRange(); rg.selectNodeContents(e); rg.collapse(false); getSelection().removeAllRanges(); getSelection().addRange(rg); } }, sel(tech));
    for (const s of ["V", "Vi", "Việ"]) await cdp.send("Input.imeSetComposition", { text: s, selectionStart: s.length, selectionEnd: s.length });
    r.cap_during_composition_runes = Array.from(await page.evaluate(() => window.__text())).length;
    await cdp.send("Input.insertText", { text: "Việ" });
    await page.waitForTimeout(100);
    r.cap_after_commit_runes = Array.from(await page.evaluate(() => window.__text())).length;
    await browser.close();
  }
  console.log(tech, JSON.stringify(r));
}
writeFileSync(new URL("../results/text.json", import.meta.url), JSON.stringify(out, null, 1));
