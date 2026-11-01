import { describe, expect, it } from "vitest";
import {
  addItem,
  clearOverride,
  emptyDoc,
  newItemId,
  removeItem,
  reorderItem,
  setBg,
  setItemBox,
  setOverride,
  setText,
  type LayoutDoc,
  type LayoutItem,
  type Op,
} from "./document";

const text = (id: string, over: Partial<LayoutItem> = {}): LayoutItem => ({
  id,
  t: "text",
  text: "Lớp 12A1",
  x: 100,
  w: 400,
  h: 120,
  cy: 500,
  r: 0,
  ...over,
});
const run = (op: Op, doc: LayoutDoc) => op(doc);
const with3 = (): LayoutDoc =>
  ["aaaaaa", "bbbbbb", "cccccc"].reduce(
    (d, id) => run(addItem("cover", text(id)), d).doc,
    emptyDoc(),
  );
const ids = (d: LayoutDoc, kind = "cover") => (d.pages[kind]?.items ?? []).map((i) => i.id);

describe("addItem", () => {
  it("adds on top and shares the other pages by reference", () => {
    const d0 = run(addItem("profile", text("pppppp")), emptyDoc()).doc;
    const d1 = run(addItem("cover", text("aaaaaa")), d0).doc;
    const d2 = run(addItem("cover", text("bbbbbb")), d1).doc;
    expect(ids(d2)).toEqual(["aaaaaa", "bbbbbb"]);
    expect(d2.pages.profile).toBe(d0.pages.profile);
    expect(d1.pages.cover?.items).toHaveLength(1); // the old document is untouched
  });
  it("refuses with the same document", () => {
    const d = with3();
    const cases: [string, LayoutItem, string][] = [
      ["bad id (short)", text("abc"), "bad_id"],
      ["bad id (char)", text("abc-def"), "bad_id"],
      ["duplicate id", text("aaaaaa"), "duplicate_id"],
      ["501 code points", text("dddddd", { text: "a".repeat(501) }), "text_too_long"],
      ["control char", text("dddddd", { text: "a\u0007b" }), "bad_text"],
      ["w 9", text("dddddd", { w: 9 }), "bad_box"],
      ["x as float is rounded, but NaN is not", text("dddddd", { x: NaN }), "bad_box"],
    ];
    for (const [name, item, why] of cases) {
      const r = run(addItem("cover", item), d);
      expect(r.refused, name).toBe(why);
      expect(r.doc, name).toBe(d);
    }
  });
  it("counts code points, not UTF-16 units (emoji, Vietnamese)", () => {
    const d = run(addItem("cover", text("dddddd", { text: "😀".repeat(500) })), emptyDoc());
    expect(d.refused).toBeUndefined();
    expect(
      run(addItem("cover", text("eeeeee", { text: "ệ".repeat(500) + "ệ" })), emptyDoc()).refused,
    ).toBe("text_too_long");
    expect(
      run(addItem("cover", text("ffffff", { text: "a\nb" })), emptyDoc()).refused,
    ).toBeUndefined();
  });
  it("keeps <script> as text and rounds a drag's fractions", () => {
    const d = run(
      addItem("cover", text("dddddd", { text: "<script>x</script>", x: 100.4, cy: 99.6 })),
      emptyDoc(),
    );
    expect(d.doc.pages.cover?.items?.[0]).toMatchObject({
      text: "<script>x</script>",
      x: 100,
      cy: 100,
    });
  });
  it("stops at 40 items per page kind and 150 in total", () => {
    const fill = (d: LayoutDoc, kind: string, n: number) => {
      for (let i = 0; i < n; i++)
        d = run(addItem(kind, text(`${kind}${String(i).padStart(5, "0")}`)), d).doc;
      return d;
    };
    let d = fill(emptyDoc(), "cover", 40);
    expect(run(addItem("cover", text("overflow1")), d)).toEqual({
      doc: d,
      refused: "too_many_on_page",
    });
    d = fill(fill(d, "profile", 40), "notes", 40);
    d = fill(d, "back", 30); // 4 x 40 = 160 > 150
    expect(run(addItem("back", text("overflow2")), d).refused).toBe("too_many_in_total");
    expect(run(addItem("back", text("overflow2")), d).doc).toBe(d);
  });
});

