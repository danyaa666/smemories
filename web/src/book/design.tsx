import { createContext, useContext, useMemo, type ReactNode } from "react";
import type { BookData } from "../api/book";

// The owner's choices for a template (epic E09, T-141 resolves them on the server, T-142 applies them). A template defines what each id looks
// like (memory.css: colours and fonts by `data-theme` and `data-fonts`) and lists the ids it knows; an id this build does not know (a newer manifest
// than the web app) falls back to the template's first one, so a book always draws.

/** The theme and font set ids a template draws. The first of each is the default and the same as in the manifest's `design` block (design.test.ts pins them). */
export type DesignIds = { themes: readonly string[]; fonts: readonly string[] };

export type Design = {
  /** The theme id in use; "" for a template that has none. */
  theme: string;
  /** The font set id in use; "" for a template that has none. */
  fonts: string;
  /** The owner's replacement of a fixed text, else `fallback` (the template's own wording). */
  text: (id: string, fallback: string) => string;
  /** The chosen choice id of a picture slot; undefined when the book has no design. */
  image: (id: string) => string | undefined;
};

/** The page groups of a book in the template's own order: the cover, the owner's profile, the friends' notes and the back. */
export const SECTION_IDS = ["cover", "profile", "notes", "back"] as const;
export type SectionId = (typeof SECTION_IDS)[number];

/**
 * The sections that print, in print order: the design's order without its hidden sections. No design, or an order that is not exactly the four
 * sections (a newer manifest than this build), prints them all in the template's own order.
 */
export function printSections(raw: BookData["design"] | undefined): readonly SectionId[] {
  const order = raw?.sections.order;
  const known =
    order?.length === SECTION_IDS.length && SECTION_IDS.every((s) => order.includes(s))
      ? (order as SectionId[])
      : SECTION_IDS;
  const hidden = raw?.sections.hidden ?? [];
  return known.filter((s) => !hidden.includes(s));
}

export function resolveDesign(raw: BookData["design"] | undefined, ids?: DesignIds): Design {
  const pick = (list: readonly string[] | undefined, id: string | undefined) =>
    list === undefined ? "" : id !== undefined && list.includes(id) ? id : (list[0] ?? "");
  const texts = raw?.texts ?? {};
  const images = raw?.images ?? {};
  return {
    theme: pick(ids?.themes, raw?.theme),
    fonts: pick(ids?.fonts, raw?.fonts),
    text: (id, fallback) => (Object.hasOwn(texts, id) ? texts[id] || fallback : fallback),
    image: (id) => (Object.hasOwn(images, id) ? images[id] : undefined),
  };
}

const NONE = resolveDesign(undefined);
const Ctx = createContext<Design>(NONE);

/** Gives the pages below it the design of one book. Without it (or with no design) every page draws the template's defaults. */
export function DesignProvider({
  design,
  ids,
  children,
}: {
  design: BookData["design"] | undefined;
  ids?: DesignIds;
  children: ReactNode;
}) {
  const value = useMemo(() => resolveDesign(design, ids), [design, ids]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export const useDesign = (): Design => useContext(Ctx);
