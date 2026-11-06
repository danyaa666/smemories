import { describe, expect, it, vi } from "vitest";
import type { LayoutDoc, PageTemplate } from "../../api/bookLayout";
import { ApiError } from "../../api/client";
import { withPage } from "./document";
import {
  allCards,
  applyTemplate,
  checkName,
  groupTemplates,
  hasWork,
  keepSummary,
  MAX_KINDS_CACHED,
  nameLength,
  remember,
  saveAsTemplate,
  saveError,
  templateError,
  type ApplyDeps,
  type SaveDeps,
  type TemplateCard,
} from "./templates";

// T-175: the model of the Templates panel with a fake API. The panel and the shell are in EditorTemplates.test.tsx.

const tpl = (over: Partial<PageTemplate> & { id: string }): PageTemplate => ({
  source: "shipped",
  label_en: over.id.toUpperCase(),
  label_vi: `vi-${over.id}`,
  kind: "cover",
  item_count: 2,
  page: { items: [] },
  ...over,
});

describe("groupTemplates", () => {
  const list = [
    tpl({ id: "clean", label_en: "Clean title", label_vi: "Tiêu đề gọn" }),
    tpl({ id: "bold", label_en: "Bold", label_vi: "Đậm" }),
    tpl({ id: "OLD", source: "own", label_en: "Old one", label_vi: "Old one", created_at: 100 }),
    tpl({ id: "NEW", source: "own", label_en: "New one", label_vi: "New one", created_at: 900 }),
    tpl({ id: "MID", source: "own", label_en: "Mid", label_vi: "Mid", created_at: 500 }),
  ];
  it("keeps the shipped order of the server and puts the own ones newest first", () => {
    const g = groupTemplates(list, "en");
    expect(g.shipped.map((c) => c.id)).toEqual(["clean", "bold"]);
    expect(g.own.map((c) => c.id)).toEqual(["NEW", "MID", "OLD"]);
    expect(allCards(g).map((c) => c.id)).toEqual(["clean", "bold", "NEW", "MID", "OLD"]);
  });
  it("names a shipped template by the language and an own one by its name", () => {
    expect(groupTemplates(list, "en").shipped[0]!.name).toBe("Clean title");
    expect(groupTemplates(list, "vi").shipped[0]!.name).toBe("Tiêu đề gọn");
    expect(groupTemplates(list, "vi-VN").shipped[1]!.name).toBe("Đậm");
    expect(groupTemplates(list, "vi").own[0]!.name).toBe("New one");
  });
  it("handles an empty list and carries the item count and the page", () => {
    expect(groupTemplates([], "en")).toEqual({ shipped: [], own: [] });
    const c = groupTemplates([tpl({ id: "x", item_count: 6, page: { items: [] } })], "en")
      .shipped[0]!;
    expect(c).toMatchObject({ id: "x", source: "shipped", items: 6, page: { items: [] } });
  });
});

describe("remember (the list cache)", () => {
  it("keeps the lists of at most two kinds, the newest", () => {
    expect(MAX_KINDS_CACHED).toBe(2);
    let l = remember({}, "cover", []);
    l = remember(l, "notes", []);
    expect(Object.keys(l)).toEqual(["cover", "notes"]);
    l = remember(l, "back", []);
    expect(Object.keys(l)).toEqual(["notes", "back"]);
    l = remember(l, "notes", [tpl({ id: "n" })]); // read again: replaced, not added
    expect(Object.keys(l)).toEqual(["back", "notes"]);
    expect(l.notes).toHaveLength(1);
  });
});

