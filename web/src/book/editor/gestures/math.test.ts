import { describe, expect, it } from "vitest";
import { pageBox, place, unplace, type Box, type Rect } from "../../layout/geometry";
import { designZoom, type PageSize } from "../../sizes";
import {
  clampPos,
  moveRect,
  normaliseDeg,
  nudge,
  resizeRect,
  rotateTo,
  snapAxis,
  snapLines,
  pointerAngle,
  type Lines,
} from "./math";
import { nudgeOf } from "./keys";

const PAPERS: PageSize[] = ["A5", "A4", "Letter"];
const NONE: Lines = { xs: [], ys: [] };
const box: Box = { x: 300, w: 200, h: 150, cy: 400, r: 0 };

describe("move: the pointer and the stored box agree on every paper and zoom", () => {
  for (const size of PAPERS)
    for (const zoom of [0.6, 1, 1.5]) {
      it(`${size} at ${zoom}`, () => {
        const pb = pageBox(size);
        const k = designZoom(size) * zoom; // screen px per design px
        const r0 = place(box, pb);
        for (const [sx, sy] of [
          [60, -40],
          [-37, 91],
          [0.4, 0.4],
        ] as const) {
          const { rect } = moveRect(r0, sx / k, sy / k, NONE, 0, pb, false);
          const stored = unplace(rect, pb); // what is saved
          const drawn = place({ ...stored }, pb); // what the next render draws
          const got = { x: (drawn.left - r0.left) * k, y: (drawn.top - r0.top) * k };
          // the stored form is whole thousandths: the error is half a unit of the page width (x) or height (y), at most 0.9 screen px at 150 %
          expect(Math.abs(got.x - sx)).toBeLessThan(0.7);
          expect(Math.abs(got.y - sy)).toBeLessThan(0.9);
          expect(Math.hypot(got.x - sx, got.y - sy)).toBeLessThan(1.1);
        }
      });
    }
});

describe("clamp", () => {
  const pb = pageBox("A5");
  it("keeps the box inside the page without changing its size", () => {
    const r: Rect = { left: -50, top: 5000, width: 100, height: 80 };
    expect(clampPos(r, pb)).toEqual({ left: 0, top: pb.h - 80, width: 100, height: 80 });
  });
  it("centres a box larger than the page, as place() draws it", () => {
    const r = clampPos({ left: 0, top: 0, width: pb.w + 100, height: 10 }, pb);
    expect(r.left).toBe(-50);
  });
  it("a drag far past the edge ends flush and comes back by the pointer distance", () => {
    const r0 = place(box, pb);
    const out = moveRect(r0, 0, 5000, NONE, 0, pb, false).rect;
    expect(out.top + out.height).toBeCloseTo(pb.h, 6);
  });
});

describe("snap", () => {
  const pb = pageBox("A4");
  const lines = snapLines(pb, 28, [{ left: 100, top: 200, width: 50, height: 60 }]);
  it("picks the nearest line within the threshold, for start, centre or end", () => {
    expect(snapAxis(97, 50, lines.xs, 6)).toEqual({ d: 3, line: 100 }); // start onto the other item's left
    expect(snapAxis(pb.w / 2 - 25 + 4, 50, lines.xs, 6)?.line).toBe(pb.w / 2); // centre onto the page centre
    expect(snapAxis(pb.w - 28 - 50 - 2, 50, lines.xs, 6)).toEqual({ d: 2, line: pb.w - 28 });
    expect(snapAxis(400, 50, [100, 700], 6)).toBeNull();
  });
  it("moveRect snaps by default and Alt (snap=false) switches it off", () => {
    const r0: Rect = { left: 0, top: 0, width: 50, height: 60 };
    const on = moveRect(r0, 96, 3, lines, 6, pb, true);
    expect(on.rect.left).toBe(100);
    expect(on.rect.top).toBe(0); // the page top edge
    expect(on.guides).toEqual({ x: [100], y: [0] });
    const off = moveRect(r0, 96, 3, lines, 6, pb, false);
    expect(off.rect.left).toBe(96);
    expect(off.rect.top).toBe(3);
    expect(off.guides).toEqual({ x: [], y: [] });
  });
  it("includes the page edges, centre lines and safe margin", () => {
    expect(lines.xs).toEqual(expect.arrayContaining([0, 28, pb.w / 2, pb.w - 28, pb.w]));
    expect(lines.ys).toEqual(expect.arrayContaining([0, 28, pb.h / 2, pb.h - 28, pb.h]));
  });
});