// The boundary cases of internal/booklayout/validate_test.go (the server's table), item box x/w/h/cy/r.
describe("box limits mirror the server", () => {
  const accepted: [string, Partial<LayoutItem>][] = [
    ["w 10", { x: -5, w: 10, h: 10, cy: 0 }],
    ["w and h 2000", { x: -1000, w: 2000, h: 2000, cy: 500 }],
    ["x at the left limit", { x: -200, w: 400, h: 10, cy: 0 }],
    ["x at the right limit", { x: 800, w: 400, h: 10, cy: 0 }],
    ["cy at the top limit", { x: 0, w: 10, h: 100, cy: -70 }],
    ["cy at the bottom limit", { x: 0, w: 10, h: 100, cy: 1070 }],
    ["r -180", { x: 0, w: 10, h: 10, cy: 0, r: -180 }],
    ["r 180", { x: 0, w: 10, h: 10, cy: 0, r: 180 }],
  ];
  const refused: [string, Partial<LayoutItem>][] = [
    ["w 9", { x: 0, w: 9, h: 10, cy: 0 }],
    ["w 2001", { x: 0, w: 2001, h: 10, cy: 0 }],
    ["h 9", { x: 0, w: 10, h: 9, cy: 0 }],
    ["h 2001", { x: 0, w: 10, h: 2001, cy: 0 }],
    ["x one left of the limit", { x: -201, w: 400, h: 10, cy: 0 }],
    ["x one right of the limit", { x: 801, w: 400, h: 10, cy: 0 }],
    ["x odd width, one left", { x: -200, w: 399, h: 10, cy: 0 }],
    ["cy above the page", { x: 0, w: 10, h: 100, cy: -71 }],
    ["cy below the page", { x: 0, w: 10, h: 100, cy: 1071 }],
    ["r 181", { x: 0, w: 10, h: 10, cy: 0, r: 181 }],
    ["r -181", { x: 0, w: 10, h: 10, cy: 0, r: -181 }],
  ];
  it.each(accepted)("accepts %s", (_n, box) => {
    expect(run(addItem("cover", text("abcdef", box)), emptyDoc()).refused).toBeUndefined();
  });
  it.each(refused)("refuses %s", (_n, box) => {
    expect(run(addItem("cover", text("abcdef", box)), emptyDoc()).refused).toBe("bad_box");
  });
});

describe("setItemBox", () => {
  it("moves one item and keeps the others by reference", () => {
    const d = with3();
    const r = run(setItemBox("cover", "bbbbbb", { x: 200, cy: 300.4 }), d);
    expect(r.doc.pages.cover?.items?.[1]).toMatchObject({ x: 200, cy: 300, w: 400 });
    expect(r.doc.pages.cover?.items?.[0]).toBe(d.pages.cover?.items?.[0]);
  });
  it("refuses an unknown item and a box outside the limits", () => {
    const d = with3();
    expect(run(setItemBox("cover", "zzzzzz", { x: 1 }), d)).toEqual({
      doc: d,
      refused: "no_such_item",
    });
    expect(run(setItemBox("cover", "aaaaaa", { w: 5 }), d)).toEqual({ doc: d, refused: "bad_box" });
    expect(run(setItemBox("cover", "aaaaaa", { x: 900 }), d)).toEqual({
      doc: d,
      refused: "bad_box",
    });
  });
});

describe("removeItem and reorderItem", () => {
  it("removes", () => {
    expect(ids(run(removeItem("cover", "bbbbbb"), with3()).doc)).toEqual(["aaaaaa", "cccccc"]);
    const d = with3();
    expect(run(removeItem("cover", "nope12"), d)).toEqual({ doc: d, refused: "no_such_item" });
  });
  it("reorders the draw order and refuses an index outside it", () => {
    const d = with3();
    expect(ids(run(reorderItem("cover", "aaaaaa", 2), d).doc)).toEqual([
      "bbbbbb",
      "cccccc",
      "aaaaaa",
    ]);
    expect(ids(run(reorderItem("cover", "cccccc", 0), d).doc)).toEqual([
      "cccccc",
      "aaaaaa",
      "bbbbbb",
    ]);
    expect(run(reorderItem("cover", "aaaaaa", 0), d).doc).toBe(d); // already there: same document, no new step
    expect(run(reorderItem("cover", "aaaaaa", 3), d)).toEqual({ doc: d, refused: "bad_value" });
    expect(run(reorderItem("cover", "nope12", 1), d).refused).toBe("no_such_item");
  });
});

