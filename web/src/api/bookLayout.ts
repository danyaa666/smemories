import type { components } from "./schema";
import { postJson, request } from "./client";

// The owner's free layout of a book (epic E09 stage 2): api/openapi.yaml, `book-layout/*`. Owner only.

type S = components["schemas"];
export type LayoutDoc = S["BookLayoutDocument"];
export type LayoutItem = S["BookLayoutItem"];
export type BookLayout = S["BookLayoutResponse"]["data"];
export type LayoutProblem = { path: string; code: string };
/** `data` of ERROR_VERSION_CONFLICT: the stored version and document. */
export type LayoutConflict = S["BookLayoutConflictError"]["data"];

/** The only source of `base_version` (book-data/get reports 0 for a null layout, not the real counter). */
export const getBookLayout = (yearbookId: string) =>
  request<BookLayout>(`/book-layout/get?yearbook_id=${encodeURIComponent(yearbookId)}`);

/** Full replace; succeeds only if `baseVersion` is the stored version (else 409 with the stored document in `ApiError.data`). */
export const saveBookLayout = (yearbookId: string, baseVersion: number, layout: LayoutDoc) =>
  postJson<{ version: number; updated_at: number }>("/book-layout/save", {
    yearbook_id: yearbookId,
    base_version: baseVersion,
    layout,
  });

/** Deletes the stored layout; the version moves on, so read it again with `getBookLayout`. */
export const resetBookLayout = (yearbookId: string) =>
  postJson<void>("/book-layout/reset", { yearbook_id: yearbookId });

// Version history (T-163, T-165): the three routes share the save rate limit, so the editor calls them only when the user asks.
export type LayoutVersionRow = S["BookLayoutVersionListResponse"]["data"]["items"][number];
export type LayoutVersion = S["BookLayoutVersionGetResponse"]["data"];
export type LayoutRestored = S["BookLayoutRestoredResponse"]["data"];

export const listLayoutVersions = (yearbookId: string) =>
  postJson<S["BookLayoutVersionListResponse"]["data"]>("/book-layout/version-list", {
    yearbook_id: yearbookId,
  });

export const getLayoutVersion = (yearbookId: string, id: string) =>
  postJson<LayoutVersion>("/book-layout/version-get", { yearbook_id: yearbookId, id });

/** Writes snapshot `id` as a new version; `baseVersion` is the version the editor holds (409 with the stored document otherwise). */
export const restoreLayoutVersion = (yearbookId: string, id: string, baseVersion: number) =>
  postJson<LayoutRestored>("/book-layout/restore", {
    yearbook_id: yearbookId,
    id,
    base_version: baseVersion,
  });
