import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { BookData } from "../../api/book";
import i18n from "../../i18n";
import { BookPages } from "../BookPages";
import { getTemplate } from "../templates";
import type { PageSize } from "../sizes";
import { pageBox, place } from "./geometry";
import type { LayoutDoc } from "./runtime";

// T-154: the book draws a layout. Structure is checked on the server-rendered markup (the pixels are tools/print-check's job).

const ROOT = resolve(process.cwd(), "..");
const manifest = JSON.parse(
  readFileSync(resolve(ROOT, "internal/templates/embed/memory-book.json"), "utf8"),
) as {
  design: {
    layout: {
      page_kinds: { id: string; editable: string[] }[];
      text_fonts: string[];
      colors: string[];
    };
  };
};
const t = i18n.getFixedT("vi");

const note = (i: number) => ({
  id: `01J9Z3K6V8Q4M7N2P5R8N${String(i).padStart(5, "0")}`,
  answers: { name: `Ánh ${i}`, message: `Giữ liên lạc ${i}` },
  signature: null,
  photo_ids: [],
});
function book(layout: LayoutDoc | null | undefined, size: PageSize = "A5", notes = 3): BookData {
  const ns = Array.from({ length: notes }, (_, i) => note(i + 1));
  return {
    layout,
    design: null,
    yearbook: {
      title: "Lớp 12A1",
      school_name: "THPT",
      class_name: "12A1",
      graduation_year: 2026,
      motto: "",
      language: "vi",
      page_size: size,
      template_id: "memory-book",
      cover_media_id: null,
    },
    profile: {
      full_name: "Hồng",
      nickname: "",
      quote: "",
      hobbies: "",
      future_plans: "",
      photo_media_id: null,
    },
    notes: ns,
    note_count: ns.length,
    approved_total: ns.length,
    truncated: false,
  };
}
const html = (b: BookData) =>
  renderToStaticMarkup(
    <BookPages data={b} size={b.yearbook.page_size} t={t} eager={false} track={() => {}} />,
  );
const count = (s: string, needle: string) => s.split(needle).length - 1;

const TEXT = {
  id: "txt001",
  t: "text" as const,
  x: 100,
  w: 400,
  h: 120,
  cy: 850,
  r: -4,
  text: "Lớp 12A1 <b>x</b>\nđường",
  font: "display",
  size: 28,
  color: "accent",
  align: "center" as const,
  bold: true,
};
const STAR = {
  id: "ast001",
  t: "asset" as const,
  asset: "sticker.gold-star",
  x: 800,
  w: 150,
  h: 150,
  cy: 100,
  r: 15,
};
const IMG = {
  id: "img001",
  t: "image" as const,
  media: "01J9Z3K6V8Q4M7N2P5R8MEDIA1",
  fit: "contain" as const,
  radius: 20,
  x: 0,
  w: 300,
  h: 200,
  cy: 500,
};
const doc = (pages: LayoutDoc["pages"]): LayoutDoc => ({ v: 1, pages });

describe("the manifest and the template agree", () => {
  const tpl = getTemplate("memory-book");
  const editable = manifest.design.layout.page_kinds.flatMap((k) => k.editable).sort();
  it("the web moves exactly the elements the manifest lists", () => {
    expect(Object.keys(tpl.layout?.natural ?? {}).sort()).toEqual(editable);
  });
  it("memory.tsx marks every one of them once with layoutId", () => {
    const src = readFileSync(resolve(ROOT, "web/src/book/templates/memory.tsx"), "utf8");
    const used = [...src.matchAll(/<Editable layoutId="([a-z.]+)"/g)].map((m) => m[1]).sort();
    expect(used).toEqual(editable);
  });
  it("every page kind is the first part of its elements' ids", () => {
    for (const k of manifest.design.layout.page_kinds)
      for (const e of k.editable) expect(e.startsWith(`${k.id}.`)).toBe(true);
  });
  it("the web resolves exactly the manifest's colour tokens and fonts", () => {
    expect(Object.keys(tpl.layout?.colors ?? {}).sort()).toEqual(
      [...manifest.design.layout.colors].sort(),
    );
    expect(Object.keys(tpl.layout?.fonts ?? {}).sort()).toEqual(
      [...manifest.design.layout.text_fonts].sort(),
    );
  });
});

