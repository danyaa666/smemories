// The pure operations on the layout document (v1) of the free-layout editor (epic E09 stage 2, T-158; ADR 0004; server rules in
// internal/booklayout/validate.go). Framework-free: every operation takes a document and returns a new one that shares what did not change by
// reference (so a history snapshot is just the old object). An operation that breaks a limit returns the SAME document and a typed reason.
// The limits are a client mirror so the editor can say no at once; the server stays the authority.
import type { LayoutDoc, LayoutItem } from "../../api/bookLayout";

export type { LayoutDoc, LayoutItem };
type Page = LayoutDoc["pages"][string];
type Bg = NonNullable<Page["bg"]>;
type Override = NonNullable<Page["overrides"]>[string];
export type Box = { x: number; w: number; h: number; cy: number; r?: number };

export const MAX_ITEMS_PAGE = 40;
export const MAX_ITEMS_TOTAL = 150;
export const MAX_TEXT = 500; // code points
export const MIN_BOX = 10;
export const MAX_BOX = 2000;
export const RATIO = 1419; // A5 height over width in thousandths, as the server (pageRatioPermille): a box legal on any paper passes

export type Refusal =
  | "too_many_on_page"
  | "too_many_in_total"
  | "text_too_long"
  | "bad_text"
  | "bad_box"
  | "bad_id"
  | "duplicate_id"
  | "no_such_item"
  | "bad_bg"
  | "bad_value";
export type OpResult = { doc: LayoutDoc; refused?: Refusal };
/** An operation as the history and the hook take it. */
export type Op = (doc: LayoutDoc) => OpResult;

export const emptyDoc = (): LayoutDoc => ({ v: 1, pages: {} });

const refuse = (doc: LayoutDoc, refused: Refusal): OpResult => ({ doc, refused });
const ok = (doc: LayoutDoc): OpResult => ({ doc });
const page = (doc: LayoutDoc, kind: string): Page => doc.pages[kind] ?? {};
const withPage = (doc: LayoutDoc, kind: string, p: Page): LayoutDoc => ({
  ...doc,
  pages: { ...doc.pages, [kind]: p },
});
const items = (doc: LayoutDoc, kind: string) => page(doc, kind).items ?? [];
const total = (doc: LayoutDoc) =>
  Object.values(doc.pages).reduce((n, p) => n + (p.items?.length ?? 0), 0);

/** The page kind of a template element id: the part before the first dot ("cover.photo" is on the cover). */
const kindOfElement = (id: string) => id.split(".")[0] ?? "";

const int = (n: unknown): n is number => typeof n === "number" && Number.isInteger(n);

/** True when every given field of a stored box is inside the server's limits. `full` boxes (items) need x, w, h and cy. */
function boxOk(b: Partial<Box>, full: boolean): boolean {
  for (const k of ["x", "w", "h", "cy", "r"] as const) {
    const v = b[k];
    if (v === undefined ? full && k !== "r" : !int(v)) return false;
  }
  if (b.w !== undefined && (b.w < MIN_BOX || b.w > MAX_BOX)) return false;
  if (b.h !== undefined && (b.h < MIN_BOX || b.h > MAX_BOX)) return false;
  if (b.r !== undefined && (b.r < -180 || b.r > 180)) return false;
  if (b.x !== undefined) {
    // at most half the box may leave the page (doubled units, as the server); without w, the widest or narrowest box
    const lo = b.w === undefined ? -MAX_BOX : -b.w;
    const hi = b.w === undefined ? 2000 - MIN_BOX : 2000 - b.w;
    if (2 * b.x < lo || 2 * b.x > hi) return false;
  }
  if (b.cy !== undefined) {
    const slack = Math.floor(((b.h ?? MAX_BOX) * RATIO) / 2000);
    if (b.cy < -slack || b.cy > 1000 + slack) return false;
  }
  return true;
}

