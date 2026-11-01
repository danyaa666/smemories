import { describe, expect, it } from "vitest";
import en from "./locales/en.json";
import vi from "./locales/vi.json";
import i18n from "./i18n";

// T-164 AC1: the count strings of the editor read right for 1 and 2 in both languages (i18next plural keys, no hand-built plurals).
describe("deleteImage.inUse plural", () => {
  const where = "the cover";
  it.each([
    ["en", 1, "used on 1 page (the cover)"],
    ["en", 2, "used on 2 pages (the cover)"],
    ["vi", 1, "dùng ở 1 trang (the cover)"],
    ["vi", 2, "dùng ở 2 trang (the cover)"],
  ])("%s %i", (lng, count, text) => {
    expect(i18n.getFixedT(lng)("editor.deleteImage.inUse", { count, where })).toContain(text);
  });
  it("no key of the old, singular-blind form is left", () => {
    expect("inUse" in en.editor.deleteImage).toBe(false);
    expect("inUse" in vi.editor.deleteImage).toBe(false);
  });
});

describe("every {{count}} string in the editor locale blocks is either a plural pair or reads right for 1", () => {
  // the strings reviewed for T-159..T-162 that carry {{count}} and no plural forms; each reads right with 1 ("1 found", "1 of 30 images")
  // or is only shown at its maximum ("already has 30 of 30")
  const allowed = new Set([
    "editor.palette.count",
    "editor.upload.count",
    "editor.upload.error.cap",
  ]);
  const walk = (o: Record<string, unknown>, prefix: string, out: string[]) => {
    for (const [k, v] of Object.entries(o)) {
      const key = `${prefix}.${k}`;
      if (typeof v === "object" && v !== null) walk(v as Record<string, unknown>, key, out);
      else if (typeof v === "string" && v.includes("{{count}}") && !/_(one|other)$/.test(k))
        out.push(key);
    }
  };
  it("lists only the reviewed ones", () => {
    const found: string[] = [];
    walk(en.editor as Record<string, unknown>, "editor", found);
    expect(found.filter((k) => !allowed.has(k))).toEqual([]);
  });
});