describe("withPage", () => {
  const doc: LayoutDoc = {
    v: 1,
    pages: { cover: { items: [] }, notes: { bg: { color: "gold" } } },
  };
  it("replaces only that kind and leaves the rest and the input alone", () => {
    const before = JSON.stringify(doc);
    const out = withPage(doc, "cover", { bg: { color: "ink" } });
    expect(out).toEqual({
      v: 1,
      pages: { cover: { bg: { color: "ink" } }, notes: doc.pages.notes },
    });
    expect(out.pages.notes).toBe(doc.pages.notes);
    expect(out).not.toBe(doc);
    expect(JSON.stringify(doc)).toBe(before);
  });
  it("adds a kind the document does not have yet", () => {
    expect(withPage(doc, "back", { items: [] }).pages.back).toEqual({ items: [] });
    expect(withPage({ v: 1, pages: {} }, "cover", {}).pages).toEqual({ cover: {} });
  });
});

describe("hasWork (is there anything to lose)", () => {
  it("is false for no page and an empty page, true for overrides, items or a background", () => {
    expect(hasWork({ v: 1, pages: {} }, "cover")).toBe(false);
    expect(hasWork({ v: 1, pages: { cover: {} } }, "cover")).toBe(false);
    expect(hasWork({ v: 1, pages: { cover: { items: [], overrides: {} } } }, "cover")).toBe(false);
    expect(hasWork({ v: 1, pages: { notes: { bg: { color: "x" } } } }, "cover")).toBe(false);
    expect(hasWork({ v: 1, pages: { cover: { bg: { color: "x" } } } }, "cover")).toBe(true);
    expect(
      hasWork({ v: 1, pages: { cover: { overrides: { "cover.title": { w: 5 } } } } }, "cover"),
    ).toBe(true);
    expect(
      hasWork(
        {
          v: 1,
          pages: { cover: { items: [{ id: "abcdef", t: "text", x: 1, w: 1, h: 1, cy: 1 }] } },
        },
        "cover",
      ),
    ).toBe(true);
  });
});

describe("templateError", () => {
  const api = (status: number, code: string) => new ApiError(status, code, code);
  it("gives every failure its own key", () => {
    expect(templateError(api(409, "version_conflict"))).toBe("conflict");
    expect(templateError(api(400, "invalid_layout"))).toBe("invalid");
    expect(templateError(api(409, "template_mismatch"))).toBe("mismatch");
    expect(templateError(api(404, "not_found"))).toBe("gone");
    expect(templateError(api(429, "rate_limited"))).toBe("busy");
    expect(templateError(api(0, "network"))).toBe("offline");
    expect(templateError(api(500, "internal_error"))).toBe("other");
    expect(templateError(new Error("x"))).toBe("other");
  });
});

describe("applyTemplate", () => {
  const card: TemplateCard = { id: "clean", source: "shipped", name: "Clean", items: 2, page: {} };
  const layout: LayoutDoc = { v: 1, pages: { cover: { bg: { color: "ink" } } } };
  function deps(over: Partial<ApplyDeps> = {}) {
    const order: string[] = [];
    let version = 4;
    const d: ApplyDeps = {
      settle: vi.fn(async () => {
        order.push("settle");
        version = 7; // a save moved it
        return true;
      }),
      version: vi.fn(() => version),
      apply: vi.fn(async (_s, _id, base) => {
        order.push(`apply@${base}`);
        return { version: 8, updated_at: 1, layout, dropped: 0 };
      }),
      adopt: vi.fn(() => void order.push("adopt")),
      enterConflict: vi.fn(),
      ...over,
    };
    return { d, order };
  }

  it("settles first, asks with the version the store has after that, then adopts the answer", async () => {
    const { d, order } = deps();
    expect(await applyTemplate(card, d)).toEqual({ ok: true, dropped: 0 });
    expect(order).toEqual(["settle", "apply@7", "adopt"]);
    expect(d.apply).toHaveBeenCalledWith("shipped", "clean", 7);
    expect(d.adopt).toHaveBeenCalledWith(layout, 8);
  });
  it("reports the dropped count", async () => {
    const { d } = deps({ apply: async () => ({ version: 8, updated_at: 1, layout, dropped: 3 }) });
    expect(await applyTemplate(card, d)).toEqual({ ok: true, dropped: 3 });
  });
  it("sends nothing when the newest edit could not be saved", async () => {
    const { d } = deps({ settle: async () => false });
    expect(await applyTemplate(card, d)).toEqual({ ok: false, error: "unsaved" });
    expect(d.apply).not.toHaveBeenCalled();
    expect(d.adopt).not.toHaveBeenCalled();
  });
  it.each([
    [409, "version_conflict", "conflict"],
    [400, "invalid_layout", "invalid"],
    [409, "template_mismatch", "mismatch"],
    [404, "not_found", "gone"],
    [429, "rate_limited", "busy"],
    [0, "network", "offline"],
    [500, "internal_error", "other"],
  ])("maps %i %s to %s and adopts nothing", async (status, code, error) => {
    const { d } = deps({
      apply: async () => {
        throw new ApiError(status, code, code);
      },
    });
    const r = await applyTemplate(card, d);
    expect(r).toMatchObject({ ok: false, error });
    expect(d.adopt).not.toHaveBeenCalled();
    expect(d.enterConflict).toHaveBeenCalledTimes(error === "conflict" ? 1 : 0);
  });
  it("starts the conflict flow with the stored document and passes Retry-After on", async () => {
    const stored = { version: 9, layout };
    const c = deps({
      apply: async () => {
        throw new ApiError(409, "version_conflict", "x", undefined, undefined, stored);
      },
    });
    await applyTemplate(card, c.d);
    expect(c.d.enterConflict).toHaveBeenCalledWith(stored);
    const b = deps({
      apply: async () => {
        throw new ApiError(429, "rate_limited", "x", undefined, 12);
      },
    });
    expect(await applyTemplate(card, b.d)).toEqual({ ok: false, error: "busy", retryAfter: 12 });
  });
});

