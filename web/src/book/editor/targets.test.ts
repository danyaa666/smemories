import { describe, expect, it } from "vitest";
import { pageBox } from "../layout/geometry";
import { getTemplate } from "../templates";
import type { LayoutDoc } from "./document";
import { isMoved, targetsOf, unrotate } from "./targets";

const spec = getTemplate("memory-book").layout!;
const pb = pageBox("A5");

describe("unrotate", () => {
  it("gives back the size of a rotated box from its bounding box", () => {
    const [w, h, deg] = [370, 400, -3];
    const a = (deg * Math.PI) / 180;
    const W = w * Math.abs(Math.cos(a)) + h * Math.abs(Math.sin(a));
    const H = w * Math.abs(Math.sin(a)) + h * Math.abs(Math.cos(a));
    const r = unrotate(W, H, deg);
    expect(r.w).toBeCloseTo(w, 6);
    expect(r.h).toBeCloseTo(h, 6);
  });
  it("leaves an unrotated or 45 degree box alone", () => {
    expect(unrotate(100, 50, 0)).toEqual({ w: 100, h: 50 });
    expect(unrotate(100, 50, 45)).toEqual({ w: 100, h: 50 });
  });
});

describe("targetsOf", () => {
  const doc: LayoutDoc = {
    v: 1,
    pages: {
      cover: {
        overrides: { "cover.photo": { hidden: true }, "cover.sticker": { x: 100 } },
        items: [
          { id: "itemAA", t: "asset", asset: "sticker.gold-star", x: 10, w: 100, h: 100, cy: 500 },
        ],
      },
    },
  };
  it("lists the page's template elements in the template's order, then its items", () => {
    expect(targetsOf(doc, "cover", spec, pb).map((t) => t.id)).toEqual([
      "cover.title",
      "cover.photo",
      "cover.sticker",
      "itemAA",
    ]);
    expect(targetsOf(doc, "notes", spec, pb).map((t) => t.id)).toEqual(["notes.heading"]);
    expect(targetsOf(doc, "cover", undefined, pb).map((t) => t.id)).toEqual(["itemAA"]);
  });
  it("marks a hidden element and takes the measured place only for an unmoved one", () => {
    const m = { left: 5, top: 6, width: 7, height: 8 };
    const t = targetsOf(doc, "cover", spec, pb, { "cover.title": m, "cover.sticker": m });
    expect(t.find((x) => x.id === "cover.photo")?.hidden).toBe(true);
    expect(t.find((x) => x.id === "cover.title")?.rect).toEqual(m); // not moved: where the template draws it
    expect(t.find((x) => x.id === "cover.sticker")?.rect).not.toEqual(m); // moved: the override decides
    expect(t.find((x) => x.id === "cover.sticker")?.r).toBe(10); // rotation kept from the template
  });
  it("knows what moves an element", () => {
    expect(isMoved(undefined)).toBe(false);
    expect(isMoved({ hidden: false })).toBe(false);
    expect(isMoved({ hidden: true })).toBe(true);
    expect(isMoved({ w: 300 })).toBe(true);
  });
});
