import { readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, test } from "vitest";
import type { BookData } from "../api/book";
import { pageCount, profilePageCount } from "./BookPages";
import { printSections, resolveDesign } from "./design";
import { photoIds } from "./ready";
import { getTemplate } from "./templates";

// T-142: the design runtime of the memory-book template. The ids the web draws must be the ids of the manifest the server validates against (T-141),
// every theme must keep its text readable, and every font family of a font set must be one that is shipped (fonts.css) with Vietnamese.

const ROOT = resolve(process.cwd(), ".."); // vitest runs in web/
const read = (f: string) => readFileSync(resolve(ROOT, f), "utf8");
const fontsCss = read("web/src/book/fonts/fonts.css");
const memoryCss = read("web/src/book/templates/memory.css");
const manifest = JSON.parse(
  readFileSync(resolve(ROOT, "internal/templates/embed/memory-book.json"), "utf8"),
) as {
  design: {
    themes: string[];
    font_sets: string[];
    sections: { id: string }[];
    texts: { id: string; max: number }[];
    images: { id: string; choices: string[] }[];
  };
};

/** The custom properties declared for each `[data-theme="x"]` or `[data-fonts="x"]` block of memory.css. */
function blocks(attr: "theme" | "fonts"): Record<string, Record<string, string>> {
  const out: Record<string, Record<string, string>> = {};
  for (const m of memoryCss.matchAll(
    new RegExp(`\\[data-${attr}="([a-z0-9-]+)"\\]\\s*\\{([^}]*)\\}`, "g"),
  )) {
    const vars: Record<string, string> = {};
    for (const v of (m[2] ?? "").matchAll(/--mb-([a-z0-9]+):\s*([^;]+);/g))
      vars[v[1] ?? ""] = (v[2] ?? "").trim();
    out[m[1] ?? ""] = vars;
  }
  return out;
}

const lum = (hex: string) => {
  const h = hex.replace("#", "");
  const full = h.length === 3 ? [...h].map((c) => c + c).join("") : h;
  const [r, g, b] = [0, 2, 4].map((i) => parseInt(full.slice(i, i + 2), 16) / 255) as [
    number,
    number,
    number,
  ];
  const f = (c: number) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
};
const contrast = (a: string, b: string) => {
  const [hi, lo] = [lum(a), lum(b)].sort((x, y) => y - x) as [number, number];
  return (hi + 0.05) / (lo + 0.05);
};

const SURFACES = ["paper", "white", "lav", "yellow", "pink", "peach", "mint", "sky"];
const TEXTS = ["ink", "soft"];

describe("memory-book design ids", () => {
  const tpl = getTemplate("memory-book");

  test("the web themes and font sets are the manifest's, default first", () => {
    expect(tpl.design?.themes).toEqual(manifest.design.themes);
    expect(tpl.design?.fonts).toEqual(manifest.design.font_sets);
  });

  test("memory.css defines exactly those themes and font sets", () => {
    expect(Object.keys(blocks("theme")).sort()).toEqual([...manifest.design.themes].sort());
    expect(Object.keys(blocks("fonts")).sort()).toEqual([...manifest.design.font_sets].sort());
  });

  test("an image slot of the manifest that the web draws keeps its default first (cover.pattern: dots is today's)", () => {
    expect(manifest.design.images.find((i) => i.id === "cover.pattern")?.choices[0]).toBe("dots");
  });
});