// T-176: saving a page as an own template. The panel and the dialog are in EditorSaveTemplate.test.tsx.

describe("checkName", () => {
  it("trims and refuses an empty name", () => {
    expect(checkName("  Bìa của tôi \n")).toEqual({ ok: true, name: "Bìa của tôi" });
    expect(checkName("")).toEqual({ ok: false, error: "empty" });
    expect(checkName(" \t ")).toEqual({ ok: false, error: "empty" });
  });
  it("allows 60 code points and refuses 61, counting composed and decomposed Vietnamese the same", () => {
    const composed = "ệ".repeat(60); // U+1EC7, one code point each
    const decomposed = "e\u0323\u0302".repeat(60); // e + dot below + circumflex: 180 code points, 60 letters once composed
    expect([...composed]).toHaveLength(60);
    expect([...decomposed]).toHaveLength(180);
    expect(nameLength(decomposed)).toBe(60);
    expect(checkName(composed)).toEqual({ ok: true, name: composed });
    expect(checkName(decomposed)).toEqual({ ok: true, name: composed }); // stored in NFC, as the server does
    expect(checkName("ệ".repeat(61))).toEqual({ ok: false, error: "long" });
    expect(checkName("e\u0323\u0302".repeat(61))).toEqual({ ok: false, error: "long" });
  });
  it("counts an emoji as one code point (not two UTF-16 units)", () => {
    expect(checkName("😀".repeat(60)).ok).toBe(true);
    expect(checkName("😀".repeat(61))).toEqual({ ok: false, error: "long" });
  });
  it("refuses control, invisible and separator characters but not a joiner", () => {
    for (const bad of ["a\u0000b", "a\nb", "a\u200bb", "a\u202eb", "a\u2028b", "a\ufeffb"])
      expect(checkName(bad)).toEqual({ ok: false, error: "chars" });
    expect(checkName("👨\u200d👩").ok).toBe(true);
  });
});

