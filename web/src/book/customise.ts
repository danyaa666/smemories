import type { BookData } from "../api/book";
import type { DesignBlock, DesignOptions } from "../api/bookDesign";

// The customise screen's working copy of a book's design (epic E09, T-144): what the owner has chosen so far, kept in local state, turned into the
// options document that `book-design/update` validates and into the resolved design the real book render applies.

/** Every choice, with the defaults filled in (the first theme, font set and image choice; the template's section order; nothing hidden; no text replaced). */
export type Draft = {
  theme: string;
  fonts: string;
  order: string[];
  hidden: string[];
  texts: Record<string, string>;
  images: Record<string, string>;
};

const known = (ids: readonly string[], id: unknown): id is string =>
  typeof id === "string" && ids.includes(id);

/** The section order a template starts with. */
export const templateOrder = (block: DesignBlock) => block.sections.map((s) => s.id);

/** Whether `order` is a legal order: every section once, the fixed ones at their place in the template's own order. */
function legalOrder(block: DesignBlock, order: readonly unknown[]): order is string[] {
  const own = templateOrder(block);
  return (
    order.length === own.length &&
    own.every((id) => order.includes(id)) &&
    block.sections.every((s, i) => s.movable || order[i] === s.id)
  );
}

/** The draft of a stored document; anything the template does not offer (a stale id) is replaced by its default, as the server does on read. */
export function draftOf(block: DesignBlock, options: DesignOptions | null | undefined): Draft {
  const o = options ?? {};
  const order = Array.isArray(o.sections?.order) ? o.sections.order : [];
  const hidden = Array.isArray(o.sections?.hidden) ? o.sections.hidden : [];
  const texts: Record<string, string> = {};
  for (const t of block.texts) {
    const v = Object.hasOwn(o.texts ?? {}, t.id) ? o.texts?.[t.id] : undefined;
    if (typeof v === "string" && v !== "" && [...v].length <= t.max) texts[t.id] = v;
  }
  return {
    theme: known(block.themes, o.theme) ? o.theme : (block.themes[0] ?? ""),
    fonts: known(block.font_sets, o.fonts) ? o.fonts : (block.font_sets[0] ?? ""),
    order: legalOrder(block, order) ? order : templateOrder(block),
    hidden: block.sections.filter((s) => s.hideable && hidden.includes(s.id)).map((s) => s.id),
    texts,
    images: Object.fromEntries(
      block.images.map((im) => {
        const v = Object.hasOwn(o.images ?? {}, im.id) ? o.images?.[im.id] : undefined;
        return [im.id, known(im.choices, v) ? v : (im.choices[0] ?? "")];
      }),
    ),
  };
}

/** The whole document `book-design/update` replaces the stored one with: every choice, a text only when it is not empty. */
export function optionsOf(d: Draft): DesignOptions {
  return {
    v: 1,
    theme: d.theme,
    fonts: d.fonts,
    sections: { order: d.order, hidden: d.hidden },
    texts: Object.fromEntries(Object.entries(d.texts).filter(([, v]) => v !== "")),
    images: d.images,
  };
}

/** Whether two drafts would save the same document. */
export const sameDraft = (a: Draft, b: Draft) =>
  JSON.stringify(optionsOf(a)) === JSON.stringify(optionsOf(b));

/** The design the book render takes (`book-data/get` carries the same shape). */
export const resolvedOf = (d: Draft): NonNullable<BookData["design"]> => {
  const o = optionsOf(d);
  return {
    theme: d.theme,
    fonts: d.fonts,
    sections: { order: d.order, hidden: d.hidden },
    texts: o.texts ?? {},
    images: d.images,
  };
};

/**
 * Moves a movable section one step among the movable places (the fixed sections stay where the template puts them, as the server demands).
 * Returns the new order, or null when it cannot move that way.
 */
export function moveSection(
  block: DesignBlock,
  order: readonly string[],
  id: string,
  step: -1 | 1,
): string[] | null {
  const places = block.sections.flatMap((s, i) => (s.movable ? [i] : []));
  const at = order.indexOf(id);
  const p = places.indexOf(at);
  const to = places[p + step];
  if (p < 0 || to === undefined) return null;
  const next = [...order];
  [next[at], next[to]] = [next[to] as string, next[at] as string];
  return next;
}

/** The section a text or image id belongs to ("cover.belongs" is on the cover). */
export const sectionOf = (id: string) => id.split(".")[0] ?? id;

/**
 * The book to preview while a control has the focus: only the pages that control changes (all of them when `focus` is null), the notes section as a
 * single page. The preview is the real render of the draft, so it hides every other section with the design's own `hidden` list.
 */
export function previewOf(
  data: BookData,
  d: Draft,
  block: DesignBlock,
  focus: string | null,
): BookData {
  const design = resolvedOf(d);
  if (focus !== null && block.sections.some((s) => s.id === focus))
    design.sections = { order: d.order, hidden: d.order.filter((id) => id !== focus) };
  return { ...data, design, notes: data.notes.slice(0, 1) };
}

/** The field a server message names: `design options: texts.cover.belongs: must be at most ...` is the text `cover.belongs`. */
export function messageField(message: string): { kind: "texts" | "images"; id: string } | null {
  const m = /\b(texts|images)\.([a-z0-9._-]+?):/.exec(message);
  return m ? { kind: m[1] as "texts" | "images", id: m[2] as string } : null;
}
