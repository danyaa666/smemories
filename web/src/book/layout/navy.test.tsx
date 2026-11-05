import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { render } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { BookData } from "../../api/book";
import i18n from "../../i18n";
import { BookPages } from "../BookPages";
import { getTemplate } from "../templates";
import type { LayoutDoc } from "./runtime";

// T-166: Navy Classic draws a layout like Memory Book does (runtime.test.tsx); structure on the markup, pixels are tools/print-check's job.

const ROOT = resolve(process.cwd(), "..");
const manifest = JSON.parse(
  readFileSync(resolve(ROOT, "internal/templates/embed/navy-classic.json"), "utf8"),
) as {
  design: {
    layout: {
      page_kinds: { id: string; editable: string[] }[];
      text_fonts: string[];
      colors: string[];
      text_size: { min: number; max: number };
    };
  };
};
const t = i18n.getFixedT("vi");
const tpl = getTemplate("navy-classic");
const kinds = manifest.design.layout.page_kinds;
const editable = kinds.flatMap((k) => k.editable).sort();

const note = (i: number) => ({
  id: `01J9Z3K6V8Q4M7N2P5R8N${String(i).padStart(5, "0")}`,
  answers: { name: `Ánh ${i}`, message: `Giữ liên lạc ${i}` },
  signature: null,
  photo_ids: [],
});
function book(layout: LayoutDoc | null | undefined, year: number | null = 2026): BookData {
  const ns = [1, 2, 3].map(note);
  return {
    layout,
    design: null,
    yearbook: {
      title: "Lớp 12A1",
      school_name: "THPT Nguyễn Huệ",
      class_name: "12A1",
      graduation_year: year,
      motto: "Học, học nữa",
      language: "vi",
      page_size: "A5",
      template_id: "navy-classic",
      cover_media_id: null,
    },
    profile: {
      full_name: "Hồng",
      nickname: "",
      quote: "Một câu",
      hobbies: "Đọc sách",
      future_plans: "",
      photo_media_id: null,
    },
    notes: ns,
    note_count: ns.length,
    approved_total: ns.length,
    truncated: false,
  };
}
const props = (b: BookData) => ({
  data: b,
  size: b.yearbook.page_size,
  t,
  eager: false,
  track: () => {},
});
const html = (b: BookData) => renderToStaticMarkup(<BookPages {...props(b)} />);
const dom = (b: BookData) => render(<BookPages {...props(b)} />).container;
const doc = (pages: LayoutDoc["pages"]): LayoutDoc => ({ v: 1, pages });

describe("the manifest and navy.tsx agree", () => {
  it("the web moves exactly the elements the manifest lists", () => {
    expect(Object.keys(tpl.layout?.natural ?? {}).sort()).toEqual(editable);
  });
  it("navy.tsx marks every one of them with layoutId (the title in both of its branches; the head's two use the page name)", () => {
    const src = readFileSync(resolve(ROOT, "web/src/book/templates/navy.tsx"), "utf8");
    const used = [...src.matchAll(/<Editable layoutId=(?:"([a-z.]+)"|\{`\$\{page\}\.([a-z]+)`\})/g)]
      .flatMap((m) => (m[1] ? [m[1]] : [`profile.${m[2]}`, `notes.${m[2]}`]))
      .sort();
    expect([...new Set(used)]).toEqual(editable);
  });
  it("every page kind is the first part of its elements' ids, in the order cover, profile, notes, back", () => {
    expect(kinds.map((k) => k.id)).toEqual(["cover", "profile", "notes", "back"]);
    for (const k of kinds) for (const e of k.editable) expect(e.startsWith(`${k.id}.`)).toBe(true);
  });
  it("the web resolves exactly the manifest's colour tokens, fonts and text sizes", () => {
    expect(Object.keys(tpl.layout?.colors ?? {}).sort()).toEqual(
      [...manifest.design.layout.colors].sort(),
    );
    expect(Object.keys(tpl.layout?.fonts ?? {}).sort()).toEqual(
      [...manifest.design.layout.text_fonts].sort(),
    );
    expect(tpl.layout?.textSize).toEqual(manifest.design.layout.text_size);
    for (const k of Object.keys(tpl.layout?.defaultColor ?? {}))
      expect(manifest.design.layout.colors).toContain(tpl.layout?.defaultColor?.[k]);
  });
});

describe("names", () => {
  it("every editable element and colour token has a name in both languages", () => {
    for (const lng of ["en", "vi"]) {
      const f = i18n.getFixedT(lng);
      for (const id of editable)
        expect(f(`editor.element.${id}`, { defaultValue: "" }), id).not.toBe("");
      for (const c of manifest.design.layout.colors)
        expect(f(`editor.color.${c}`, { defaultValue: "" }), c).not.toBe("");
    }
  });
});