describe("no layout", () => {
  it("null and undefined print the same, and nothing is marked", () => {
    expect(html(book(null))).toBe(html(book(undefined)));
    expect(html(book(null))).not.toContain("data-layout");
  });
  it("an empty layout, or one with only unknown ids, adds no item and moves nothing: unwrapping the markers gives today's page", () => {
    // the one difference is a wrapper without a box (`display: contents`) around each movable element, which the editor finds it by
    const unwrap = (markup: string) => {
      const root = document.createElement("div");
      root.innerHTML = markup;
      root.querySelectorAll("[data-layout-id]").forEach((e) => e.replaceWith(...e.childNodes));
      return root.innerHTML;
    };
    const base = html(book(null));
    for (const d of [
      doc({}),
      doc({
        nowhere: { items: [TEXT] },
        cover: { overrides: { "cover.nothing": { hidden: true } } },
      }),
    ]) {
      const out = html(book(d));
      expect(out).toContain('data-layout-id="cover.photo"');
      expect(out).not.toContain("data-layout-item");
      expect(unwrap(out)).toBe(unwrap(base));
    }
  });
});

describe("items", () => {
  const out = html(book(doc({ cover: { items: [TEXT, STAR, IMG] } })));
  it("draws each at its placed box, in order, rotated about the centre", () => {
    const r = place(TEXT, pageBox("A5"));
    expect(out).toContain(
      `data-layout-item="txt001" style="position:absolute;left:${r.left}px;top:${r.top}px;width:${r.width}px;height:${r.height}px;transform:rotate(-4deg)"`,
    );
    expect(out.indexOf("txt001")).toBeLessThan(out.indexOf("ast001"));
    expect(out.indexOf("ast001")).toBeLessThan(out.indexOf("img001"));
  });
  it("text is React text: markup is escaped, newlines kept, the theme's font and colour tokens used", () => {
    expect(out).toContain("Lớp 12A1 &lt;b&gt;x&lt;/b&gt;\nđường");
    expect(out).not.toContain("<b>x</b>");
    expect(out).toContain("font-family:var(--mb-disp)");
    expect(out).toContain("color:var(--mb-a1)");
    expect(out).toContain("white-space:pre-wrap");
    expect(out).toContain("font-size:28px");
    expect(out).toContain("font-weight:700");
    expect(out).toContain("text-align:center");
  });
  it("an image asks the print version, with its fit and radius", () => {
    expect(out).toContain('data-layout-item="img001"');
    expect(out).toContain("object-fit:contain");
    expect(out).toContain("border-radius:20px");
  });
  it("the layer carries the page's theme and font set, so tokens follow the stage 1 choice", () => {
    const b = book(doc({ cover: { items: [TEXT] } }));
    b.design = {
      theme: "peach",
      fonts: "classic",
      sections: { order: ["cover", "profile", "notes", "back"], hidden: [] },
      texts: {},
      images: {},
    } as BookData["design"];
    expect(html(b)).toContain('<div class="mb-layer" data-theme="peach" data-fonts="classic">');
  });
  it("an asset the registry does not have, and a kind it does not know, are skipped", () => {
    const o = html(
      book(
        doc({
          cover: {
            items: [
              { ...STAR, asset: "sticker.nope" },
              { ...TEXT, id: "weird1", t: "video" as "text" },
              TEXT,
            ],
          },
        }),
      ),
    );
    expect(o).not.toContain('data-layout-item="ast001"');
    expect(o).toContain('data-layout-item="txt001"');
    expect(count(o, "data-layout-item=")).toBe(1); // the unknown kind draws nothing, not an empty box
  });
  it("40 items draw 40 boxes", () => {
    const items = Array.from({ length: 40 }, (_, i) => ({
      ...TEXT,
      id: `item${String(i).padStart(3, "0")}`,
    }));
    expect(count(html(book(doc({ cover: { items } }))), "data-layout-item=")).toBe(40);
  });
  it("the notes kind repeats on every notes page, a page kind with nothing draws nothing", () => {
    const o = html(book(doc({ notes: { items: [TEXT] } }), "A5", 3));
    expect(count(o, 'data-layout-item="txt001"')).toBe(3);
    expect(
      count(html(book(doc({ profile: { items: [TEXT] } }))), 'data-layout-item="txt001"'),
    ).toBe(1);
  });
  it("is the same on A4 and Letter apart from the placed numbers", () => {
    for (const size of ["A4", "Letter"] as const) {
      const r = place(TEXT, pageBox(size));
      expect(html(book(doc({ cover: { items: [TEXT] } }), size))).toContain(`top:${r.top}px;`);
    }
  });
});