describe("themes", () => {
  const themes = blocks("theme");

  test("lavender is exactly today's colours", () => {
    expect(themes["lavender"]).toMatchObject({
      ink: "#3b2f4a",
      soft: "#5e5170",
      paper: "#fffbf6",
      white: "#fff",
      lav: "#e6defa",
      yellow: "#fff1b5",
      pink: "#fcd5e2",
      peach: "#ffdccb",
      mint: "#d2eedf",
      sky: "#d3e8f8",
      rule: "rgba(59, 47, 74, 0.32)",
      shadow: "rgba(59, 47, 74, 0.12)",
    });
  });

  test("every theme declares the same variables", () => {
    const keys = Object.keys(themes["lavender"] ?? {}).sort();
    expect(keys.length).toBeGreaterThan(14);
    for (const [id, vars] of Object.entries(themes))
      expect([id, Object.keys(vars).sort()]).toEqual([id, keys]);
  });

  test("memory.css has no colour outside the theme blocks", () => {
    const rest = memoryCss
      .replace(/\[data-theme="[a-z]+"\]\s*\{[^}]*\}/g, "")
      .replace(/\/\*[\s\S]*?\*\//g, "")
      .replace(/rgba\(255, 255, 255, [\d.]+\)/g, ""); // the cover spine is translucent white in every theme
    expect(rest.match(/#[0-9a-fA-F]{3,8}\b|rgba?\(/g)).toBeNull();
  });

  // The pairs are listed in docs/templates.md ("Themes").
  for (const id of ["lavender", "peach", "mint"]) {
    test(`${id}: text is at least 4.5:1 on every surface`, () => {
      const v = themes[id] ?? {};
      for (const t of TEXTS)
        for (const s of SURFACES)
          expect(
            contrast(v[t] ?? "", v[s] ?? ""),
            `${id}: ${t} ${v[t]} on ${s} ${v[s]}`,
          ).toBeGreaterThanOrEqual(4.5);
    });
  }
});

describe("font sets", () => {
  const sets = blocks("fonts");
  const faces = new Set([...fontsCss.matchAll(/font-family:\s*"([^"]+)"/g)].map((m) => m[1]));

  test("playful is today's stacks", () => {
    expect(sets["playful"]).toEqual({
      body: "Nunito, sans-serif",
      disp: '"Baloo 2", Nunito, sans-serif',
      cute: '"Patrick Hand", Nunito, cursive',
    });
  });

  test("every family of a set is shipped with a Vietnamese subset", () => {
    for (const [id, vars] of Object.entries(sets))
      for (const stack of Object.values(vars)) {
        const family = /^"?([^",]+)"?/.exec(stack)?.[1] ?? "";
        expect(faces.has(family), `${id}: ${family} has a @font-face in fonts.css`).toBe(true);
        const stem = family.replace(/ /g, "");
        const has = readdirSync(resolve(ROOT, "web/src/book/fonts")).some(
          (f) => f.startsWith(`${stem}-`) && f.endsWith("-vietnamese.woff2"),
        );
        expect(has, `${id}: ${family} Vietnamese file`).toBe(true);
      }
  });

  test("classic names no family of playful, so choosing it requests none of playful's files", () => {
    const fam = (v: Record<string, string> = {}) =>
      new Set(
        Object.values(v).flatMap((s) =>
          [...s.matchAll(/"([^"]+)"|([A-Z][A-Za-z]+)/g)].map((m) => m[1] ?? m[2]),
        ),
      );
    const playful = fam(sets["playful"]);
    for (const f of fam(sets["classic"])) expect(playful.has(f), f).toBe(false);
  });
});

describe("resolveDesign", () => {
  const ids = { themes: ["lavender", "peach", "mint"], fonts: ["playful", "classic"] };
  const raw = {
    theme: "mint",
    fonts: "classic",
    sections: { order: ["cover"], hidden: [] },
    texts: { "cover.sticker": "hi", empty: "" },
    images: { "cover.pattern": "plain" },
  };

  test("no design: the first (default) theme and font set, the template's own text, no image choice", () => {
    for (const d of [resolveDesign(null, ids), resolveDesign(undefined, ids)]) {
      expect([d.theme, d.fonts]).toEqual(["lavender", "playful"]);
      expect(d.text("cover.sticker", "own")).toBe("own");
      expect(d.image("cover.pattern")).toBeUndefined();
    }
  });

  test("a design is applied; an id this build does not know falls back to the default", () => {
    const d = resolveDesign(raw, ids);
    expect([d.theme, d.fonts]).toEqual(["mint", "classic"]);
    expect(d.text("cover.sticker", "own")).toBe("hi");
    expect(d.text("empty", "own")).toBe("own");
    expect(d.text("toString", "own")).toBe("own");
    expect(d.image("cover.pattern")).toBe("plain");
    const odd = resolveDesign({ ...raw, theme: "neon", fonts: "comic" }, ids);
    expect([odd.theme, odd.fonts]).toEqual(["lavender", "playful"]);
  });

  test("a template with no choices has no theme", () => {
    expect([resolveDesign(raw).theme, resolveDesign(raw).fonts]).toEqual(["", ""]);
  });
});

// T-143: sections, texts and images.
const design = (order: string[], hidden: string[] = []): NonNullable<BookData["design"]> => ({
  theme: "lavender",
  fonts: "playful",
  sections: { order, hidden },
  texts: {},
  images: { "cover.pattern": "dots" },
});
const book = (notes: number, d: BookData["design"]) =>
  ({
    yearbook: { template_id: "memory-book" },
    notes: Array.from({ length: notes }, (_, i) => ({ id: `n${i}` })),
    design: d,
  }) as unknown as BookData;

describe("sections", () => {
  const all = ["cover", "profile", "notes", "back"];

  test("no design, or the default one, prints every section in the template's order", () => {
    expect(printSections(undefined)).toEqual(all);
    expect(printSections(null)).toEqual(all);
    expect(printSections(design(all))).toEqual(all);
  });

  test("the order is followed and hidden sections are left out", () => {
    expect(printSections(design(["cover", "notes", "profile", "back"]))).toEqual([
      "cover",
      "notes",
      "profile",
      "back",
    ]);
    expect(printSections(design(all, ["profile", "back"]))).toEqual(["cover", "notes"]);
  });

  test("an order that is not exactly this build's four sections prints the default order", () => {
    for (const order of [["cover", "back"], ["cover", "profile", "notes", "extra", "back"], []])
      expect(printSections(design(order))).toEqual(all);
  });

  test("the sections of the web are the manifest's, in its order", () => {
    expect(manifest.design.sections.map((s) => s.id)).toEqual(all);
  });

  test("pages that print: hidden sections do not count, notes pages are one per note", () => {
    expect(pageCount(book(5, null))).toBe(8); // cover, profile, 5 notes, back
    expect(profilePageCount(book(5, null))).toBe(6);
    expect(pageCount(book(5, design(all, ["profile"])))).toBe(7);
    expect(pageCount(book(5, design(all, ["back"])))).toBe(7);
    expect(pageCount(book(5, design(all, ["profile", "back"])))).toBe(6);
    expect(profilePageCount(book(5, design(all, ["profile", "back"])))).toBe(5);
    expect(pageCount(book(5, design(["cover", "notes", "profile", "back"])))).toBe(8);
    expect(pageCount(book(0, design(all, ["profile", "back"])))).toBe(1); // the cover alone
  });
});

test("the photos of a hidden section are not fetched", () => {
  const all = ["cover", "profile", "notes", "back"];
  const b = {
    yearbook: { cover_media_id: "C" },
    profile: { photo_media_id: "P" },
    notes: [{ photo_ids: ["N1", "N2"] }],
  };
  expect(photoIds({ ...b, design: null })).toEqual(["C", "P", "N1", "N2"]);
  expect(photoIds({ ...b, design: design(all, ["profile"]) })).toEqual(["C", "N1", "N2"]);
  expect(
    photoIds({ ...b, design: design(all, ["profile", "back"]) }, { cover: false, notes: true }),
  ).toEqual(["N1", "N2"]);
});

describe("texts and images of the web are the manifest's", () => {
  const source = read("web/src/book/templates/memory.tsx");

  test("every fixed text of the manifest is a slot of the template (Own, read through useDesign().text), and only those", () => {
    const used = [...source.matchAll(/\bid[=:]\s*"([a-z0-9-]+\.[a-z0-9-]+)"/g)].map((m) => m[1]);
    expect([...used].sort()).toEqual(manifest.design.texts.map((t) => t.id).sort()); // each once
  });

  test("the files of the cover.pattern choices are those of the manifest", () => {
    const choices = manifest.design.images.find((i) => i.id === "cover.pattern")?.choices;
    const m = /PATTERNS[^=]*=\s*\{([^}]*)\}/.exec(source);
    expect(
      m?.[1]
        ?.split(",")
        .map((x) => x.split(":")[0]?.trim())
        .filter(Boolean),
    ).toEqual(choices);
  });
});

