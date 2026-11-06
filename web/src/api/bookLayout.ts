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

// Page templates (T-172): shipped, authored pages that hold only text and asset items. Reading them is owner only and shares the save rate limit.
export type PageTemplate = S["BookLayoutTemplateListResponse"]["data"]["items"][number];

/** The shipped page templates of the book's current template, optionally of one page kind (`cover`, `profile`, `notes`, `back`). */
export const listPageTemplates = (yearbookId: string, kind?: string) =>
  postJson<S["BookLayoutTemplateListResponse"]["data"]>("/book-layout/template-list", {
    yearbook_id: yearbookId,
    ...(kind === undefined ? {} : { kind }),
  });

// Own templates and apply (T-173, screen T-175). `apply-template` is the only one of them that writes the layout; all share the save rate limit.
export type TemplateSource = PageTemplate["source"];

/** What `template-create` answers: the new template without its page; `stripped` counts the image items and the picture background left out. */
export type OwnTemplateCreated = S["BookLayoutTemplateCreateResponse"]["data"];

/** Copies the book's STORED `pages.<kind>` into a new own template (T-173: 409 `ERROR_TEMPLATE_LIMIT` with `data.limit` at 20 per user, all kinds). */
export const createOwnTemplate = (yearbookId: string, kind: string, name: string) =>
  postJson<OwnTemplateCreated>("/book-layout/template-create", {
    yearbook_id: yearbookId,
    kind,
    name,
  });

/** Deletes one of the caller's own templates (404 for an unknown or foreign id); books that used it do not change. */
export const deleteOwnTemplate = (id: string) =>
  postJson<void>("/book-layout/template-delete", { id });

/** Replaces `pages.<kind>` with the template's page in one write; the answer is the whole new document, like `restore`. */
export const applyPageTemplate = (
  yearbookId: string,
  baseVersion: number,
  kind: string,
  source: TemplateSource,
  templateId: string,
) =>
  postJson<LayoutRestored>("/book-layout/apply-template", {
    yearbook_id: yearbookId,
    base_version: baseVersion,
    kind,
    source,
    template_id: templateId,
  });
