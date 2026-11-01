// Drag, resize, rotate and the keyboard steps of the layout editor (T-160, ADR 0004 Q2). Pointer Events with a pointerId filter and pointer capture;
// the box as drawn on the edited page is the start, the pointer delta is converted with the stage's current scale (read from the page element's bounding
// box, so zoom and fit-to-window are exact), snapped, clamped, and written back with `unplace` through the T-158 store: `begin()` at pointer down,
// `preview()` on every pointer move (at most one per animation frame), one `commit()` at pointer up. Escape or a cancelled pointer restores the start.
import type { TFunction } from "i18next";
import {
  useEffect,
  useLayoutEffect,
  useState,
  type KeyboardEvent as ReactKey,
  type PointerEvent as ReactPointer,
  type RefObject,
} from "react";
import type { PageBox, Rect } from "../../layout/geometry";
import { removeItem, setItemBox, setOverride, type Op, type Refusal } from "../document";
import { patchOf, type Target } from "../targets";
import { nudgeOf } from "./keys";
import {
  moveRect,
  NO_GUIDES,
  nudge,
  pointerAngle,
  resizeRect,
  rotateTo,
  snapLines,
  SNAP_PX,
  type Guides,
  type Handle,
  type Lines,
  type Nudge,
} from "./math";

export type GestureStore = {
  begin: () => void;
  preview: (op: Op) => Refusal | undefined;
  commit: () => void;
  cancel: () => void;
  apply: (op: Op) => Refusal | undefined;
  select: (ids: readonly string[]) => void;
};

/** The op that writes a drawn box (and rotation) of a target: an item through `setItemBox`, a template element as the whole box of its override. */
export const boxOp = (x: Target, rect: Rect, deg: number, pb: PageBox): Op =>
  x.item
    ? setItemBox(x.kind, x.id, patchOf(rect, deg, pb))
    : setOverride(x.id, patchOf(rect, deg, pb));

const pct = (v: number) => String(Math.round(v * 10) / 10);

/** What a change says in words: the new place (% of the page from the left and top), size or angle. */
export function describe(
  t: TFunction,
  what: "move" | "resize" | "rotate",
  rect: Rect,
  deg: number,
  pb: PageBox,
  lead?: string,
): string {
  const key = { move: "movedTo", resize: "resized", rotate: "rotated" }[what];
  return t(`editor.said.${key}`, {
    lead: lead ?? "",
    x: pct((rect.left / pb.w) * 100),
    y: pct((rect.top / pb.h) * 100),
    w: pct((rect.width / pb.w) * 100),
    h: pct((rect.height / pb.h) * 100),
    deg,
  }).trim();
}

export type Mode = "move" | "resize" | "rotate";
type Pointer = { x: number; y: number; alt: boolean; shift: boolean };
type Gesture = {
  pid: number;
  target: Target;
  mode: Mode;
  handle: Handle | undefined;
  r0: Rect;
  deg0: number;
  from: Pointer;
  k: number; // screen px per design px
  centre: { x: number; y: number }; // screen px, for the rotation
  a0: number;
  lines: Lines;
  slop: number;
  moved: boolean;
  last: Pointer;
  raf: number;
  out: { rect: Rect; deg: number };
};
type Burst = { id: string; rect: Rect; deg: number; timer: number };

const MOUSE_SLOP = 3; // screen px before a press becomes a drag (the delta still counts from the press, so there is no dead zone in the result)
const TOUCH_SLOP = 8;
const BURST_MS = 600; // keys closer together than this are one history step

export type GestureOpts = {
  targets: readonly Target[];
  pb: PageBox;
  /** The safe margin in design px. */
  safe: number;
  selection: readonly string[];
  readOnly: boolean;
  /** The element that is exactly the page on screen (its bounding box gives the scale). */
  pageEl: RefObject<HTMLElement | null>;
  store: GestureStore;
  announce: (s: string) => void;
  t: TFunction;
};

/**
 * The state machine behind the hook: one gesture at a time (a second pointer waits), and bursts of key steps. A class, because it is all imperative
 * (window listeners, animation frames) and has to keep one identity for the life of the stage; `o` is replaced on every render.
 */
class Controller {
  o!: GestureOpts;
  private cur: Gesture | null = null;
  private burst: Burst | null = null;
  private dragged = false;

