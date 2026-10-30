import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, test } from "vitest";
import type { BookData } from "../api/book";
import type { DesignBlock } from "../api/bookDesign";
import en from "../locales/en.json";
import vi from "../locales/vi.json";
import {
  draftOf,
  messageField,
  moveSection,
  optionsOf,
  previewOf,
  sameDraft,
  sectionOf,
} from "./customise";
import { pageCount } from "./BookPages";
import { getTemplate } from "./templates";

// T-144: the working copy of a design and its translation to the document the server validates.

const manifest = JSON.parse(
  readFileSync(resolve(process.cwd(), "../internal/templates/embed/memory-book.json"), "utf8"),
).design as DesignBlock;

describe("draftOf and optionsOf", () => {
  test("no stored design is the template's defaults", () => {
    const d = draftOf(manifest, null);
    expect(d).toEqual({
      theme: "lavender",
      fonts: "playful",
      order: ["cover", "profile", "notes", "back"],
      hidden: [],
      texts: {},
      images: { "cover.pattern": "dots" },
    });
    expect(optionsOf(d)).toEqual({
      v: 1,
      theme: "lavender",
      fonts: "playful",
      sections: { order: ["cover", "profile", "notes", "back"], hidden: [] },
      texts: {},
      images: { "cover.pattern": "dots" },
    });
  });

  test("a stored design is read back, and what the template no longer offers falls back to the default", () => {
    const d = draftOf(manifest, {
      v: 1,
      theme: "peach",
      fonts: "gone",
      sections: {
        order: ["cover", "notes", "profile", "back"],
        hidden: ["profile", "cover", "gone"],
      },
      texts: { "cover.belongs": "Của Hồng", "gone.text": "x", "back.pact": "x".repeat(19) },
      images: { "cover.pattern": "plain", "gone.image": "x" },
    });
    expect(d.theme).toBe("peach");
    expect(d.fonts).toBe("playful");
    expect(d.order).toEqual(["cover", "notes", "profile", "back"]);
    expect(d.hidden).toEqual(["profile"]); // the cover is not hideable
    expect(d.texts).toEqual({ "cover.belongs": "Của Hồng" }); // a text over the manifest's max is dropped
    expect(d.images).toEqual({ "cover.pattern": "plain" });
  });

  test("an order that moves a fixed section is replaced by the template's own", () => {
    expect(
      draftOf(manifest, { sections: { order: ["notes", "cover", "profile", "back"] } }).order,
    ).toEqual(["cover", "profile", "notes", "back"]);
  });

  test("the document leaves out empty texts, and the same document is not a change", () => {
    const a = draftOf(manifest, null);
    const b = { ...a, texts: { "cover.belongs": "" } };
    expect(optionsOf(b).texts).toEqual({});
    expect(sameDraft(a, b)).toBe(true);
    expect(sameDraft(a, { ...a, theme: "mint" })).toBe(false);
  });
});

describe("moveSection", () => {
  const order = ["cover", "profile", "notes", "back"];
  test("movable sections swap places; fixed ones never move or get passed", () => {
    expect(moveSection(manifest, order, "notes", -1)).toEqual([
      "cover",
      "notes",
      "profile",
      "back",
    ]);
    expect(moveSection(manifest, order, "profile", 1)).toEqual([
      "cover",
      "notes",
      "profile",
      "back",
    ]);
    expect(moveSection(manifest, order, "profile", -1)).toBeNull();
    expect(moveSection(manifest, order, "notes", 1)).toBeNull();
    expect(moveSection(manifest, order, "cover", 1)).toBeNull();
    expect(moveSection(manifest, order, "back", -1)).toBeNull();
  });
  test("with a movable section between two fixed ones the places are the movable ones only", () => {
    const b: DesignBlock = {
      ...manifest,
      sections: [
        { id: "a", hideable: false, movable: false, fixed: "first" },
        { id: "b", hideable: true, movable: true, fixed: null },
        { id: "c", hideable: true, movable: false, fixed: null },
        { id: "d", hideable: true, movable: true, fixed: null },
      ],
    };
    expect(moveSection(b, ["a", "b", "c", "d"], "d", -1)).toEqual(["a", "d", "c", "b"]);
  });
});