describe("resize", () => {
  const pb = pageBox("A4");
  const r0: Rect = { left: 200, top: 300, width: 100, height: 50 };
  it("an unrotated corner handle grows the box and keeps the opposite corner", () => {
    const r = resizeRect(r0, 0, "se", 40, 20, false, pb);
    expect(r).toEqual({ left: 200, top: 300, width: 140, height: 70 });
    const nw = resizeRect(r0, 0, "nw", -10, -10, false, pb);
    expect(nw).toEqual({ left: 190, top: 290, width: 110, height: 60 });
  });
  it("an edge handle changes one side only", () => {
    expect(resizeRect(r0, 0, "e", 30, 99, false, pb)).toEqual({ ...r0, width: 130 });
    expect(resizeRect(r0, 0, "n", 99, -20, false, pb)).toEqual({ ...r0, top: 280, height: 70 });
  });
  it("is rotation-aware: dragging the right edge of a box turned 90 degrees follows its own axis", () => {
    // turned 90 degrees clockwise, the box's right edge points down the screen; a 20 px pull down widens it by 20 and moves its centre 10 down
    const r = resizeRect(r0, 90, "e", 0, 20, false, pb);
    expect(r.width).toBeCloseTo(120, 9);
    expect(r.height).toBeCloseTo(50, 9);
    expect(r.left + r.width / 2).toBeCloseTo(250, 9);
    expect(r.top + r.height / 2).toBeCloseTo(325 + 10, 9);
    // a pull sideways does nothing to it
    expect(resizeRect(r0, 90, "e", 20, 0, false, pb).width).toBeCloseTo(100, 9);
  });
  it("the opposite corner stays on the page for a rotated corner drag", () => {
    const deg = 30;
    const centre = { x: r0.left + 50, y: r0.top + 25 };
    const rot = (x: number, y: number, c: { x: number; y: number }) => {
      const a = (deg * Math.PI) / 180;
      return {
        x: c.x + x * Math.cos(a) - y * Math.sin(a),
        y: c.y + x * Math.sin(a) + y * Math.cos(a),
      };
    };
    const before = rot(-50, -25, centre); // top left corner on the page
    const r = resizeRect(r0, deg, "se", 25, 15, false, pb);
    const c2 = { x: r.left + r.width / 2, y: r.top + r.height / 2 };
    const after = rot(-r.width / 2, -r.height / 2, c2);
    expect(after.x).toBeCloseTo(before.x, 6);
    expect(after.y).toBeCloseTo(before.y, 6);
  });
  it("Shift keeps the aspect ratio", () => {
    const r = resizeRect(r0, 0, "se", 60, 5, true, pb);
    expect(r.width / r.height).toBeCloseTo(2, 9);
    expect(r.width).toBeCloseTo(160, 9);
    const e = resizeRect(r0, 0, "e", 40, 0, true, pb);
    expect(e.width / e.height).toBeCloseTo(2, 9);
    expect(e.left).toBe(200);
  });
  it("never goes below 2 % of the page width or above the page", () => {
    const tiny = resizeRect(r0, 0, "se", -5000, -5000, false, pb);
    expect(tiny.width).toBeCloseTo(0.02 * pb.w, 9);
    expect(tiny.height).toBeCloseTo(0.02 * pb.w, 9);
    const huge = resizeRect(r0, 0, "se", 9000, 9000, false, pb);
    expect(huge.width).toBe(pb.w);
    expect(huge.height).toBe(pb.h);
    const ratioTiny = resizeRect(r0, 0, "se", -5000, -5000, true, pb);
    expect(Math.min(ratioTiny.width, ratioTiny.height)).toBeGreaterThanOrEqual(0.02 * pb.w - 1e-9);
  });
  it("a box flush to the right and bottom edge can still be resized from its corner handle (the result is moved back inside)", () => {
    const flush: Rect = { left: pb.w - 100, top: pb.h - 50, width: 100, height: 50 };
    const r = resizeRect(flush, 0, "se", 30, 30, false, pb);
    expect(r.left + r.width).toBeLessThanOrEqual(pb.w + 1e-9);
    expect(r.width).toBe(130);
  });
});

