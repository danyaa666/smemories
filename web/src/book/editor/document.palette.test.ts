import { describe, expect, it } from "vitest";
import {
  addItem,
  duplicateItem,
  mediaUses,
  purgeMedia,
  removeProblems,
  setBg,
  setItemProps,
  type LayoutDoc,
  type LayoutItem,
} from "./document";

// T-162: the document operations the palette needs (copy, picture properties, deleting a design image, removing what the server refused).
const MEDIA = "01J9Z3K6V8Q4M7N2P5R8MEDIA1";
const asset = (id: string, over: Partial<LayoutItem> = {}): LayoutItem => ({
  id,
  t: "asset",
  asset: "sticker.gold-star",
  x: 100,
  w: 250,
  h: 250,
  cy: 500,
  ...over,
});
const image = (id: string, media = MEDIA): LayoutItem => ({
  id,
  t: "image",
  media,
  x: 100,
  w: 400,
  h: 300,
  cy: 300,
});
const doc = (cover: LayoutItem[], extra: LayoutDoc["pages"] = {}): LayoutDoc => ({
  v: 1,
  pages: { cover: { items: cover }, ...extra },
});

describe("duplicateItem", () => {
  it("adds a copy on top with a new id, shifted, and leaves the original", () => {
    const r = duplicateItem("cover", "aaaaaa", "bbbbbb", 30, 21)(doc([asset("aaaaaa", { r: 5 })]));
    expect(r.refused).toBeUndefined();
    const items = r.doc.pages.cover!.items!;
    expect(items.map((i) => i.id)).toEqual(["aaaaaa", "bbbbbb"]);
    expect(items[1]).toMatchObject({
      x: 130,
      cy: 521,
      w: 250,
      h: 250,
      r: 5,
      asset: "sticker.gold-star",
    });
    expect(items[0]!.x).toBe(100);
  });
  it("shifts the other way when the copy would leave the page too far", () => {
    const r = duplicateItem(
      "cover",
      "aaaaaa",
      "bbbbbb",
      30,
      21,
    )(doc([asset("aaaaaa", { x: 870 })]));
    expect(r.doc.pages.cover!.items![1]).toMatchObject({ x: 840, cy: 479 });
  });
  it("keeps the 40 and 150 limits and refuses an unknown item", () => {
    const full = doc(
      Array.from({ length: 40 }, (_, i) => asset(`item${String(i).padStart(2, "0")}`)),
    );
    expect(duplicateItem("cover", "item00", "newnew", 30, 21)(full).refused).toBe(
      "too_many_on_page",
    );
    expect(duplicateItem("cover", "nothing", "newnew", 30, 21)(doc([])).refused).toBe(
      "no_such_item",
    );
  });
});

describe("setItemProps", () => {
  it("sets and clears the tint of an asset", () => {
    const d = setItemProps("cover", "aaaaaa", { tint: "accent" })(doc([asset("aaaaaa")])).doc;
    expect(d.pages.cover!.items![0]!.tint).toBe("accent");
    const back = setItemProps("cover", "aaaaaa", { tint: undefined })(d).doc;
    expect("tint" in back.pages.cover!.items![0]!).toBe(false);
  });
  it("sets fit, radius and media of an image, and refuses values the server would refuse", () => {
    const d0 = doc([image("iiiiii")]);
    const d = setItemProps("cover", "iiiiii", {
      fit: "contain",
      radius: 500,
      media: "01J9Z3K6V8Q4M7N2P5R8MEDIA2",
    })(d0).doc;
    expect(d.pages.cover!.items![0]).toMatchObject({
      fit: "contain",
      radius: 500,
      media: "01J9Z3K6V8Q4M7N2P5R8MEDIA2",
    });
    for (const bad of [
      { radius: 501 },
      { radius: -1 },
      { radius: 1.5 },
      { fit: "fill" as "cover" },
      { media: "" },
    ])
      expect(setItemProps("cover", "iiiiii", bad)(d0).refused).toBe("bad_value");
  });
  it("does not put a tint on an image or a radius on an asset", () => {
    expect(setItemProps("cover", "iiiiii", { tint: "ink" })(doc([image("iiiiii")])).refused).toBe(
      "bad_value",
    );
    expect(setItemProps("cover", "aaaaaa", { radius: 3 })(doc([asset("aaaaaa")])).refused).toBe(
      "bad_value",
    );
    expect(setItemProps("cover", "zzzzzz", { fit: "cover" })(doc([])).refused).toBe("no_such_item");
  });
});

describe("a design image in use", () => {
  const used = doc(
    [image("iiiiii"), asset("aaaaaa"), image("jjjjjj", "01J9Z3K6V8Q4M7N2P5R8OTHER1")],
    {
      back: { bg: { media: MEDIA }, items: [image("kkkkkk")] },
      profile: { bg: { color: "paper" } },
    },
  );
  it("lists the page kinds that use it, item or background", () => {
    expect(mediaUses(used, MEDIA).sort()).toEqual(["back", "cover"]);
    expect(mediaUses(used, "01J9Z3K6V8Q4M7N2P5R8NOUSE1")).toEqual([]);
  });
  it("purgeMedia removes every item and background that uses it, and nothing else", () => {
    const d = purgeMedia(MEDIA)(used).doc;
    expect(d.pages.cover!.items!.map((i) => i.id)).toEqual(["aaaaaa", "jjjjjj"]);
    expect(d.pages.back).toEqual({ items: [] });
    expect(d.pages.profile).toEqual({ bg: { color: "paper" } });
    expect(mediaUses(d, MEDIA)).toEqual([]);
  });
});

describe("removeProblems", () => {
  const d = doc([asset("aaaaaa"), image("iiiiii"), asset("cccccc")], {
    back: { bg: { media: MEDIA } },
  });
  it("removes the items and backgrounds named by the paths, also by a field of them", () => {
    const r = removeProblems([
      "pages.cover.items[1].media",
      "pages.cover.items[0]",
      "pages.back.bg.media",
    ])(d).doc;
    expect(r.pages.cover!.items!.map((i) => i.id)).toEqual(["cccccc"]);
    expect(r.pages.back!.bg).toBeUndefined();
  });
  it("leaves the document alone for a path it does not understand", () => {
    expect(
      removeProblems(["pages.cover.overrides.cover.title.x", "version", "pages.cover.items"])(d)
        .doc,
    ).toBe(d);
  });
});

describe("limits still hold for palette items", () => {
  it("addItem refuses the 41st item and the 151st in the book", () => {
    const page = (n: number, p: string) =>
      Array.from({ length: n }, (_, i) => asset(`${p}${String(i).padStart(3, "0")}`));
    expect(addItem("cover", asset("newnew"))(doc(page(40, "c"))).refused).toBe("too_many_on_page");
    const full: LayoutDoc = {
      v: 1,
      pages: {
        cover: { items: page(40, "c") },
        profile: { items: page(40, "p") },
        back: { items: page(40, "b") },
        notes: { items: page(30, "n") },
      },
    };
    expect(addItem("notes", asset("newnew"))(full).refused).toBe("too_many_in_total");
  });
  it("setBg takes a colour, an asset or a media id", () => {
    expect(setBg("cover", { media: MEDIA })(doc([])).doc.pages.cover!.bg).toEqual({ media: MEDIA });
  });
});