describe("keepSummary", () => {
  it("counts the items kept, the moved elements, a colour or asset background, and the images that are not copied", () => {
    const it = (id: string, t: "text" | "asset" | "image") => ({ id, t, x: 1, w: 1, h: 1, cy: 1 });
    const doc: LayoutDoc = {
      v: 1,
      pages: {
        cover: {
          bg: { color: "gold" },
          overrides: { "cover.title": { w: 5 }, "cover.motto": { w: 6 } },
          items: [
            it("tAAAAA", "text"),
            it("sAAAAA", "asset"),
            it("iAAAAA", "image"),
            it("iBBBBB", "image"),
          ],
        },
        back: { bg: { media: "01J9Z3K6V8Q4M7N2P5R8M00001" }, items: [it("tBBBBB", "text")] },
      },
    };
    expect(keepSummary(doc, "cover")).toEqual({ items: 2, moved: 2, background: true, images: 2 });
    // a picture background is an image that is not copied, so it is not "a background" either
    expect(keepSummary(doc, "back")).toEqual({ items: 1, moved: 0, background: false, images: 1 });
    expect(keepSummary(doc, "notes")).toEqual({ items: 0, moved: 0, background: false, images: 0 });
  });
});

describe("saveError", () => {
  const api = (status: number, code: string) => new ApiError(status, code, code);
  it("gives every failure its own key", () => {
    expect(saveError(api(409, "template_limit"))).toBe("limit");
    expect(saveError(api(413, "payload_too_large"))).toBe("large");
    expect(saveError(api(400, "invalid_layout"))).toBe("invalid");
    expect(saveError(api(400, "invalid_body"))).toBe("param");
    expect(saveError(api(404, "not_found"))).toBe("gone");
    expect(saveError(api(429, "rate_limited"))).toBe("busy");
    expect(saveError(api(0, "network_error"))).toBe("offline");
    expect(saveError(api(500, "internal_error"))).toBe("other");
    expect(saveError(new Error("x"))).toBe("other");
  });
});

describe("saveAsTemplate", () => {
  const made = {
    id: "01J9Z3K6V8Q4M7N2P5R8C00001",
    name: "Bìa",
    kind: "cover",
    stripped: 1,
  } as never;
  function deps(over: Partial<SaveDeps> = {}) {
    const order: string[] = [];
    const d: SaveDeps = {
      settle: vi.fn(async () => {
        order.push("settle");
        return true;
      }),
      create: vi.fn(async (n: string) => {
        order.push(`create:${n}`);
        return made;
      }),
      ...over,
    };
    return { d, order };
  }
  it("settles first, then creates once, and reports what the server says was stripped", async () => {
    const { d, order } = deps();
    expect(await saveAsTemplate("Bìa", d)).toEqual({
      ok: true,
      id: "01J9Z3K6V8Q4M7N2P5R8C00001",
      name: "Bìa",
      stripped: 1,
    });
    expect(order).toEqual(["settle", "create:Bìa"]);
  });
  it("sends nothing when the newest edit could not be saved", async () => {
    const { d } = deps({ settle: async () => false });
    expect(await saveAsTemplate("Bìa", d)).toEqual({ ok: false, error: "unsaved" });
    expect(d.create).not.toHaveBeenCalled();
  });
  it.each([
    [409, "template_limit", "limit"],
    [413, "payload_too_large", "large"],
    [400, "invalid_layout", "invalid"],
    [400, "invalid_body", "param"],
    [404, "not_found", "gone"],
    [429, "rate_limited", "busy"],
    [0, "network_error", "offline"],
    [500, "internal_error", "other"],
  ])("maps %i %s to %s", async (status, code, error) => {
    const { d } = deps({
      create: async () => {
        throw new ApiError(status, code, code);
      },
    });
    expect(await saveAsTemplate("Bìa", d)).toMatchObject({ ok: false, error });
  });
  it("passes the limit from the 409 data and Retry-After on", async () => {
    const lim = deps({
      create: async () => {
        throw new ApiError(409, "template_limit", "x", undefined, undefined, { limit: 20 });
      },
    });
    expect(await saveAsTemplate("Bìa", lim.d)).toEqual({ ok: false, error: "limit", limit: 20 });
    const busy = deps({
      create: async () => {
        throw new ApiError(429, "rate_limited", "x", undefined, 12);
      },
    });
    expect(await saveAsTemplate("Bìa", busy.d)).toEqual({
      ok: false,
      error: "busy",
      retryAfter: 12,
    });
  });
});
