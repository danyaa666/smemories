import type { components } from "./schema";
import { postJson, request } from "./client";

// The plain-text export and the switch for its public photo links (decision D-32): api/openapi.yaml,
// `GET /api/book-export/get-text` and `POST /api/yearbook/revoke-photo-links`. Owner only.

export type BookText = components["schemas"]["BookExportResponse"]["data"];

/** `lang` is left out on purpose: the file follows the book's language. The answer holds bearer links and is never cached. */
export const getBookText = (yearbookId: string) =>
  request<BookText>(`/book-export/get-text?yearbook_id=${encodeURIComponent(yearbookId)}`, {
    cache: "no-store",
  });

export const revokePhotoLinks = (yearbookId: string) =>
  postJson<void>("/yearbook/revoke-photo-links", { id: yearbookId });
