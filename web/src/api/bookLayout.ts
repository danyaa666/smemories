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
