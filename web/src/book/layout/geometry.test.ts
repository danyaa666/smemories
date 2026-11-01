import { describe, expect, it } from "vitest";
import { boxStyle, pageBox, place, unplace, type Box } from "./geometry";

// T-154: the draw rule of ADR 0004 Q1 ("centre"), ported from docs/spikes/editor/src/units.mjs.

const A5 = pageBox("A5");
const A4 = pageBox("A4");
const LETTER = pageBox("Letter");
const sticker: Box = { x: 600, w: 180, h: 180, cy: 700, r: 15 };

describe("place", () => {
  it("draws left = x * W, top = cy * H - h / 2", () => {
    expect(place({ x: 500, w: 200, h: 100, cy: 500 }, A5)).toEqual({
      left: 408,
      top: A5.h / 2 - 40.8,
      width: 163.2,
      height: 81.6,
    });
  });

  it("keeps the shape (width and height in page-width units) on every paper", () => {
    for (const page of [A5, A4, LETTER]) {
      const r = place(sticker, page);
      expect(r.width).toBeCloseTo(146.88, 5);
      expect(r.height).toBeCloseTo(146.88, 5);
    }
  });

  it("round trips on A5, A4 and Letter (the stored integers come back)", () => {
    for (const page of [A5, A4, LETTER])
      for (const b of [
        sticker,
        { x: 20, w: 400, h: 120, cy: 300 },
        { x: 100, w: 500, h: 500, cy: 400 },
      ])
        expect(unplace(place(b, page), page)).toEqual({ x: b.x, w: b.w, h: b.h, cy: b.cy });
  });

  it("a box flush to the A5 bottom edge is moved back inside on Letter, not cut", () => {
    const h = 150;
    const b: Box = {
      x: 100,
      w: h * (1000 / 816),
      h: h * (1000 / 816),
      cy: (1000 * (A5.h - h / 2)) / A5.h,
    };
    const r = place(b, LETTER);
    expect(r.top + r.height).toBeCloseTo(LETTER.h, 5); // flush to the Letter bottom
    expect(r.top).toBeGreaterThanOrEqual(0);
  });

  it("clamps at all four edges", () => {
    const w = 200;
    const a = place({ x: -50, w, h: w, cy: 500 }, A5);
    expect(a.left).toBe(0);
    const b = place({ x: 990, w, h: w, cy: 500 }, A5);
    expect(b.left + b.width).toBeCloseTo(A5.w, 5);
    const c = place({ x: 400, w, h: w, cy: -100 }, A5);
    expect(c.top).toBe(0);
    const d = place({ x: 400, w, h: w, cy: 1100 }, A5);
    expect(d.top + d.height).toBeCloseTo(A5.h, 5);
  });

  it("centres a box larger than the page instead of pinning one edge", () => {
    const r = place({ x: 0, w: 1200, h: 100, cy: 500 }, A5);
    expect(r.left).toBeCloseTo((A5.w - r.width) / 2, 5);
  });
});

describe("unplace", () => {
  it("is the inverse of place for a rect inside the page, rounding to integers", () => {
    const rect = { left: 100.4, top: 200.2, width: 300.7, height: 150.1 };
    const b = unplace(rect, A4);
    expect(
      Number.isInteger(b.x) &&
        Number.isInteger(b.w) &&
        Number.isInteger(b.h) &&
        Number.isInteger(b.cy),
    ).toBe(true);
    const back = place(b, A4);
    for (const k of ["left", "top", "width", "height"] as const)
      expect(Math.abs(back[k] - rect[k])).toBeLessThan(0.7);
  });
});

describe("boxStyle", () => {
  it("rotates about the centre: the centre of the drawn box does not depend on r", () => {
    const s = boxStyle(sticker, A5);
    expect(s.transform).toBe("rotate(15deg)");
    expect(s.position).toBe("absolute");
    // CSS rotates about the element's centre by default (transform-origin 50% 50%), so left/top/width/height are those of the unrotated box
    const flat = boxStyle({ ...sticker, r: 0 }, A5);
    expect([s.left, s.top, s.width, s.height]).toEqual([
      flat.left,
      flat.top,
      flat.width,
      flat.height,
    ]);
    expect(flat.transform).toBeUndefined();
  });
});