  constructor(private setGuides: (g: Guides) => void) {}

  /** True just after a drag: the click that follows the release is not a new selection. */
  justDragged = () => this.dragged;

  /** Pointer down on a target (move) or one of its handles (resize, rotate). */
  down = (e: ReactPointer, x: Target, mode: Mode, handle?: Handle) => {
    const c = this.o;
    if (c.readOnly || x.hidden || this.cur) return; // a second pointer waits until the first has ended
    if (e.pointerType === "mouse" && e.button !== 0) return;
    this.flush();
    if (mode === "move" && !c.selection.includes(x.id)) {
      if (e.pointerType === "touch") return; // touch: the tap (its click) selects; a swipe over an unselected item pans the stage and selects nothing
      c.store.select([x.id]);
    }
    const page = c.pageEl.current?.getBoundingClientRect();
    if (!page || page.width <= 0) return;
    const k = page.width / c.pb.w;
    try {
      e.currentTarget.setPointerCapture(e.pointerId);
    } catch {
      // a synthetic or already-ended pointer: the window listeners still follow it
    }
    const centre = {
      x: page.left + (x.rect.left + x.rect.width / 2) * k,
      y: page.top + (x.rect.top + x.rect.height / 2) * k,
    };
    const from = { x: e.clientX, y: e.clientY, alt: e.altKey, shift: e.shiftKey };
    const others = c.targets.filter((y) => y.id !== x.id && !y.hidden).map((y) => y.rect);
    this.cur = {
      pid: e.pointerId,
      target: x,
      mode,
      handle,
      r0: x.rect,
      deg0: x.r,
      from,
      k,
      centre,
      a0: pointerAngle(centre.x, centre.y, from.x, from.y),
      lines: mode === "move" ? snapLines(c.pb, c.safe, others) : { xs: [], ys: [] }, // indexed once, here
      slop: e.pointerType === "mouse" ? MOUSE_SLOP : TOUCH_SLOP,
      moved: false,
      last: from,
      raf: 0,
      out: { rect: x.rect, deg: x.r },
    };
    c.store.begin();
    window.addEventListener("pointermove", this.move);
    window.addEventListener("pointerup", this.up);
    window.addEventListener("pointercancel", this.abort);
    window.addEventListener("keydown", this.esc, true);
  };

  private move = (e: PointerEvent) => {
    const g = this.cur;
    if (!g || e.pointerId !== g.pid) return;
    g.last = { x: e.clientX, y: e.clientY, alt: e.altKey, shift: e.shiftKey };
    if (!g.moved) {
      if (Math.hypot(g.last.x - g.from.x, g.last.y - g.from.y) < g.slop) return;
      g.moved = true;
    }
    if (g.raf) return;
    g.raf = requestAnimationFrame(() => {
      g.raf = 0;
      if (this.cur === g) this.preview(g);
    });
  };
  private up = (e: PointerEvent) => {
    const g = this.cur;
    if (!g || e.pointerId !== g.pid) return;
    g.last = { x: e.clientX, y: e.clientY, alt: e.altKey, shift: e.shiftKey };
    if (!g.moved && Math.hypot(g.last.x - g.from.x, g.last.y - g.from.y) >= g.slop) g.moved = true;
    if (g.moved) this.preview(g);
    this.end(true);
  };
  private abort = (e: PointerEvent) => {
    if (this.cur && e.pointerId === this.cur.pid) this.end(false);
  };
  private esc = (e: KeyboardEvent) => {
    if (e.key !== "Escape") return;
    e.stopPropagation(); // the gesture is cancelled; the selection stays
    e.preventDefault();
    this.end(false);
  };

  private preview(g: Gesture) {
    const { pb, store } = this.o;
    const p = g.last;
    const dx = (p.x - g.from.x) / g.k;
    const dy = (p.y - g.from.y) / g.k;
    let rect = g.r0;
    let deg = g.deg0;
    let guides = NO_GUIDES;
    if (g.mode === "move") {
      const m = moveRect(g.r0, dx, dy, g.lines, SNAP_PX / g.k, pb, !p.alt);
      rect = m.rect;
      guides = m.guides;
    } else if (g.mode === "resize") {
      rect = resizeRect(g.r0, g.deg0, g.handle!, dx, dy, p.shift, pb);
    } else {
      deg = rotateTo(g.deg0, g.a0, pointerAngle(g.centre.x, g.centre.y, p.x, p.y), p.shift);
    }
    g.out = { rect, deg };
    store.preview(boxOp(g.target, rect, deg, pb));
    this.setGuides(guides);
  }