/** Rounds the box fields of a gesture (a drag gives fractions) before they are checked and stored. */
const rounded = <T extends Partial<Box>>(b: T): T => {
  const out: Partial<Box> = { ...b };
  for (const k of ["x", "w", "h", "cy", "r"] as const) {
    const v = out[k];
    if (typeof v === "number" && Number.isFinite(v)) out[k] = Math.round(v);
  }
  return out as T;
};

// Go's strings.TrimSpace (unicode.IsSpace): unlike JS trim() it keeps U+FEFF and also trims U+0085.
const GO_EDGE_SPACE =
  /^[\t-\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\t-\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/g;
// Refused characters: Cc except \n, Cf except the joiner U+200D, Zl, Zp, U+FFFD, and lone surrogates (JSON turns them into U+FFFD).
const BAD_TEXT_CHAR = /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}\p{Cs}\uFFFD]/u;

/**
 * Plain text, mirroring the server: internal/booklayout/validate.go (text) with internal/textx CleanMultiline is the source of truth.
 * Order as there: \r\n -> \n, NFC, trim, refuse the characters above (bad_text), then at most 500 code points (text_too_long).
 */
function textRefusal(text: string): Refusal | undefined {
  const clean = text.replace(/\r\n/g, "\n").normalize("NFC").replace(GO_EDGE_SPACE, "");
  if (BAD_TEXT_CHAR.test(clean.replace(/\n|\u200d/g, ""))) return "bad_text";
  if ([...clean].length > MAX_TEXT) return "text_too_long";
  return undefined;
}

const find = (doc: LayoutDoc, kind: string, id: string) =>
  items(doc, kind).findIndex((i) => i.id === id);
const replaceAt = <T>(list: T[], i: number, v: T) => list.map((x, j) => (j === i ? v : x));

/** A fresh item id: 12 characters of [A-Za-z0-9] (the server wants 6..26). */
export function newItemId(): string {
  const a = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
  const bytes = crypto.getRandomValues(new Uint8Array(12));
  return Array.from(bytes, (b) => a[b % a.length]).join("");
}

/** Moves, resizes or rotates an added item (any subset of x, w, h, cy, r). */
export const setItemBox =
  (kind: string, id: string, patch: Partial<Box>): Op =>
  (doc) => {
    const i = find(doc, kind, id);
    if (i < 0) return refuse(doc, "no_such_item");
    const next = { ...items(doc, kind)[i]!, ...rounded(patch) };
    if (!boxOk(next, true)) return refuse(doc, "bad_box");
    return ok(
      withPage(doc, kind, { ...page(doc, kind), items: replaceAt(items(doc, kind), i, next) }),
    );
  };

/** Adds an item on top of the page (the end of the array is drawn last). `item.id` comes from `newItemId()`. */
export const addItem =
  (kind: string, item: LayoutItem): Op =>
  (doc) => {
    if (!/^[A-Za-z0-9]{6,26}$/.test(item.id)) return refuse(doc, "bad_id");
    if (Object.values(doc.pages).some((p) => p.items?.some((i) => i.id === item.id)))
      return refuse(doc, "duplicate_id");
    if (items(doc, kind).length >= MAX_ITEMS_PAGE) return refuse(doc, "too_many_on_page");
    if (total(doc) >= MAX_ITEMS_TOTAL) return refuse(doc, "too_many_in_total");
    const next = { ...item, ...rounded(item) };
    if (!boxOk(next, true)) return refuse(doc, "bad_box");
    if (next.t === "text") {
      const r = textRefusal(next.text ?? "");
      if (r) return refuse(doc, r);
    }
    return ok(withPage(doc, kind, { ...page(doc, kind), items: [...items(doc, kind), next] }));
  };

export const removeItem =
  (kind: string, id: string): Op =>
  (doc) => {
    const i = find(doc, kind, id);
    if (i < 0) return refuse(doc, "no_such_item");
    return ok(
      withPage(doc, kind, {
        ...page(doc, kind),
        items: items(doc, kind).filter((_, j) => j !== i),
      }),
    );
  };

