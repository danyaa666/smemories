import type { components } from "./schema";
import { request } from "./client";

// The whole book for the browser renderer (decision D-24): api/openapi.yaml, `GET /api/book-data/get`. Owner only.

export type BookData = components["schemas"]["BookDataResponse"]["data"];
export type BookNote = BookData["notes"][number];

/** Up to 300 notes: the answer can be several MB (dense Vietnamese, emoji), so it is read as one plain fetch with no size assumption. */
export const getBookData = (yearbookId: string) =>
  request<BookData>(`/book-data/get?yearbook_id=${encodeURIComponent(yearbookId)}`);