describe("no layout", () => {
  it("a book with no layout, or with only unknown ids, is today's page once the markers are unwrapped", () => {
    const unwrap = (markup: string) => {
      const root = document.createElement("div");
      root.innerHTML = markup;
      root.querySelectorAll("[data-layout-id]").forEach((e) => e.replaceWith(...e.childNodes));
      return root.innerHTML;
    };
    for (const year of [2026, null]) {
      const base = html(book(null, year));
      expect(base).not.toContain("data-layout");
      expect(html(book(undefined, year))).toBe(base);
      const out = html(book(doc({}), year));
      expect(out).toContain('data-layout-id="cover.title"');
      expect(unwrap(out)).toBe(unwrap(base));
    }
  });
});

describe("overrides", () => {
  it("every editable element, moved, is a direct child of its page with its data", () => {
    const pages = Object.fromEntries(
      kinds.map((k) => [
        k.id,
        { overrides: Object.fromEntries(k.editable.map((id) => [id, { x: 100 }])) },
      ]),
    ) as LayoutDoc["pages"];
    const root = dom(book(doc(pages)));
    for (const id of editable) {
      const els = root.querySelectorAll(`[data-layout-id="${id}"]`);
      expect(els.length, id).toBeGreaterThan(0);
      for (const el of els) {
        expect(el.className, id).toBe("mb-ov");
        expect(el.parentElement?.classList.contains("nc-page"), id).toBe(true);
      }
    }
  });
  it("a moved element keeps its text; the year block moves as one box; hidden leaves it and its blank in the flow", () => {
    const root = dom(
      book(
        doc({
          cover: {
            overrides: {
              "cover.year": { x: 100, w: 600, h: 300, cy: 500 },
              "cover.motto": { hidden: true },
            },
          },
        }),
      ),
    );
    const year = root.querySelector('[data-layout-id="cover.year"]')!;
    expect(year.querySelector(".nc-year-block .nc-year")?.textContent).toBe("2026");
    expect(year.querySelector(".nc-year-block .nc-class-of")).not.toBeNull();
    expect(root.querySelector(".nc-motto")).toBeNull();
    expect(root.querySelector(".nc-cover-foot [aria-hidden]")).not.toBeNull(); // the motto's blank keeps the class name where it was
  });
  it("the cover title is one element with and without a graduation year", () => {
    for (const year of [2026, null]) {
      const el = dom(book(doc({ cover: { overrides: { "cover.title": { x: 10 } } } }), year));
      expect(el.querySelectorAll('[data-layout-id="cover.title"]').length).toBe(1);
      expect(el.querySelector('[data-layout-id="cover.title"]')?.textContent).toBe("Lớp 12A1");
    }
  });
  it("the back footer exists only with a graduation year", () => {
    expect(html(book(doc({}), null))).not.toContain('data-layout-id="back.footer"');
    expect(html(book(doc({}), 2026))).toContain('data-layout-id="back.footer"');
  });
  it("the notes elements are on every notes page", () => {
    const root = dom(book(doc({})));
    expect(root.querySelectorAll('[data-layout-id="notes.heading"]').length).toBe(1);
  });
});

describe("page background", () => {
  it("a colour token replaces the tone's fill behind the frame; an unknown token keeps it", () => {
    const own = html(book(doc({ cover: { bg: { color: "gold" } } })));
    expect(own).toContain('class="nc-page nc-navy"');
    expect(own).toContain("background-color:var(--nc-gold)");
    expect(html(book(doc({ cover: { bg: { color: "accent" } } })))).not.toContain(
      "background-color",
    );
  });
  it("the owner's design image is drawn full-bleed under the frame", () => {
    const out = html(book(doc({ profile: { bg: { media: "01J9Z3K6V8Q4M7N2P5R8MEDIA1" } } })));
    const page = out.slice(out.indexOf('data-page="profile"'));
    expect(page.indexOf("position:absolute;inset:0;width:100%")).toBeLessThan(
      page.indexOf("nc-head"),
    );
  });
});

describe("items", () => {
  const TEXT = {
    id: "txt001",
    t: "text" as const,
    x: 100,
    w: 400,
    h: 120,
    cy: 850,
    text: "Lớp 12A1",
  };
  it("a text with no colour is light on the dark pages and dark on the cream ones; the layer carries the theme and fonts", () => {
    const b = book(
      doc({ cover: { items: [TEXT] }, profile: { items: [TEXT] }, back: { items: [TEXT] } }),
    );
    b.design = {
      theme: "burgundy",
      fonts: "modern",
      sections: { order: ["cover", "profile", "notes", "back"], hidden: [] },
      texts: {},
      images: {},
    } as BookData["design"];
    const out = html(b);
    expect(out).toContain(
      '<div class="mb-layer nc-layer" data-theme="burgundy" data-fonts="modern">',
    );
    const colours = [...out.matchAll(/data-layout-item="txt001"[^>]*><[^>]*color:([^;"]+)/g)].map(
      (m) => m[1],
    );
    expect(colours).toEqual(["var(--nc-light)", "var(--nc-dark)", "var(--nc-light)"]);
    expect(out).toContain("font-family:var(--nc-caps)");
  });
});