  private end(commit: boolean) {
    const g = this.cur;
    if (!g) return;
    this.cur = null;
    cancelAnimationFrame(g.raf);
    window.removeEventListener("pointermove", this.move);
    window.removeEventListener("pointerup", this.up);
    window.removeEventListener("pointercancel", this.abort);
    window.removeEventListener("keydown", this.esc, true);
    this.setGuides(NO_GUIDES);
    const { store, announce, t, pb } = this.o;
    if (commit && g.moved) {
      this.dragged = true;
      setTimeout(() => (this.dragged = false), 0);
      store.commit();
      announce(describe(t, g.mode, g.out.rect, g.out.deg, pb));
    } else {
      store.cancel();
      if (g.moved) announce(t("editor.said.cancelled"));
    }
  }

  /** A key on a focused target: the same maths as the pointer through `nudge`; a burst of keys is one history step. */
  key = (e: ReactKey, x: Target) => {
    const c = this.o;
    if (e.ctrlKey || e.metaKey) return this.flush(); // undo and redo must see the burst as committed
    if (c.readOnly || this.cur || x.hidden) return;
    if ((e.key === "Delete" || e.key === "Backspace") && x.item) {
      e.preventDefault();
      this.flush();
      if (!c.store.apply(removeItem(x.kind, x.id))) {
        c.store.select([]);
        c.announce(c.t("editor.deleted"));
        c.pageEl.current?.focus(); // the focused button is gone: keep the keyboard on the stage
      }
      return;
    }
    const n: Nudge | null = nudgeOf(e);
    if (!n) return;
    e.preventDefault();
    let b = this.burst;
    if (!b || b.id !== x.id) {
      this.flush();
      c.store.begin();
      b = this.burst = { id: x.id, rect: x.rect, deg: x.r, timer: 0 };
    }
    const next = nudge(b.rect, b.deg, n, c.pb);
    b.rect = next.rect;
    b.deg = next.deg;
    c.store.preview(boxOp(x, next.rect, next.deg, c.pb));
    clearTimeout(b.timer);
    b.timer = window.setTimeout(this.flush, BURST_MS);
    const lead =
      n.kind === "move"
        ? c.t("editor.said.leadMove", {
            dir: c.t(`editor.dir.${dirOf(n.dx, n.dy)}`),
            n: Math.abs(n.dx || n.dy),
          })
        : undefined;
    c.announce(describe(c.t, n.kind, next.rect, next.deg, c.pb, lead));
  };

  /** Ends a burst of key steps as one history step. */
  flush = () => {
    const b = this.burst;
    if (!b) return;
    clearTimeout(b.timer);
    this.burst = null;
    this.o.store.commit();
  };

  /** The options of the latest render. */
  use(o: GestureOpts) {
    this.o = o;
  }

  dispose() {
    this.flush();
    this.end(false);
  }
}

const dirOf = (dx: number, dy: number) =>
  dx < 0 ? "left" : dx > 0 ? "right" : dy < 0 ? "up" : "down";

/** What a box needs of the gestures. One object for the life of the stage, so memoised boxes do not draw again when the gesture state changes. */
export type Api = Pick<Controller, "down" | "key" | "justDragged"> & { blur: () => void };

export function useGestures(o: GestureOpts): { guides: Guides; api: Api } {
  const [guides, setGuides] = useState<Guides>(NO_GUIDES);
  const [ctl] = useState(() => new Controller(setGuides));
  const [api] = useState<Api>(() => ({
    down: ctl.down,
    key: ctl.key,
    justDragged: ctl.justDragged,
    blur: ctl.flush,
  }));
  useLayoutEffect(() => {
    ctl.use(o);
  });
  useEffect(() => ctl.flush, [o.selection, ctl]); // another selection ends a burst
  useEffect(() => () => ctl.dispose(), [ctl]);
  return { guides, api };
}
