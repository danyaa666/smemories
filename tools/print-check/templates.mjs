// The shipped page templates of a book template (internal/templates/embed/pagetemplates/<tpl>/*.json), read from disk for the print check and the contact sheet (T-177).
import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { ROOT } from "./lib.mjs";

export const KINDS = ["cover", "profile", "notes", "back"];
export const NAME_LONG = "Nguyễn Thị Phương Anh";

/** Every template of the folder: { id, kind, label_en, label_vi, page }, in kind order, then by id. */
export function shippedTemplates(tpl = "memory-book") {
  const dir = path.join(ROOT, "internal/templates/embed/pagetemplates", tpl);
  return readdirSync(dir)
    .filter((f) => f.endsWith(".json"))
    .map((f) => JSON.parse(readFileSync(path.join(dir, f), "utf8")))
    .sort((a, b) => KINDS.indexOf(a.kind) - KINDS.indexOf(b.kind) || a.id.localeCompare(b.id));
}

/** A layout document that draws the given pages: { cover: page, ... }. */
export const layoutOf = (pages) => ({ v: 1, pages });

/** The sample book of the contact sheet and the print case: a long Vietnamese name, a photo on every page, three notes (the second one long), signatures. */
export function sampleBook(makeBook, size, { signatures = false } = {}) {
  const book = makeBook({ size, notes: 3, template: "memory-book", longNote: 1, signatures });
  book.profile.full_name = NAME_LONG;
  book.yearbook.title = "Lớp 12A1 - Trường THPT Chu Văn An";
  return book;
}
