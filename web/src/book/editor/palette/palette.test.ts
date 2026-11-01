import { describe, expect, it } from "vitest";
import { assets, getAsset } from "../../assets";
import { MAX_BOX, MIN_BOX, addItem, emptyDoc, type LayoutDoc } from "../document";
import { assetItem, copyShift, imageItem } from "./defaults";
import { fold, matchAssets } from "./search";

describe("search", () => {
  it("folds case and diacritics, including đ", () => {
    expect(fold("Nắng ĐỒNG")).toBe("nang dong");
    expect(fold("Mặt trời")).toBe("mat troi");
  });
  const list = [
    { ...assets[0]!, id: "a", label_vi: "Mặt trời nắng", label_en: "Sunny sun", tags: ["weather"] },
    { ...assets[0]!, id: "b", label_vi: "Đám mây", label_en: "Cloud", tags: ["sky"] },
  ];
  it('"nang" finds "nắng", also by english label, tag and several words', () => {
    expect(matchAssets(list, "nang").map((a) => a.id)).toEqual(["a"]);
    expect(matchAssets(list, "NẮNG").map((a) => a.id)).toEqual(["a"]);
    expect(matchAssets(list, "dam may").map((a) => a.id)).toEqual(["b"]);
    expect(matchAssets(list, "cloud").map((a) => a.id)).toEqual(["b"]);
    expect(matchAssets(list, "sky").map((a) => a.id)).toEqual(["b"]);
    expect(matchAssets(list, "sun cloud")).toEqual([]);
  });
  it("an empty query is everything", () => {
    expect(matchAssets(list, "  ")).toHaveLength(2);
  });
});

describe("add defaults", () => {
  it("an asset is 25 % of the page width, keeps its aspect ratio and is centred", () => {
    for (const a of assets) {
      const it = assetItem(a, "newone1");
      expect(it).toMatchObject({ t: "asset", asset: a.id, w: 250, x: 375, cy: 500, r: 0 });
      expect(Math.abs(it.h / it.w - a.height / a.width)).toBeLessThan(0.01);
      expect(addItem("cover", it)(emptyDoc()).refused).toBeUndefined(); // legal for the server on any asset
    }
  });
  it("an image has its long side at 50 % of the page width, `contain`, its aspect kept", () => {
    expect(imageItem({ id: "M", width: 4000, height: 3000 }, "newone1")).toMatchObject({
      t: "image",
      media: "M",
      fit: "contain",
      w: 500,
      h: 375,
      x: 250,
      cy: 500,
    });
    expect(imageItem({ id: "M", width: 3000, height: 4000 }, "newone1")).toMatchObject({
      w: 375,
      h: 500,
      x: 313,
    });
  });
  it("an extreme aspect stays inside the server's box limits", () => {
    for (const [w, h] of [
      [12000, 20],
      [20, 12000],
    ] as const) {
      const it = imageItem({ id: "M", width: w, height: h }, "newone1");
      for (const v of [it.w, it.h]) expect(v >= MIN_BOX && v <= MAX_BOX).toBe(true);
    }
  });
  it("a new item goes on top of the z-order", () => {
    const base: LayoutDoc = {
      v: 1,
      pages: { cover: { items: [assetItem(getAsset("sticker.gold-star")!, "first01")] } },
    };
    const next = addItem("cover", assetItem(getAsset("sticker.gold-star")!, "second1"))(base).doc;
    expect(next.pages.cover!.items!.map((i) => i.id)).toEqual(["first01", "second1"]);
  });
  it("a copy moves 3 % of the page width right and down", () => {
    expect(copyShift({ w: 816, h: 1158 })).toEqual({ dx: 30, dcy: 21 });
  });
});
