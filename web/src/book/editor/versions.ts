// The model of the version history panel (T-165): relative time, the "N items will be dropped" hint, the small preview cache and the restore flow.
// No React here, so the rules are tested with a fake API (versions.test.ts). The server routes are in api/bookLayout.ts (T-163).
import { ApiError } from "../../api/client";
import type { LayoutConflict, LayoutDoc, LayoutRestored } from "../../api/bookLayout";

export const MAX_ROWS = 30; // the server's cap, kept here only for the tests
export const MAX_CACHED = 5; // bodies of up to 128 KiB each

/** "12 minutes ago" in the app language. Under a minute is "now"; hours up to 48, then days (calendar precision is not needed). */
export function relativeTime(ms: number, now: number, locale: string): string {
  const f = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  const s = Math.max(0, Math.round((now - ms) / 1000));
  if (s < 60) return f.format(0, "second");
  if (s < 3600) return f.format(-Math.floor(s / 60), "minute");
  if (s < 48 * 3600) return f.format(-Math.floor(s / 3600), "hour");
  return f.format(-Math.floor(s / 86400), "day");
}

/** The absolute time for a `title` attribute. */
export const absoluteTime = (ms: number, locale: string) =>
  new Date(ms).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" });

/**
 * How many items and page backgrounds point at an asset or design image that is gone (a hint; the server's `dropped` is the truth).
 * `hasMedia` is null while the list of design images is not complete (loading, failed or paged): media are then not counted.
 */
export function missingCount(
  doc: LayoutDoc,
  hasAsset: (id: string) => boolean,
  hasMedia: ((id: string) => boolean) | null,
): number {
  let n = 0;
  const gone = (asset?: string, media?: string) =>
    (asset !== undefined && !hasAsset(asset)) ||
    (media !== undefined && hasMedia !== null && !hasMedia(media));
  for (const p of Object.values(doc.pages)) {
    if (p.bg && gone(p.bg.asset, p.bg.media)) n++;
    for (const i of p.items ?? []) if (gone(i.asset, i.media)) n++;
  }
  return n;
}

/** The newest `max` entries by use; a snapshot never changes, so an entry never goes stale. */
export class PreviewCache<T> {
  private m = new Map<string, T>();
  constructor(private max = MAX_CACHED) {}
  get(id: string): T | undefined {
    const v = this.m.get(id);
    if (v !== undefined) {
      this.m.delete(id);
      this.m.set(id, v);
    }
    return v;
  }
  set(id: string, v: T) {
    this.m.delete(id);
    this.m.set(id, v);
    while (this.m.size > this.max) this.m.delete(this.m.keys().next().value!);
  }
  get size() {
    return this.m.size;
  }
}

/** Why a call of the history failed, as the key under `editor.history.err.*` (each has its own text in vi and en). */
export type HistoryError =
  "conflict" | "invalid" | "template" | "gone" | "busy" | "offline" | "unsaved" | "other";

export function historyError(e: unknown): HistoryError {
  if (!(e instanceof ApiError)) return "other";
  if (e.status === 0) return "offline";
  if (e.status === 429) return "busy";
  if (e.status === 404) return "gone";
  if (e.code === "version_conflict") return "conflict";
  if (e.code === "version_template_changed") return "template";
  if (e.code === "invalid_layout") return "invalid";
  return "other";
}

export type RestoreDeps = {
  /** `store.settle`: true when the server has the newest document. */
  settle: () => Promise<boolean>;
  /** `store.currentVersion`: read after `settle`, because a save moves it. */
  version: () => number;
  restore: (id: string, baseVersion: number) => Promise<LayoutRestored>;
  adopt: (layout: LayoutDoc, version: number) => void;
  enterConflict: (stored: LayoutConflict | undefined) => void;
};

export type RestoreResult =
  { ok: true; dropped: number } | { ok: false; error: HistoryError; retryAfter?: number };

/**
 * Restore (spec 24 requirement 4): settle first so the server's version is the editor's, then ask with that version. Only an answer is adopted;
 * every failure leaves the store, its history and its autosave as they were (a conflict starts the conflict flow, which is the autosave's own).
 */
export async function restoreVersion(id: string, d: RestoreDeps): Promise<RestoreResult> {
  if (!(await d.settle())) return { ok: false, error: "unsaved" };
  try {
    const r = await d.restore(id, d.version());
    d.adopt(r.layout, r.version);
    return { ok: true, dropped: r.dropped };
  } catch (e) {
    const error = historyError(e);
    if (error === "conflict") d.enterConflict((e as ApiError).data as LayoutConflict | undefined);
    return {
      ok: false,
      error,
      ...(e instanceof ApiError && e.retryAfter !== undefined ? { retryAfter: e.retryAfter } : {}),
    };
  }
}
