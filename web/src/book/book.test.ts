import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import type { BookData } from "../api/book";
import { fitElement, type FitInfo } from "./fit";
import { noteFits, ownFits, toPt } from "./fitReport";
import { paginate } from "./paginate";
import { DESIGN_W, SIZES, designHeight, pageCss } from "./sizes";

describe("paginate", () => {
  const n = (k: number) => Array.from({ length: k }, (_, i) => i);
  it("0 notes make no notes page, 1 note one page", () => {
    expect(paginate(n(0), 4)).toEqual([]);
    expect(paginate(n(1), 4)).toEqual([[0]]);
  });
  it("40 notes of 4 per page make 10 full pages in order", () => {
    const pages = paginate(n(40), 4);
    expect(pages).toHaveLength(10);
    expect(pages.every((p) => p.length === 4)).toBe(true);
    expect(pages.flat()).toEqual(n(40));
  });
  it("the last page holds the rest", () => {
    expect(paginate(n(9), 4).map((p) => p.length)).toEqual([4, 4, 1]);
  });
});

describe("page sizes", () => {
  it("writes one @page rule in millimetres with no margin", () => {
    expect(pageCss("A5")).toBe("@page { size: 148mm 210mm; margin: 0 }");
    expect(pageCss("Letter")).toBe("@page { size: 215.9mm 279.4mm; margin: 0 }");
  });
  it("the design box follows the page ratio", () => {
    expect(designHeight("A5")).toBe(Math.round((DESIGN_W * 210) / 148));
    expect(SIZES.A4.h / SIZES.A4.w).toBeCloseTo(designHeight("A4") / DESIGN_W, 2);
  });
});

/** jsdom has no layout: this box needs 10 px of height per font px, and has 100 px. */
function box(fontPx: number, height = 100) {
  const el = document.createElement("div");
  el.style.fontSize = `${fontPx}px`;
  document.body.append(el);
  Object.defineProperty(el, "scrollHeight", { get: () => parseFloat(el.style.fontSize) * 10 });
  Object.defineProperty(el, "clientHeight", { value: height });
  return el;
}

describe("fitElement", () => {
  it("leaves text that fits", () => {
    const el = box(8);
    expect(fitElement(el, "8px", 0.5)).toEqual({ state: "ok", base: 8, px: 8 });
    expect(el.style.fontSize).toBe("8px");
  });
  it("shrinks in 0.5 px steps until it fits", () => {
    const el = box(12);
    expect(fitElement(el, "12px", 0.5)).toEqual({ state: "shrunk", base: 12, px: 10 });
    expect(el.style.fontSize).toBe("10px"); // 100 px of content in a 100 px box: the first size that fits
  });
  it("stops at the per-element minimum and reports overflow", () => {
    const el = box(20, 80);
    expect(fitElement(el, "20px", 0.5)).toMatchObject({ state: "overflow", px: 10 });
    expect(el.style.fontSize).toBe("10px"); // never below half of 20 px
    const el2 = box(20, 80);
    expect(fitElement(el2, "20px", 0.8)).toMatchObject({ state: "overflow", px: 16 });
    expect(el2.style.fontSize).toBe("16px");
  });
});

