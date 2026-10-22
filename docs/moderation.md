# Notes moderation (T-013)

The owner's inbox reads and changes the notes friends sent through a collection link. Contract: `api/openapi.yaml` (tag `notes`, `/api/note/*`);
code: `internal/notes/moderation*.go`; Postman: `postman/notes-moderation.postman_collection.json`.

| Route | What |
|---|---|
| `GET /api/note/get-list` | one page (`status`, `limit` 50/100, `next_id`) plus `counts` of the whole book; approved in book order, the rest newest first |
| `POST /api/note/batch-update-status` | 1 to 100 notes to `pending`, `approved` or `hidden`, all or nothing; approving appends in the order given |
| `POST /api/note/reorder` | the approved set in a new order; any other set is `ERROR_ORDER_STALE` (409) |
| `POST /api/note/batch-delete` | 1 to 100 notes, their photo links, photo rows and objects; hard delete |
| `GET /api/note-collection/get-list` | the book's collection links (T-105): one page (`limit` 50/100, `next_id`), newest first, revoked ones included |

## Collection links (T-105)

- **Cap.** At most **50 links per book, revoked ones included** (`maxLinks`), on top of the older cap of 5 active (not revoked) links. `note-collection/create` answers `409 ERROR_LIMIT_REACHED` for the 51st (or the 6th active one). A
  revoked link cannot be deleted yet, so the 50 is permanent for the book; the check runs in the create transaction under the book row lock, so parallel creates cannot pass it (`TestLinkCap`, and `TestLimit` for the parallel creates against the 5-active cap).
- **Paging.** `note-collection/get-list` is keyset-paged like the note list: `limit` 1..100 (default 50), `next_id` is the sealed `(created_at, id)` of the last link, bound to the book (`httpx.Cursors`, scope `note-collection/get-list:<book id>`),
  `""` on the last page, a foreign or damaged token is `ERROR_PARAM`. Because the position is a key and not an offset, a link created between two requests never moves a page (`TestLinkListPaging`). With the cap at 50 the default page
  holds every link; the web list still follows the "load more" pattern (`NoteLinks.tsx`).
- **Photos sent through a link** do not count toward the owner's photo quota: see "Quotas" in [media.md](media.md).

## Rules

- **Statuses** are the Go constants `StatusPending`, `StatusApproved`, `StatusHidden` (the column is a plain `VARCHAR(16)`). An unknown value in the request is
  `ERROR_PARAM`; an unknown value read from the table is an internal error, never a silent default.
- **Position.** `note_tab.sort_order` is 1..n for approved notes and `NULL` for every other note, with no gap and no duplicate after every operation. Leaving the
  approved set (hide, pending, delete) renumbers the rest in the same transaction.
- **Lock order.** Every moderation transaction takes the owner's `yearbook_tab` row `FOR UPDATE` first, then the note rows in id order, then the media rows. The
  public submit takes the book `FOR SHARE` first and the yearbook delete `FOR UPDATE` first, so moderation, submit and delete queue and never deadlock
  (`TestModerationConcurrency`, `TestModerationRacesSubmitAndBookDelete`).
- **Delete.** The photo objects are removed first (`media.Service.DeleteObjects`); if the object store fails the answer is `ERROR_STORAGE_ERROR` (502), nothing in
  the database changed and the same call is retried. Then one transaction deletes the photo links, the `media_tab` rows (clearing a cover or profile photo that
  points at one, through the same `RefClearer` path as `media/delete`) and the notes. Repeating a delete that already succeeded is `ERROR_NOT_FOUND`.
- **Access.** Verified email required (`ERROR_EMAIL_NOT_VERIFIED`); another user's book or note is `ERROR_NOT_FOUND`, never 403. Writes are limited to 120 per user
  per minute (`ERROR_RATE_LIMITED`); reads are not counted. Ids (and `yearbook_id`) must be canonical upper-case ULIDs, else `ERROR_PARAM` (the id columns compare case-insensitively, so another spelling would match the row); ids in a batch must not repeat.
- **The 300-note cap** of a link still counts hidden and pending notes (`TestHiddenNotesCountTowardTheCap`).
- **Cursor.** `next_id` is sealed with the scope `note/get-list:<yearbook>:<status>`; a token of another list, yearbook or user is `ERROR_PARAM`.

## Performance

The approved list reads `idx_yearbook_id_status_sort_order` (keyset on `sort_order`); the other lists filter on the same index and sort by `created_at, id` in
memory (at most a few hundred notes per book, so no new index). Photos and link labels are read with one query each for the whole page.

## Book data (T-056)

`GET /api/book-data/get?yearbook_id=` (package `internal/bookdata`) hands the browser renderer the whole book in one owner-only response: the yearbook, the owner's
profile and the **approved** notes in book order (`sort_order`, then id: the order of `note/get-list?status=approved`), each with its answers and photo ids.

- **Access.** Same rules as the note list: session, verified email, canonical upper-case ULID (`ERROR_PARAM`), another user's or a missing book is `ERROR_NOT_FOUND`.
- **Left out.** Pending and hidden notes, storage keys, hashes, emails, the birthday, link labels and every internal id. Photos are public ids, read with
  `media/get-content?id=&variant=print`.
- **Queries.** Yearbook and profile (2, plus 1 for the cover/profile photo ids), the approved notes (1) and their photo links (1, skipped when there are no notes);
  the count does not depend on the number of notes (`TestQueryCountAndCap`).
- **Bound.** At most 300 notes, the reorder cap. The 300-note limit is per link and a book can have up to five active links, so a book with more approved
  notes is cut after the 300th in book order, never silently: `note_count` is the number returned, `approved_total` the real approved count and `truncated`
  (always present) is true when `approved_total` is more than `note_count` (`TestTruncatedFlag`: 299, 300, 301 notes). The renderer shows a warning then.
  The count is one extra query, run only when the 300 cap was reached. With the built-in templates' fields at their maximum length (name 60, relationship 60,
  message 2000 characters, ASCII) and 3 photos per note the answer is about 690 KB (`TestMaximalBookIsUnderOneMiB`). Text that needs 2 or more bytes per
  character, or a template that asks for all nine fields, can pass 1 MiB: the renderer must not assume it. The API does not compress the response (no gzip in the router), so that size goes over the wire.
