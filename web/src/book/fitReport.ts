import type { BookData } from "../api/book";
import type { FitInfo } from "./fit";
import { designZoom, type PageSize } from "./sizes";

/** A friend's note whose text was set in a smaller size, or is still cut off at the smallest size. */
export type NoteFit = {
  id: string;
  /** Position in the book, from 1 (for a note without a name). */
  number: number;
  name: string;
  cut: boolean;
  /** The smallest text size used for this note on paper, in points. */
  pt: number;
};

/** The note id in a fit key: "note:<id>" or "note:<id>:<field>" (a note can have several boxes). */
const noteIdOf = (key: string) => (key.startsWith("note:") ? key.split(":")[1] : undefined);

/** Design px -> points on paper (the book is drawn in a box zoomed to the paper), rounded to half a point. */
export const toPt = (px: number, size: PageSize) =>
  Math.round(px * designZoom(size) * 0.75 * 2) / 2;

/**
 * Which friends' notes were shrunk or cut and how small their text got, from what the fit reported.
 * Cut notes first, then the smallest text first. A note that fits at the designed size is not listed.
 */
export function noteFits(states: Record<string, FitInfo>, data: BookData): NoteFit[] {
  const byId = new Map<string, { cut: boolean; px: number }>();
  for (const [key, info] of Object.entries(states)) {
    const id = noteIdOf(key);
    if (!id || info.state === "ok") continue;
    const prev = byId.get(id);
    byId.set(id, {
      cut: (prev?.cut ?? false) || info.state === "overflow",
      px: Math.min(prev?.px ?? Infinity, info.px),
    });
  }
  const out: NoteFit[] = [];
  data.notes.forEach((n, i) => {
    const f = byId.get(n.id);
    if (f)
      out.push({
        id: n.id,
        number: i + 1,
        name: (n.answers.name ?? "").trim(),
        cut: f.cut,
        pt: toPt(f.px, data.yearbook.page_size),
      });
  });
  return out.sort((a, b) => Number(b.cut) - Number(a.cut) || a.pt - b.pt || a.number - b.number);
}

/** Boxes on the owner's own pages (not friends' notes) by state. */
export function ownFits(states: Record<string, FitInfo>) {
  const own = Object.entries(states).filter(([k]) => noteIdOf(k) === undefined);
  return {
    cut: own.filter(([, i]) => i.state === "overflow").length,
    smaller: own.filter(([, i]) => i.state === "shrunk").length,
  };
}