describe("which notes were shrunk or cut (fitReport)", () => {
  const data = (names: (string | undefined)[], size: "A5" | "Letter" = "A5") =>
    ({
      yearbook: { page_size: size },
      notes: names.map((name, i) => ({
        id: `N${i + 1}`,
        answers: name === undefined ? {} : { name },
        photo_ids: [],
      })),
    }) as unknown as BookData;
  const info = (state: FitInfo["state"], px: number): FitInfo => ({ state, base: 20, px });

  it("converts design px to points on the paper: 20 design px are 10.3 pt on A5 and 15 pt on Letter", () => {
    expect(toPt(20, "A5")).toBe(10.5);
    expect(toPt(20, "Letter")).toBe(15);
    expect(toPt(10, "A5")).toBe(5);
  });

  it("lists a note once, with its smallest size, cut notes first and then the smallest text", () => {
    const fits = noteFits(
      {
        "note:N1:message": info("shrunk", 14),
        "note:N1:wish": info("shrunk", 12), // the same note: the smaller size counts
        "note:N2:message": info("overflow", 10),
        "note:N3": info("shrunk", 9), // the basic layout's key has no field
        "note:N4:message": info("ok", 20), // fits as designed: not listed
        "profile:quote": info("shrunk", 8), // not a friend's note
      },
      data(["Linh", "Minh", "Hoa", "Nam"]),
    );
    expect(fits.map((f) => [f.name, f.cut, f.pt])).toEqual([
      ["Minh", true, 5],
      ["Hoa", false, 4.5],
      ["Linh", false, 6],
    ]);
  });

  it("falls back to the position of a note without a name", () => {
    const [f] = noteFits({ "note:N2:message": info("shrunk", 10) }, data(["A", undefined]));
    expect(f).toMatchObject({ name: "", number: 2 });
  });

  it("counts the owner's own boxes apart from friends' notes", () => {
    expect(
      ownFits({
        "profile:hobbies": info("overflow", 10),
        "cover:title": info("shrunk", 60),
        "note:N1:message": info("overflow", 10),
      }),
    ).toEqual({ cut: 1, smaller: 1 });
  });
});

const font = (name: string) => readFileSync(join(process.cwd(), "src/book/fonts", name), "utf8"); // vitest runs in web/

describe("template fonts cover Vietnamese", () => {
  // The files are subsets (latin, latin-ext, vietnamese); a letter is covered when some @font-face of the family declares its
  // code point in a unicode-range. (Glyph coverage of the files was checked with fontTools; the browser check is in tools/print-check.)
  const css = font("fonts.css");
  const ranges = (family: string) =>
    [...css.matchAll(/@font-face\s*\{([^}]*)\}/g)]
      .filter((m) => m[1]!.includes(`font-family: "${family}"`))
      .flatMap((m) => [...m[1]!.matchAll(/U\+([0-9A-F]+)(?:-([0-9A-F]+))?/gi)])
      .map((r) => [parseInt(r[1]!, 16), parseInt(r[2] ?? r[1]!, 16)] as const);
  const PANGRAM =
    "Tôi yêu những người bạn thân thiết: ưu ái, đằm thắm, nghĩ ngợi, quả ổi, ngũ cốc, ĐẶNG HUỲNH.";
  const ALL = "aàáảãạăằắẳẵặâầấẩẫậeèéẻẽẹêềếểễệiìíỉĩịoòóỏõọôồốổỗộơờớởỡợuùúủũụưừứửữựyỳýỷỹỵđ";
  it.each(["Baloo 2", "Patrick Hand", "Nunito", "Be Vietnam Pro", "Cormorant Garamond"])(
    "%s declares every Vietnamese letter",
    (family) => {
      const r = ranges(family);
      expect(r.length).toBeGreaterThan(0);
      for (const ch of new Set([...PANGRAM, ...ALL, ...ALL.toUpperCase()])) {
        if (!/[\p{L}]/u.test(ch)) continue;
        const cp = ch.normalize("NFC").codePointAt(0)!;
        expect(
          r.some(([a, b]) => cp >= a && cp <= b),
          `${family}: ${ch} U+${cp.toString(16)}`,
        ).toBe(true);
      }
    },
  );
  it("ships the licence texts and the README row for the lookalikes", () => {
    for (const f of ["OFL-Baloo2.txt", "OFL-PatrickHand.txt", "OFL-BeVietnamPro.txt"])
      expect(font(f)).toContain("SIL OPEN FONT LICENSE Version 1.1");
    const readme = font("README.md");
    expect(readme).toContain("Baloo 2");
    expect(readme).toContain("Patrick Hand");
    expect(readme).toContain("Be Vietnam Pro");
  });
});
