// The model of the Templates panel (T-175): the list grouped for display, the per-kind cache, the apply flow and its errors. No React here, so the
// rules are tested with a fake API (templates.test.ts). The routes are in api/bookLayout.ts (T-172, T-173).
import { ApiError } from "../../api/client";
import type {
  LayoutConflict,
  LayoutDoc,
  LayoutRestored,
  PageTemplate,
  TemplateSource,
} from "../../api/bookLayout";
import { historyError, type HistoryError } from "./versions";

export const MAX_KINDS_CACHED = 2; // list bodies of up to 40 items per template, at most 20 own ones
export const MAX_THUMBS = 12; // thumbnails drawn at once

/** A template as the panel shows it. `name` is already in the language asked for. */
export type TemplateCard = {
  id: string;
  source: TemplateSource;
  name: string;
  items: number;
  page: PageTemplate["page"];
};

/** The list of one page kind in two groups: the shipped ones as the server sorted them, the student's own ones newest first. */
export type TemplateGroups = { shipped: TemplateCard[]; own: TemplateCard[] };

export function groupTemplates(items: PageTemplate[], locale: string): TemplateGroups {
  const vi = locale.startsWith("vi");
  const card = (x: PageTemplate): TemplateCard => ({
    id: x.id,
    source: x.source,
    name: x.source === "own" || !vi ? x.label_en : x.label_vi, // an own template has the student's name in both
    items: x.item_count,
    page: x.page,
  });
  const created = (x: PageTemplate) => x.created_at ?? 0;
  return {
    shipped: items.filter((x) => x.source === "shipped").map(card),
    own: items
      .filter((x) => x.source === "own")
      .sort((a, b) => created(b) - created(a)) // stable: the server's order stays for equal times
      .map(card),
  };
}

/** The cards in the order they are shown and walked with the arrow keys. */
export const allCards = (g: TemplateGroups): TemplateCard[] => [...g.shipped, ...g.own];

/** The lists already read, by page kind. Adding one keeps the newest `MAX_KINDS_CACHED` kinds only (a body is up to 40 items per template). */
export type Lists = Record<string, PageTemplate[]>;
export const remember = (lists: Lists, kind: string, items: PageTemplate[]): Lists =>
  Object.fromEntries([
    ...Object.entries(lists)
      .filter(([k]) => k !== kind)
      .slice(-(MAX_KINDS_CACHED - 1)),
    [kind, items],
  ]);

/** True when applying would replace something the student made: moved elements, added items or a background. */
export function hasWork(doc: LayoutDoc, kind: string): boolean {
  const p = doc.pages[kind];
  return !!p && (!!p.bg || Object.keys(p.overrides ?? {}).length > 0 || (p.items?.length ?? 0) > 0);
}

/** Why a template call failed, as the key under `editor.templates.err.*`; each has its own text in vi and en. */
export type TemplateError = HistoryError | "mismatch";

export function templateError(e: unknown): TemplateError {
  if (e instanceof ApiError && e.code === "template_mismatch") return "mismatch";
  return historyError(e);
}

export type ApplyDeps = {
  /** `store.settle`: true when the server has the newest document. */
  settle: () => Promise<boolean>;
  /** `store.currentVersion`: read after `settle`, because a save moves it. */
  version: () => number;
  apply: (source: TemplateSource, id: string, baseVersion: number) => Promise<LayoutRestored>;
  /** Takes the answer as the saved document and forgets the undo and redo steps. */
  adopt: (layout: LayoutDoc, version: number) => void;
  enterConflict: (stored: LayoutConflict | undefined) => void;
};

export type ApplyResult =
  { ok: true; dropped: number } | { ok: false; error: TemplateError; retryAfter?: number };

/**
 * Apply (spec 34 requirement 5): settle first so the server's version is the editor's, then ask with that version. Only an answer is adopted; every
 * failure leaves the store, its history and its autosave as they were (a conflict starts the conflict flow, which is the autosave's own).
 */
export async function applyTemplate(card: TemplateCard, d: ApplyDeps): Promise<ApplyResult> {
  if (!(await d.settle())) return { ok: false, error: "unsaved" };
  try {
    const r = await d.apply(card.source, card.id, d.version());
    d.adopt(r.layout, r.version);
    return { ok: true, dropped: r.dropped };
  } catch (e) {
    const error = templateError(e);
    if (error === "conflict") d.enterConflict((e as ApiError).data as LayoutConflict | undefined);
    return {
      ok: false,
      error,
      ...(e instanceof ApiError && e.retryAfter !== undefined ? { retryAfter: e.retryAfter } : {}),
    };
  }
}
