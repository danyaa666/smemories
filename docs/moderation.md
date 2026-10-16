# Notes moderation (T-013)

The owner's inbox reads and changes the notes friends sent through a collection link. Contract: `api/openapi.yaml` (tag `notes`, `/api/note/*`);
code: `internal/notes/moderation*.go`; Postman: `postman/notes-moderation.postman_collection.json`.

| Route | What |
|---|---|
| `GET /api/note/get-list` | one page (`status`, `limit` 50/100, `next_id`) plus `counts` of the whole book; approved in book order, the rest newest first |
| `POST /api/note/batch-update-status` | 1 to 100 notes to `pending`, `approved` or `hidden`, all or nothing; approving appends in the order given |
| `POST /api/note/reorder` | the approved set in a new order; any other set is `ERROR_ORDER_STALE` (409) |
| `POST /api/note/batch-delete` | 1 to 100 notes, their photo links, photo rows and objects; hard delete |

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
