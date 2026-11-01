// The one draw rule of the free layout (ADR 0004 Q1, "centre"), shared by the book runtime and the editor. A stored box is
// `x`, `w`, `h` in thousandths of the page WIDTH (the shape is the same on every paper), `cy` the vertical centre in thousandths of the page
// HEIGHT and `r` degrees about the centre. It is drawn in design px (the 816-px-wide box of sizes.ts): `left = x * W`, `top = cy * H - h / 2`, then
// moved back inside the page. `unplace` is the inverse; an editor gesture works on the drawn box and writes `unplace(...)` back.
import type { CSSProperties } from "react";
import { designHeight, DESIGN_W, type PageSize } from "../sizes";

export type Box = { x: number; w: number; h: number; cy: number; r?: number };
/** The page a box is drawn on, in design px. */
export type PageBox = { w: number; h: number };
/** A drawn box in design px, origin at the page corner (before the rotation). */
export type Rect = { left: number; top: number; width: number; height: number };

export const pageBox = (size: PageSize): PageBox => ({ w: DESIGN_W, h: designHeight(size) });

/** Moves `pos` so that `[pos, pos + len]` lies inside `[0, max]`; a box longer than the page is centred on it. */
export function inside(pos: number, len: number, max: number): number {
  return len >= max ? (max - len) / 2 : Math.min(Math.max(pos, 0), max - len);
}

/** The box as drawn on `page`, kept inside it (an item near the bottom edge on Letter moves up with the page and is never cut). */
export function place(box: Box, page: PageBox): Rect {
  const width = (box.w * page.w) / 1000;
  const height = (box.h * page.w) / 1000;
  return {
    left: inside((box.x * page.w) / 1000, width, page.w),
    top: inside((box.cy * page.h) / 1000 - height / 2, height, page.h),
    width,
    height,
  };
}

/** The stored form of a rect drawn on `page` (the inverse of `place`; integers, as the server stores them). `r` is not part of the rect. */
export function unplace(rect: Rect, page: PageBox): Omit<Box, "r"> {
  return {
    x: Math.round((rect.left * 1000) / page.w),
    w: Math.round((rect.width * 1000) / page.w),
    h: Math.round((rect.height * 1000) / page.w),
    cy: Math.round(((rect.top + rect.height / 2) * 1000) / page.h),
  };
}

/** The CSS of an absolutely positioned box: the placed rect, rotated about its centre. */
export function boxStyle(box: Box, page: PageBox): CSSProperties {
  const r = place(box, page);
  return {
    position: "absolute",
    left: r.left,
    top: r.top,
    width: r.width,
    height: r.height,
    transform: box.r ? `rotate(${box.r}deg)` : undefined,
  };
}
