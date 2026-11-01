// The maths of the editor gestures (T-160, ADR 0004 Q2). Pure: no React, no DOM. Everything is in design px of the page being edited (the 816-px-wide
// box); a rect is the box as drawn (`place`, before the rotation), so a pointer delta divided by the stage's screen-px-per-design-px is added to it
// directly and the result is written back with `unplace` (layout/geometry.ts). The same functions serve the pointer, the keyboard and the panel buttons.
import { inside, type PageBox, type Rect } from "../../layout/geometry";

export type Handle = "nw" | "n" | "ne" | "e" | "se" | "s" | "sw" | "w";
export type Lines = { xs: number[]; ys: number[] };
/** The snap lines a move ended on, to be drawn as guides (design px). */
export type Guides = { x: number[]; y: number[] };

export const SNAP_PX = 6; // screen px
export const MIN_FRAC = 0.02; // smallest side, as a share of the page width
export const NO_GUIDES: Guides = { x: [], y: [] };
const NO_LINES: Lines = { xs: [], ys: [] };

/** Moves `rect` so that it lies inside the page (the size is not changed; a box larger than the page is centred, as `place` draws it). */
export const clampPos = (r: Rect, page: PageBox): Rect => ({
  ...r,
  left: inside(r.left, r.width, page.w),
  top: inside(r.top, r.height, page.h),
});

/** Normalises an angle to (-180, 180]. */
export function normaliseDeg(d: number): number {
  const a = ((d % 360) + 360) % 360;
  return a > 180 ? a - 360 : a;
}

/** The snap lines of a page: its edges, centre lines, safe margin and the drawn edges and centres of the other boxes. Built once at pointer down. */
export function snapLines(page: PageBox, safe: number, others: readonly Rect[]): Lines {
  const xs = [0, safe, page.w / 2, page.w - safe, page.w];
  const ys = [0, safe, page.h / 2, page.h - safe, page.h];
  for (const o of others) {
    xs.push(o.left, o.left + o.width / 2, o.left + o.width);
    ys.push(o.top, o.top + o.height / 2, o.top + o.height);
  }
  return { xs, ys };
}

/** The offset that puts the start, centre or end of `[lo, lo + size]` on the nearest line within `thr`, and that line; null when none is near. */
export function snapAxis(
  lo: number,
  size: number,
  lines: readonly number[],
  thr: number,
): { d: number; line: number } | null {
  let best: { d: number; line: number } | null = null;
  for (const line of lines)
    for (const e of [lo, lo + size / 2, lo + size]) {
      const d = line - e;
      if (Math.abs(d) <= thr && (best === null || Math.abs(d) < Math.abs(best.d)))
        best = { d, line };
    }
  return best;
}

/** A move by `(dx, dy)`: snapped (unless `snap` is false), then kept inside the page. */
export function moveRect(
  r0: Rect,
  dx: number,
  dy: number,
  lines: Lines,
  thr: number,
  page: PageBox,
  snap: boolean,
): { rect: Rect; guides: Guides } {
  let left = r0.left + dx;
  let top = r0.top + dy;
  const guides: Guides = { x: [], y: [] };
  if (snap) {
    const sx = snapAxis(left, r0.width, lines.xs, thr);
    const sy = snapAxis(top, r0.height, lines.ys, thr);
    if (sx) {
      left += sx.d;
      guides.x.push(sx.line);
    }
    if (sy) {
      top += sy.d;
      guides.y.push(sy.line);
    }
  }
  return { rect: clampPos({ ...r0, left, top }, page), guides };
}

const rad = (deg: number) => (deg * Math.PI) / 180;

/**
 * A resize by the pointer delta `(dx, dy)` (page axes) from one of the eight handles of a box rotated by `deg`. The delta is projected onto the box's own
 * axes, the dragged edges move, the opposite edges stay where they are on the page, and the centre follows. Shift keeps the aspect ratio. The sides
 * stay between 2 % of the page width and the page; the position is then kept inside the page.
 */