describe("previewOf", () => {
  const data = {
    design: null,
    yearbook: {
      template_id: "memory-book",
      page_size: "A5",
      language: "vi",
      title: "T",
      cover_media_id: null,
    },
    profile: { photo_media_id: null },
    notes: [1, 2, 3].map((i) => ({
      id: `n${i}`,
      answers: { name: `B${i}`, message: "m" },
      signature: null,
      photo_ids: [],
    })),
  } as unknown as BookData;
  const d = draftOf(manifest, { sections: { hidden: ["profile"] } });

  test("shows one notes page, and with no focus what prints", () => {
    const p = previewOf(data, d, manifest, null);
    expect(p.notes).toHaveLength(1);
    expect(pageCount(p)).toBe(3); // cover, one notes page, back
  });
  test("a focused control shows its own section even when that section is hidden", () => {
    expect(pageCount(previewOf(data, d, manifest, "profile"))).toBe(1);
    expect(pageCount(previewOf(data, d, manifest, "notes"))).toBe(1);
    expect(pageCount(previewOf(data, d, manifest, "cover"))).toBe(1);
  });
  test("the preview takes the draft's theme, texts and images", () => {
    const p = previewOf(
      data,
      { ...d, theme: "mint", texts: { "back.pact": "Hẹn" } },
      manifest,
      "back",
    );
    expect(p.design).toMatchObject({
      theme: "mint",
      texts: { "back.pact": "Hẹn" },
      images: { "cover.pattern": "dots" },
    });
  });
});

describe("messageField", () => {
  test("names the text a server message is about", () => {
    expect(
      messageField("design options: texts.cover.belongs: must be at most 40 characters"),
    ).toEqual({
      kind: "texts",
      id: "cover.belongs",
    });
    expect(messageField("design options: images.cover.pattern: unknown choice")).toEqual({
      kind: "images",
      id: "cover.pattern",
    });
    expect(messageField("design options: theme: unknown id")).toBeNull();
  });
  test("sectionOf", () => expect(sectionOf("back.pact")).toBe("back"));
});

// The names the screen shows come from the locale files, so every id of the manifest needs one in both languages; the placeholders need the template's wording.
describe("memory-book customise names (pinned to the manifest)", () => {
  const get = (o: unknown, path: string) =>
    path.split(".").reduce<unknown>((x, k) => (x as Record<string, unknown> | undefined)?.[k], o);
  const names = (locale: unknown) => {
    const key = (rest: string) => get(locale, `customise.tpl.memory-book.${rest}`);
    return [
      ...manifest.themes.map((t) => key(`themes.${t}`)),
      ...manifest.font_sets.map((f) => key(`fonts.${f}`)),
      ...manifest.sections.map((s) => key(`sections.${s.id}`)),
      ...manifest.texts.map((t) => key(`texts.${t.id.replaceAll(".", "_")}`)),
      ...manifest.images.map((i) => key(`images.${i.id.replaceAll(".", "_")}.label`)),
      ...manifest.images.flatMap((i) =>
        i.choices.map((c) => key(`images.${i.id.replaceAll(".", "_")}.choices.${c}`)),
      ),
    ];
  };
  test.each([
    ["en", en],
    ["vi", vi],
  ])("%s has a name for every theme, font set, section, text and image choice", (_, locale) => {
    for (const n of names(locale)) expect(typeof n === "string" && n !== "").toBe(true);
  });
  test("every text has its default wording key, in both languages", () => {
    const defaults = getTemplate("memory-book").textDefaults ?? {};
    expect(Object.keys(defaults).sort()).toEqual(manifest.texts.map((t) => t.id).sort());
    for (const key of Object.values(defaults))
      for (const locale of [en, vi]) expect(typeof get(locale, key)).toBe("string");
  });
  test("the pact is short enough to stay at full size (T-143 QA)", () => {
    expect(manifest.texts.find((t) => t.id === "back.pact")?.max).toBe(18);
  });
});
