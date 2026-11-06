import { describe, expect, it, vi } from "vitest";
import type { LayoutDoc, PageTemplate } from "../../api/bookLayout";
import { ApiError } from "../../api/client";
import { withPage } from "./document";
import {
  allCards,
  applyTemplate,
  groupTemplates,
  hasWork,
  MAX_KINDS_CACHED,
  remember,
  templateError,
  type ApplyDeps,
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