// T-145: Navy Classic. The same pins as the memory-book's, for navy.tsx, navy.css and navy-classic.json.
describe("navy-classic design", () => {
  const navyCss = read("web/src/book/templates/navy.css");
  const navySource = read("web/src/book/templates/navy.tsx");
  const navy = JSON.parse(read("internal/templates/embed/navy-classic.json"))
    .design as typeof manifest.design;
  const tpl = getTemplate("navy-classic");
  const vars = (attr: "theme" | "fonts") => {
    const out: Record<string, Record<string, string>> = {};
    for (const m of navyCss.matchAll(
      new RegExp(`\\[data-${attr}="([a-z0-9-]+)"\\]\\s*\\{([^}]*)\\}`, "g"),
    )) {
      const v: Record<string, string> = {};
      for (const x of (m[2] ?? "").matchAll(/--nc-([a-z0-9]+):\s*([^;]+);/g))
        v[x[1] ?? ""] = (x[2] ?? "").trim();
      out[m[1] ?? ""] = v;
    }
    return out;
  };
  const themes = vars("theme");
  const sets = vars("fonts");

  test("the web themes and font sets are the manifest's, default first; navy.css defines exactly those", () => {
    expect(tpl.design?.themes).toEqual(navy.themes);
    expect(tpl.design?.fonts).toEqual(navy.font_sets);
    expect(Object.keys(themes).sort()).toEqual([...navy.themes].sort());
    expect(Object.keys(sets).sort()).toEqual([...navy.font_sets].sort());
    expect(navy.sections.map((s) => s.id)).toEqual(["cover", "profile", "notes", "back"]);
    expect(navy.images).toEqual([]);
  });

  test("navy is exactly today's colours (the canvas)", () => {
    expect(themes["navy"]).toEqual({
      dark: "#1b2a4a",
      light: "#f7f3ea",
      gold: "#c9a86a",
      bronze: "#7a5c2b",
      photo: "#dcd6c9",
      rule: "rgba(27, 42, 74, 0.22)",
    });
  });

  test("every theme declares the same variables, and navy.css has no colour outside them", () => {
    const keys = Object.keys(themes["navy"] ?? {}).sort();
    for (const [id, v] of Object.entries(themes))
      expect([id, Object.keys(v).sort()]).toEqual([id, keys]);
    const rest = navyCss
      .replace(/\[data-theme="[a-z]+"\]\s*\{[^}]*\}/g, "")
      .replace(/\/\*[\s\S]*?\*\//g, "");
    expect(rest.match(/#[0-9a-fA-F]{3,8}\b|rgba?\(/g)).toBeNull();
    expect(navySource.match(/#[0-9a-fA-F]{3,8}\b/g)).toBeNull();
  });

  // The pairs are listed in docs/templates.md ("Themes of navy-classic").
  for (const id of ["navy", "burgundy", "forest"])
    test(`${id}: light on dark, gold on dark, dark on light and bronze on light are at least 4.5:1`, () => {
      const v = themes[id] ?? {};
      for (const [fg, bg] of [
        ["light", "dark"],
        ["gold", "dark"],
        ["dark", "light"],
        ["bronze", "light"],
      ] as const)
        expect(
          contrast(v[fg] ?? "", v[bg] ?? ""),
          `${id}: ${fg} ${v[fg]} on ${bg} ${v[bg]}`,
        ).toBeGreaterThanOrEqual(4.5);
    });

  test("classic is today's fonts; every family of a set is shipped with a Vietnamese subset; the sets share no family", () => {
    expect(sets["classic"]).toEqual({
      serif: '"Cormorant Garamond", serif',
      caps: '"Be Vietnam Pro", sans-serif',
      italic: "italic",
    });
    const faces = new Set([...fontsCss.matchAll(/font-family:\s*"([^"]+)"/g)].map((m) => m[1]));
    const fam = (set: Record<string, string> = {}) =>
      new Set(["serif", "caps"].map((k) => /^"?([^",]+)"?/.exec(set[k] ?? "")?.[1] ?? ""));
    for (const [id, set] of Object.entries(sets))
      for (const f of fam(set)) {
        expect(faces.has(f), `${id}: ${f} has a @font-face`).toBe(true);
        expect(
          readdirSync(resolve(ROOT, "web/src/book/fonts")).some(
            (x) => x.startsWith(`${f.replace(/ /g, "")}-`) && x.endsWith("-vietnamese.woff2"),
          ),
          `${id}: ${f} Vietnamese file`,
        ).toBe(true);
      }
    for (const f of fam(sets["modern"])) expect(fam(sets["classic"]).has(f), f).toBe(false);
  });

  test("every fixed text of the manifest is a slot of the template, and only those; each has a default wording", () => {
    const used = [...navySource.matchAll(/\bid[=:]\s*[{]?[`"]([a-z0-9$.{}]+)[`"]/g)].flatMap((m) =>
      (m[1] ?? "").includes("${page}")
        ? ["profile", "notes"].map((p) => (m[1] ?? "").replace("${page}", p))
        : [m[1] ?? ""],
    );
    expect([...used].sort()).toEqual(navy.texts.map((t) => t.id).sort());
    expect(Object.keys(tpl.textDefaults ?? {}).sort()).toEqual(navy.texts.map((t) => t.id).sort());
  });

  test("a design this build does not know draws the first theme and font set", () => {
    const d = resolveDesign(
      { theme: "neon", fonts: "comic", sections: { order: [], hidden: [] }, texts: {}, images: {} },
      tpl.design,
    );
    expect([d.theme, d.fonts]).toEqual(["navy", "classic"]);
  });
});