/** Moves an item to `toIndex` of the draw order (0 is the back, length - 1 the front). */
export const reorderItem =
  (kind: string, id: string, toIndex: number): Op =>
  (doc) => {
    const list = items(doc, kind);
    const i = find(doc, kind, id);
    if (i < 0) return refuse(doc, "no_such_item");
    if (!int(toIndex) || toIndex < 0 || toIndex >= list.length) return refuse(doc, "bad_value");
    if (toIndex === i) return ok(doc);
    const next = list.filter((_, j) => j !== i);
    next.splice(toIndex, 0, list[i]!);
    return ok(withPage(doc, kind, { ...page(doc, kind), items: next }));
  };

/** Moves, resizes, rotates or hides a template element by its layout id ("cover.photo"); the patch is merged into the earlier override. */
export const setOverride =
  (elementId: string, patch: Partial<Box> & { hidden?: boolean }): Op =>
  (doc) => {
    const kind = kindOfElement(elementId);
    if (!kind || kind === elementId || elementId.endsWith(".")) return refuse(doc, "bad_id");
    if (patch.hidden !== undefined && typeof patch.hidden !== "boolean")
      return refuse(doc, "bad_value");
    const p = page(doc, kind);
    const next: Override = { ...p.overrides?.[elementId], ...rounded(patch) };
    if (!boxOk(next, false)) return refuse(doc, "bad_box");
    return ok(withPage(doc, kind, { ...p, overrides: { ...p.overrides, [elementId]: next } }));
  };

/** Drops the override of a template element: it goes back to the template's own place and shows again (T-159 "reset this element"). */
export const clearOverride =
  (elementId: string): Op =>
  (doc) => {
    const kind = kindOfElement(elementId);
    const p = page(doc, kind);
    if (!p.overrides || !Object.hasOwn(p.overrides, elementId)) return ok(doc);
    const rest = { ...p.overrides };
    delete rest[elementId];
    const next = { ...p };
    if (Object.keys(rest).length > 0) next.overrides = rest;
    else delete next.overrides;
    return ok(withPage(doc, kind, next));
  };

/** Sets the page background: exactly one of `asset`, `media`, `color`, or null to remove it. */
export const setBg =
  (kind: string, bg: Bg | null): Op =>
  (doc) => {
    const p = page(doc, kind);
    if (bg === null) {
      if (!p.bg) return ok(doc);
      const rest = { ...p };
      delete rest.bg;
      return ok(withPage(doc, kind, rest));
    }
    const set = [bg.asset, bg.media, bg.color].filter((v) => typeof v === "string" && v !== "");
    if (set.length !== 1 || Object.keys(bg).length !== 1) return refuse(doc, "bad_bg");
    return ok(withPage(doc, kind, { ...p, bg }));
  };

type TextPatch = Partial<Pick<LayoutItem, "text" | "font" | "size" | "color" | "align" | "bold">>;

/** Changes the text or the text style of a text item. */
export const setText =
  (kind: string, id: string, patch: TextPatch): Op =>
  (doc) => {
    const i = find(doc, kind, id);
    if (i < 0) return refuse(doc, "no_such_item");
    const item = items(doc, kind)[i]!;
    if (item.t !== "text") return refuse(doc, "bad_value");
    if (patch.text !== undefined) {
      const r = textRefusal(patch.text);
      if (r) return refuse(doc, r);
    }
    if (patch.size !== undefined && !int(patch.size)) return refuse(doc, "bad_value");
    return ok(
      withPage(doc, kind, {
        ...page(doc, kind),
        items: replaceAt(items(doc, kind), i, { ...item, ...patch }),
      }),
    );
  };

export const ops = {
  setItemBox,
  addItem,
  removeItem,
  reorderItem,
  setOverride,
  clearOverride,
  setBg,
  setText,
};
