// The shipped page templates of Navy Classic (internal/templates/embed/pagetemplates/navy-classic/*.json), read from disk for the print check and the contact sheet (T-178).
import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { ROOT } from "./lib.mjs";

export const KINDS = ["cover", "profile", "notes", "back"];
export const NAME_LONG = "Nguyễn Thị Phương Anh";
/** The pages of the PDF of the sample book, per page kind: cover, profile, three notes pages (nine notes, four to a page), back. */
export const PDF_PAGE = { cover: [1], profile: [2], notes: [3, 4, 5], back: [6] };

/** Every template of the folder: { id, kind, label_en, label_vi, page }, in kind order, then by id. */
export function shippedTemplates() {
  const dir = path.join(ROOT, "internal/templates/embed/pagetemplates/navy-classic");
  return readdirSync(dir)
    .filter((f) => f.endsWith(".json"))
    .map((f) => JSON.parse(readFileSync(path.join(dir, f), "utf8")))
    .sort((a, b) => KINDS.indexOf(a.kind) - KINDS.indexOf(b.kind) || a.id.localeCompare(b.id));
}

/** A layout document that draws the given pages: { cover: page, ... }. */
export const layoutOf = (pages) => ({ v: 1, pages });

/** The sample book of the contact sheet and the print case: a long Vietnamese name, a photo on the profile, nine notes (the second one long), signatures. */
export function sampleBook(makeBook, size, { signatures = false } = {}) {
  const book = makeBook({ size, notes: 9, template: "navy-classic", longNote: 1, signatures });
  book.profile.full_name = NAME_LONG;
  return book;
}

/**
 * Runs in the page (it is passed to the browser as source, so it uses nothing outside itself). For one sheet: what a decoration (a layout item) must stay off, and what it must stay inside.
 * Off: every text of the page outside the layout layer (the boxes of its glyph lines), the signatures, the template's own ornaments (the dividers and the cap), the autograph grid,
 * the letter column and the footer of the profile. Inside: the inner rule of the gold frame, with 4 px to spare, on the pages that have one. `bad` lists what breaks a rule.
 */
export function covers(sheet) {
  const s = sheet.getBoundingClientRect();
  const hit = (a, b) => a.l < b.r && b.l < a.r && a.t < b.b && b.t < a.b;
  const box = (r) => ({ l: r.left, r: r.right, t: r.top, b: r.bottom });
  const f = s.width / 816; // design px to CSS px of this sheet
  const obstacles = [];
  const walker = document.createTreeWalker(sheet, NodeFilter.SHOW_TEXT);
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    if (!n.textContent.trim() || n.parentElement.closest(".nc-layer, [data-layout-item]")) continue;
    const range = document.createRange();
    range.selectNodeContents(n);
    for (const r of range.getClientRects()) if (r.width > 1 && r.height > 1) obstacles.push({ what: n.textContent.trim().slice(0, 24), box: box(r) });
  }
  for (const g of sheet.querySelectorAll("svg.bk-sig")) obstacles.push({ what: "signature", box: box(g.getBoundingClientRect()) });
  for (const g of sheet.querySelectorAll("svg")) if (!g.closest(".nc-layer, [data-layout-item]") && !g.classList.contains("bk-sig")) obstacles.push({ what: "ornament", box: box(g.getBoundingClientRect()) });
  for (const [sel, what] of [[".nc-grid", "the autograph grid"], [".nc-letter-text", "the letter"], [".nc-footer", "the footer"]]) {
    const e = sheet.querySelector(sel);
    if (e) obstacles.push({ what, box: box(e.getBoundingClientRect()) });
  }
  const frames = [...sheet.querySelectorAll(".nc-frame")].map((e) => box(e.getBoundingClientRect()));
  const inner = frames.length ? frames.reduce((a, b) => (b.r - b.l < a.r - a.l ? b : a)) : null;
  const bad = [];
  for (const it of sheet.querySelectorAll("[data-layout-item]")) {
    // the painted part of the asset (the shapes inside its svg, rotated with the box), not the whole box: a tape fills 80 x 50 % of it
    const parts = [...it.querySelectorAll("path, polygon, polyline, rect, circle, ellipse, line, image")].map((e) => e.getBoundingClientRect()).filter((r) => r.width > 0 && r.height > 0);
    const a = parts.length ? { l: Math.min(...parts.map((r) => r.left)), r: Math.max(...parts.map((r) => r.right)), t: Math.min(...parts.map((r) => r.top)), b: Math.max(...parts.map((r) => r.bottom)) } : box(it.getBoundingClientRect());
    for (const o of obstacles) if (hit(a, o.box)) bad.push(`${it.dataset.layoutItem} covers ${o.what}`);
    if (inner && (a.l < inner.l + 4 * f || a.r > inner.r - 4 * f || a.t < inner.t + 4 * f || a.b > inner.b - 4 * f)) bad.push(`${it.dataset.layoutItem} crosses the inner rule of the frame`);
  }
  return { bad, page: { w: s.width, h: s.height } };
}
