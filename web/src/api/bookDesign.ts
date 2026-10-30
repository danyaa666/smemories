import type { components } from "./schema";
import { postJson, request } from "./client";

// The owner's design choices for a book (epic E09, decision D-41): api/openapi.yaml, `book-design/*`. Owner only.

type S = components["schemas"];
export type BookDesign = S["BookDesignResponse"]["data"];
export type DesignOptions = S["BookDesignOptions"];
/** The choices the book's template offers (the `design` block of its manifest). */
export type DesignBlock = S["BookDesignBlock"];

export const getBookDesign = (yearbookId: string) =>
  request<BookDesign>(`/book-design/get?yearbook_id=${encodeURIComponent(yearbookId)}`);

/** Replaces the whole options document (it is not merged with the earlier one). */
export const updateBookDesign = (yearbookId: string, options: DesignOptions) =>
  postJson<BookDesign>("/book-design/update", { yearbook_id: yearbookId, options });

/** Deletes the stored options: the book goes back to the template's own look. */
export const resetBookDesign = (yearbookId: string) =>
  postJson<void>("/book-design/reset", { yearbook_id: yearbookId });
