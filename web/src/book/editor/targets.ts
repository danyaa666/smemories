// What can be selected on one page of the layout editor and where it is (epic E09 stage 2, T-159). Pure: no React, no DOM.
// A target is an item the student added (a box of the document) or an editable element of the template (`cover.title`). Everything here is in design
// px of the page being edited (the 816-px-wide box, ADR 0004 Q1); the stage turns it into screen px.
import type { LayoutDoc, LayoutItem } from "./document";
import { naturalBox, kindOf, type Natural, type TemplateLayout } from "../layout/runtime";
import { place, unplace, type Box, type PageBox, type Rect } from "../layout/geometry";

export type Target = {
  id: string;
  /** The page kind it is on ("cover", "notes", ...). */
  kind: string;
  /** The item of the document, or undefined for a template element. */
  item?: LayoutItem;
  /** Where it is drawn, in design px, before the rotation. */
  rect: Rect;
  r: number;
  /** An element the student hid (it is not drawn; its place is only where it comes back). */
  hidden: boolean;
};

/** True when the element has an override that moves or hides it; the same test as `Editable` (layout/runtime.tsx) uses to draw it in a box of its own. */
export function isMoved(ov: ElementOverride | undefined): boolean {
  return !!ov && (!!ov.hidden || [ov.x, ov.w, ov.h, ov.cy, ov.r].some((v) => v !== undefined));
}
type ElementOverride = NonNullable<NonNullable<LayoutDoc["pages"][string]["overrides"]>[string]>;

export const overrideOf = (doc: LayoutDoc, id: string) => doc.pages[kindOf(id)]?.overrides?.[id];

/** The box of a template element once its override is applied to the template's own place. */
export function elementBox(nat: Natural, ov: ElementOverride | undefined): Box {
  const base = naturalBox(nat);
  return {
    x: ov?.x ?? base.x,
    w: ov?.w ?? base.w,
    h: ov?.h ?? base.h,
    cy: ov?.cy ?? base.cy,
    r: ov?.r ?? base.r,
  };
}

/**
 * The size of a rotated rectangle from the bounding box a browser measures for it: W = w|cos| + h|sin|, H = w|sin| + h|cos|.
 * A template element that the template itself tilts (the cover photo, -3 degrees) is measured as its bounding box.
 */
export function unrotate(W: number, H: number, deg: number): { w: number; h: number } {
  const a = (deg * Math.PI) / 180;
  const c = Math.abs(Math.cos(a));
  const s = Math.abs(Math.sin(a));
  const det = c * c - s * s;
  if (Math.abs(det) < 0.05) return { w: W, h: H }; // about 45 degrees: the two sizes cannot be told apart
  return { w: (W * c - H * s) / det, h: (H * c - W * s) / det };
}

/**
 * The targets of one page kind in draw order, bottom first: the template's own elements, then the elements the student moved (each is drawn in a box of its
 * own, appended to the page, so their order is the order of the page's DOM: `order` lists the layout ids in that order), then the items in array order.
 */
export function targetsOf(
  doc: LayoutDoc,
  kind: string,
  spec: TemplateLayout | undefined,
  page: PageBox,
  /** The place of an element the template draws itself, as measured on the stage (design px); absent: the template's own natural place. */
  measured: Readonly<Record<string, Rect | undefined>> = {},
  /** The layout ids of the elements the stage found, in the page's DOM order (draw order); an id it did not find yet is drawn last. */
  order: readonly string[] = [],
): Target[] {
  const out: Target[] = [];
  const moved = new Set<string>();
  for (const [id, nat] of Object.entries(spec?.natural ?? {})) {
    if (kindOf(id) !== kind) continue;
    const ov = overrideOf(doc, id);
    const box = elementBox(nat, ov);
    if (isMoved(ov)) moved.add(id);
    const m = isMoved(ov) ? undefined : measured[id];
    out.push({
      id,
      kind,
      rect: m ?? place(box, page),
      r: box.r ?? 0,
      hidden: !!ov?.hidden,
    });
  }
  const rank = (x: Target) => {
    const i = order.indexOf(x.id);
    return i < 0 ? order.length : i;
  };
  out.sort(
    (a, b) =>
      Number(moved.has(a.id)) - Number(moved.has(b.id)) ||
      (moved.has(a.id) ? rank(a) - rank(b) : 0),
  ); // stable: unmoved keep the template's order
  for (const item of doc.pages[kind]?.items ?? [])
    out.push({ id: item.id, kind, item, rect: place(item, page), r: item.r ?? 0, hidden: false });
  return out;
}

/** The stored fields (a patch for `setItemBox` or `setOverride`) of a rect drawn on `page`. */
export const patchOf = (rect: Rect, r: number, page: PageBox) => ({ ...unplace(rect, page), r });
