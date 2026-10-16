import { describe, expect, it } from "vitest";
import { fitElement } from "./fit";
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
    expect(fitElement(el, "8px", 0.5)).toBe("ok");
    expect(el.style.fontSize).toBe("8px");
  });
  it("shrinks in 0.5 px steps until it fits", () => {
    const el = box(12);
    expect(fitElement(el, "12px", 0.5)).toBe("shrunk");
    expect(el.style.fontSize).toBe("10px"); // 100 px of content in a 100 px box: the first size that fits
  });
  it("stops at the per-element minimum and reports overflow", () => {
    const el = box(20, 80);
    expect(fitElement(el, "20px", 0.5)).toBe("overflow");
    expect(el.style.fontSize).toBe("10px"); // never below half of 20 px
    const el2 = box(20, 80);
    expect(fitElement(el2, "20px", 0.8)).toBe("overflow");
    expect(el2.style.fontSize).toBe("16px");
  });
});
