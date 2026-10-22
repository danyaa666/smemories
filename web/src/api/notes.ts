import type { components } from "./schema";
import { ApiError, postJson, request } from "./client";

// The owner's side of the notes: link management (T-067) and the moderation inbox (T-013), api/openapi.yaml tag `notes`.
// Ids are canonical upper-case ULIDs; the server compares them as sent.

type S = components["schemas"];
export type NoteItem = S["NoteItem"];
export type NoteStatus = NoteItem["status"];
export type NoteCounts = S["NoteCounts"];
export type NotePage = S["NoteListResponse"]["data"];
export type NoteCollection = S["NoteCollection"];
export type NoteCollectionCreated = S["NoteCollectionCreated"];

export const PAGE_SIZE = 100;
/** The server refuses more than this many ids in one batch. */
export const BATCH_MAX = 100;

const q = (o: Record<string, string | number | undefined>) =>
  Object.entries(o)
    .filter(([, v]) => v !== undefined && v !== "")
    .map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`)
    .join("&");

/** One page of notes; `status` omitted means all. Approved is in book order, the rest newest first. */
export const listNotes = (
  yearbookId: string,
  status: NoteStatus | undefined,
  nextId?: string,
  limit = PAGE_SIZE,
) =>
  request<NotePage>(
    `/note/get-list?${q({ yearbook_id: yearbookId, status, limit, next_id: nextId })}`,
  );

export const updateNoteStatus = (yearbookId: string, ids: string[], status: NoteStatus) =>
  postJson<{ counts: NoteCounts }>("/note/batch-update-status", {
    yearbook_id: yearbookId,
    ids,
    status,
  });

export const reorderNotes = (yearbookId: string, ids: string[]) =>
  postJson<void>("/note/reorder", { yearbook_id: yearbookId, ids });

/** Deletes in batches of at most 100; returns the counts after the last batch. */
export async function deleteNotes(yearbookId: string, ids: string[]): Promise<NoteCounts> {
  let counts: NoteCounts | undefined;
  for (let i = 0; i < ids.length; i += BATCH_MAX) {
    const r = await postJson<{ counts: NoteCounts }>("/note/batch-delete", {
      yearbook_id: yearbookId,
      ids: ids.slice(i, i + BATCH_MAX),
    });
    counts = r.counts;
  }
  if (!counts) throw new ApiError(0, "invalid_body", "No ids");
  return counts;
}

/** One page of a book's links, newest first (the server's default page size; a book has at most 50 links). */
export const listLinks = (yearbookId: string, nextId?: string) =>
  request<S["NoteCollectionListResponse"]["data"]>(
    `/note-collection/get-list?${q({ yearbook_id: yearbookId, next_id: nextId })}`,
  );

export const createLink = (yearbookId: string, label: string, deadlineAt: number | null) =>
  postJson<NoteCollectionCreated>("/note-collection/create", {
    yearbook_id: yearbookId,
    ...(label ? { label } : {}),
    deadline_at: deadlineAt,
  });

export const revokeLink = (id: string) => postJson<void>("/note-collection/revoke", { id });

/**
 * The link a friend opens: `<base>/c/<token>`. The base is VITE_PUBLIC_BASE_URL (the web twin of SMEM_PUBLIC_BASE_URL),
 * else the origin the owner is on. The token is a bearer secret: show it once, never store it.
 */
export function shareUrl(token: string): string {
  const base =
    (import.meta.env.VITE_PUBLIC_BASE_URL as string | undefined) || window.location.origin;
  return `${base.replace(/\/+$/, "")}/c/${token}`;
}

/** A 4xx is the answer, not a glitch: do not retry it. A network error or 5xx gets one more try. */
export const retryOnce = (n: number, e: unknown) =>
  !(e instanceof ApiError && e.status >= 400 && e.status < 500) && n < 1;