describe("setOverride", () => {
  it("moves, then hides, merging into the earlier override", () => {
    const d1 = run(setOverride("cover.photo", { x: 120, w: 500, h: 500, cy: 400 }), emptyDoc()).doc;
    const d2 = run(setOverride("cover.photo", { hidden: true }), d1).doc;
    expect(d2.pages.cover?.overrides?.["cover.photo"]).toEqual({
      x: 120,
      w: 500,
      h: 500,
      cy: 400,
      hidden: true,
    });
    expect(d1.pages.cover?.overrides?.["cover.photo"]?.hidden).toBeUndefined();
  });
  it("accepts a partial box (the server allows any subset) and refuses bad ones", () => {
    expect(run(setOverride("cover.title", { hidden: true }), emptyDoc()).refused).toBeUndefined();
    expect(run(setOverride("cover.title", { x: 1 }), emptyDoc()).refused).toBeUndefined();
    for (const patch of [{ w: 9 }, { r: 181 }, { cy: 2500 }]) {
      expect(run(setOverride("cover.title", patch), emptyDoc()).refused).toBe("bad_box");
    }
    expect(run(setOverride("nodot", { x: 1 }), emptyDoc()).refused).toBe("bad_id");
    expect(run(setOverride("cover.", { x: 1 }), emptyDoc()).refused).toBe("bad_id");
  });
  it("puts a back-page element on its own page kind", () => {
    const d = run(setOverride("back.pact", { hidden: true }), emptyDoc()).doc;
    expect(Object.keys(d.pages)).toEqual(["back"]);
  });
});

describe("clearOverride", () => {
  it("drops one override, keeps the others, and removes an empty map", () => {
    const d1 = run(setOverride("cover.title", { x: 10, hidden: true }), emptyDoc()).doc;
    const d2 = run(setOverride("cover.photo", { x: 20 }), d1).doc;
    const d3 = run(clearOverride("cover.title"), d2).doc;
    expect(d3.pages.cover?.overrides).toEqual({ "cover.photo": { x: 20 } });
    const d4 = run(clearOverride("cover.photo"), d3).doc;
    expect(d4.pages.cover).toEqual({});
  });
  it("is a no-op for an element with no override", () => {
    const d = with3();
    expect(run(clearOverride("cover.title"), d).doc).toBe(d);
  });
});

describe("setBg", () => {
  it("sets exactly one of asset, media, color; null removes", () => {
    const d1 = run(setBg("cover", { asset: "shape.x" }), emptyDoc()).doc;
    expect(d1.pages.cover?.bg).toEqual({ asset: "shape.x" });
    const d2 = run(setBg("cover", { color: "accent" }), d1).doc;
    expect(d2.pages.cover?.bg).toEqual({ color: "accent" });
    expect(run(setBg("cover", null), d2).doc.pages.cover).toEqual({});
    expect(run(setBg("back", null), d2).doc).toBe(d2);
  });
  it("refuses none or two", () => {
    expect(run(setBg("cover", {}), emptyDoc()).refused).toBe("bad_bg");
    expect(run(setBg("cover", { asset: "a", color: "b" }), emptyDoc()).refused).toBe("bad_bg");
    expect(run(setBg("cover", { color: "" }), emptyDoc()).refused).toBe("bad_bg");
  });
});

describe("setText", () => {
  it("changes text and style of a text item only", () => {
    const d = with3();
    const r = run(
      setText("cover", "aaaaaa", { text: "Xin chào 👋", bold: true, size: 28, align: "center" }),
      d,
    );
    expect(r.doc.pages.cover?.items?.[0]).toMatchObject({
      text: "Xin chào 👋",
      bold: true,
      size: 28,
      align: "center",
    });
    const withAsset = run(
      addItem("cover", {
        id: "stick1",
        t: "asset",
        asset: "sticker.star",
        x: 1,
        w: 100,
        h: 100,
        cy: 1,
      }),
      d,
    ).doc;
    expect(run(setText("cover", "stick1", { text: "x" }), withAsset).refused).toBe("bad_value");
  });
  it("refuses a long text, a control character, a fractional size and an unknown item", () => {
    const d = with3();
    expect(run(setText("cover", "aaaaaa", { text: "a".repeat(501) }), d)).toEqual({
      doc: d,
      refused: "text_too_long",
    });
    expect(run(setText("cover", "aaaaaa", { text: "a\u0000" }), d).refused).toBe("bad_text");
    expect(run(setText("cover", "aaaaaa", { size: 12.5 }), d).refused).toBe("bad_value");
    expect(run(setText("cover", "nope12", { text: "x" }), d).refused).toBe("no_such_item");
  });
});

describe("newItemId", () => {
  it("makes ids the server accepts", () => {
    const seen = new Set(Array.from({ length: 50 }, newItemId));
    expect(seen.size).toBe(50);
    for (const id of seen) expect(id).toMatch(/^[A-Za-z0-9]{6,26}$/);
  });
});