describe("overrides", () => {
  it("a moved element is drawn in its own box, absolutely placed and rotated, with its data", () => {
    const ov = { x: 120, w: 500, h: 500, cy: 400, r: 5 };
    const out = html(book(doc({ cover: { overrides: { "cover.photo": ov } } })));
    const r = place(ov, pageBox("A5"));
    expect(out).toContain(
      `data-layout-id="cover.photo" class="mb-ov" style="position:absolute;left:${r.left}px;top:${r.top}px;width:${r.width}px;height:${r.height}px;transform:rotate(5deg)"`,
    );
    expect(out).toContain("mb-cover-photo");
  });
  it("an override that omits a field starts from the template's own place", () => {
    const out = html(book(doc({ cover: { overrides: { "cover.sticker": { x: 100 } } } })));
    expect(out).toContain('data-layout-id="cover.sticker" class="mb-ov"');
    expect(out).toContain("transform:rotate(10deg)"); // the sticker's own tilt
    expect(out).toContain("viết"); // the sticker keeps its text (data binding)
  });
  it("hidden leaves the element out; an element in the flow leaves its blank, one that is absolute does not", () => {
    const flow = html(book(doc({ cover: { overrides: { "cover.title": { hidden: true } } } })));
    expect(flow).not.toContain("mb-cover-title");
    expect(flow).toContain('aria-hidden="true" style="flex:none;width:672px;height:120px"');
    const abs = html(book(doc({ cover: { overrides: { "cover.photo": { hidden: true } } } })));
    expect(abs).not.toContain("mb-cover-photo");
    expect(abs).not.toContain("flex:none;width:370px");
  });
  it("an override with no geometry and not hidden moves nothing", () => {
    expect(
      html(book(doc({ cover: { overrides: { "cover.photo": { hidden: false } } } }))),
    ).not.toContain("mb-ov");
  });
});

describe("page background", () => {
  it("a colour token replaces the paper colour and the dotted picture", () => {
    const out = html(book(doc({ cover: { bg: { color: "accent" } } })));
    expect(out).toContain("background-color:var(--mb-a1)");
  });
  it("the owner's design image is drawn full-bleed under the page content and replaces the dotted paper", () => {
    const out = html(book(doc({ cover: { bg: { media: "01J9Z3K6V8Q4M7N2P5R8MEDIA1" } } })));
    const page = out.slice(
      out.indexOf('<div class="mb-page"'),
      out.indexOf('<div class="mb-spine">'),
    );
    expect(page).toContain(
      '<img class="bk-photo " style="position:absolute;inset:0;width:100%;height:100%"',
    );
    expect(page).not.toContain("background-image");
  });
  it("an unknown colour token or asset keeps the template's own paper", () => {
    for (const bg of [{ color: "nope" }, { asset: "sticker.nope" }]) {
      const out = html(book(doc({ cover: { bg } })));
      expect(out).toContain("background-color:var(--mb-lav);background-image:url(");
    }
  });
});
