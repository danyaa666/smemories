import { readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, test } from "vitest";
import { getTemplate } from "./templates";
import { resolveDesign } from "./design";

// T-142: the design runtime of the memory-book template. The ids the web draws must be the ids of the manifest the server validates against (T-141),
// every theme must keep its text readable, and every font family of a font set must be one that is shipped (fonts.css) with Vietnamese.

const ROOT = resolve(process.cwd(), ".."); // vitest runs in web/
const read = (f: string) => readFileSync(resolve(ROOT, f), "utf8");
const fontsCss = read("web/src/book/fonts/fonts.css");
const memoryCss = read("web/src/book/templates/memory.css");
const manifest = JSON.parse(
  readFileSync(resolve(ROOT, "internal/templates/embed/memory-book.json"), "utf8"),
) as {
  design: { themes: string[]; font_sets: string[]; images: { id: string; choices: string[] }[] };
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