describe("rotate", () => {
  it("turns by the angle the pointer moved, not to the pointer's absolute angle", () => {
    // the pointer starts straight above the centre (the handle) and moves a quarter turn clockwise
    const a0 = pointerAngle(0, 0, 0, -50);
    const a1 = pointerAngle(0, 0, 50, 0);
    expect(rotateTo(10, a0, a1, false)).toBe(100);
  });
  it("Shift snaps to 15 degree steps", () => {
    expect(rotateTo(0, 0, 22, true)).toBe(15);
    expect(rotateTo(0, 0, 23, true)).toBe(30);
    expect(rotateTo(0, 0, 22, false)).toBe(22);
  });
  it("normalises to (-180, 180]", () => {
    expect(normaliseDeg(180)).toBe(180);
    expect(normaliseDeg(-180)).toBe(180);
    expect(normaliseDeg(181)).toBe(-179);
    expect(normaliseDeg(-181)).toBe(179);
    expect(normaliseDeg(540)).toBe(180);
    expect(normaliseDeg(0)).toBe(0);
    expect(rotateTo(170, 0, 30, false)).toBe(-160);
  });
  it("a full pointer turn comes back to the same angle", () => {
    expect(rotateTo(35, -170, 190, false)).toBe(35);
  });
});

describe("keyboard steps use the same maths", () => {
  const pb = pageBox("A5");
  const r0 = place(box, pb);
  it("an arrow moves 1 design px, Shift 10, and the clamp holds", () => {
    const k = (key: string, shiftKey = false) =>
      nudgeOf({ key, shiftKey, altKey: false, ctrlKey: false, metaKey: false });
    expect(k("ArrowRight")).toEqual({ kind: "move", dx: 1, dy: 0 });
    expect(k("ArrowUp", true)).toEqual({ kind: "move", dx: 0, dy: -10 });
    const n = nudge(r0, 0, k("ArrowRight")!, pb);
    expect(n.rect.left - r0.left).toBe(1);
    const edge = nudge({ ...r0, left: 0 }, 0, k("ArrowLeft", true)!, pb);
    expect(edge.rect.left).toBe(0);
  });
  it("Alt+Shift+Arrow resizes by 10, Alt+Arrow alone belongs to the browser", () => {
    const e = (key: string, over: object) => ({
      key,
      shiftKey: false,
      altKey: false,
      ctrlKey: false,
      metaKey: false,
      ...over,
    });
    expect(nudgeOf(e("ArrowRight", { altKey: true, shiftKey: true }))).toEqual({
      kind: "resize",
      dw: 10,
      dh: 0,
    });
    expect(nudgeOf(e("ArrowLeft", { altKey: true }))).toBeNull();
    expect(nudgeOf(e("ArrowLeft", { ctrlKey: true }))).toBeNull();
    expect(nudgeOf(e("ArrowLeft", { metaKey: true }))).toBeNull();
  });
  it("rotates with , and . by 1 and with Shift by 15", () => {
    const e = (key: string, shiftKey: boolean) =>
      nudgeOf({ key, shiftKey, altKey: false, ctrlKey: false, metaKey: false });
    expect(e(",", false)).toEqual({ kind: "rotate", ddeg: -1 });
    expect(e(".", false)).toEqual({ kind: "rotate", ddeg: 1 });
    expect(e("<", true)).toEqual({ kind: "rotate", ddeg: -15 }); // Shift+, types < on a US layout
    expect(e(">", true)).toEqual({ kind: "rotate", ddeg: 15 });
    expect(nudge(r0, 179, { kind: "rotate", ddeg: 1 }, pb).deg).toBe(180);
    expect(nudge(r0, 180, { kind: "rotate", ddeg: 1 }, pb).deg).toBe(-179);
  });
  it("a keyboard resize follows the box's own axes when it is rotated", () => {
    const n = nudge(r0, 90, { kind: "resize", dw: 10, dh: 0 }, pb);
    expect(n.rect.width).toBeCloseTo(r0.width + 10, 9);
    expect(n.rect.height).toBeCloseTo(r0.height, 9);
  });
});