export function resizeRect(
  r0: Rect,
  deg: number,
  handle: Handle,
  dx: number,
  dy: number,
  keepRatio: boolean,
  page: PageBox,
): Rect {
  const c = Math.cos(rad(deg));
  const s = Math.sin(rad(deg));
  const lx = dx * c + dy * s; // the delta in the box's own axes
  const ly = -dx * s + dy * c;
  const min = MIN_FRAC * page.w;
  const growsW = handle.includes("w") || handle.includes("e");
  const growsH = handle.includes("n") || handle.includes("s");
  let w = r0.width + (handle.includes("e") ? lx : handle.includes("w") ? -lx : 0);
  let h = r0.height + (handle.includes("s") ? ly : handle.includes("n") ? -ly : 0);
  if (keepRatio) {
    const lo = Math.max(min / r0.width, min / r0.height);
    const hi = Math.min(page.w / r0.width, page.h / r0.height);
    const sw = w / r0.width;
    const sh = h / r0.height;
    const f = Math.min(hi, Math.max(lo, growsW && growsH ? Math.max(sw, sh) : growsW ? sw : sh));
    w = r0.width * f;
    h = r0.height * f;
  } else {
    w = Math.min(page.w, Math.max(min, w));
    h = Math.min(page.h, Math.max(min, h));
  }
  // The edges opposite the handle keep their place; an axis the handle does not touch (the other axis of an edge handle) grows about the middle.
  // Local coordinates: the old box is [-width/2, width/2] x [-height/2, height/2] about its centre.
  let l = -w / 2;
  let r = w / 2;
  let t = -h / 2;
  let b = h / 2;
  if (growsW) {
    if (handle.includes("w")) {
      r = r0.width / 2;
      l = r - w;
    } else {
      l = -r0.width / 2;
      r = l + w;
    }
  }
  if (growsH) {
    if (handle.includes("n")) {
      b = r0.height / 2;
      t = b - h;
    } else {
      t = -r0.height / 2;
      b = t + h;
    }
  }
  const mx = (l + r) / 2; // the new centre in the box's own axes, relative to the old one
  const my = (t + b) / 2;
  const cx = r0.left + r0.width / 2 + mx * c - my * s;
  const cy = r0.top + r0.height / 2 + mx * s + my * c;
  return clampPos({ left: cx - w / 2, top: cy - h / 2, width: w, height: h }, page);
}

/** The angle of the pointer seen from the centre, in degrees (0 is to the right, clockwise on the screen). */
export const pointerAngle = (cx: number, cy: number, px: number, py: number) =>
  (Math.atan2(py - cy, px - cx) * 180) / Math.PI;

/** The rotation after the pointer turned from angle `a0` to `a1` about the centre: relative to where it was grabbed, whole degrees, Shift snaps to 15. */
export function rotateTo(deg0: number, a0: number, a1: number, snap15: boolean): number {
  let d = deg0 + (a1 - a0);
  if (snap15) d = Math.round(d / 15) * 15;
  return normaliseDeg(Math.round(d));
}

export type Nudge =
  | { kind: "move"; dx: number; dy: number }
  | { kind: "resize"; dw: number; dh: number }
  | { kind: "rotate"; ddeg: number };

/**
 * One keyboard or button step: the move, the resize (the bottom right corner follows, along the box's own axes) or the rotation, with the same
 * clamps as the pointer. Snapping is off: a box sitting on a guide must be able to leave it by one pixel.
 */
export function nudge(
  rect: Rect,
  deg: number,
  n: Nudge,
  page: PageBox,
): { rect: Rect; deg: number } {
  if (n.kind === "move")
    return { rect: moveRect(rect, n.dx, n.dy, NO_LINES, 0, page, false).rect, deg };
  if (n.kind === "rotate") return { rect, deg: normaliseDeg(Math.round(deg + n.ddeg)) };
  const c = Math.cos(rad(deg));
  const s = Math.sin(rad(deg));
  const dx = n.dw * c - n.dh * s; // the step is along the box's own axes
  const dy = n.dw * s + n.dh * c;
  return { rect: resizeRect(rect, deg, "se", dx, dy, false, page), deg };
}
