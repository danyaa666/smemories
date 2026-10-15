# E04_T-013 — Notes moderation API (approve, hide, reorder, delete)

> **From T-066 QA (2026-10-12):** the note `status` column is now a plain VARCHAR(16) (`pending`, `approved`, `hidden`) with no database check. Validate it in Go against named constants in the store/service (reject unknown values with a 400 before any write; unknown values read from the table are an internal error, not a silent default), and add a test. Moderation transactions that touch notes and photos must take the `yearbook_tab` row lock first (docs/db-conventions.md, Writes).

> **E10 contract note (2026-10-09, D-25..D-27):** this task is built on the v2 API contract and database conventions, not the v1 ones written below. Read `docs/api-contract.md`, `docs/db-conventions.md` and `docs/go-conventions.md` first. Wherever this spec names a `/v1/...` path, an old error code, an RFC 3339 time, a `cursor`, a foreign key or a `DATETIME` column, use the v2 equivalent (route map: `docs/api-contract.md` section 8). New tables are `_tab` tables with BIGINT ms timestamps and no foreign keys. Epic: `.team/epics/E10-skills-alignment/PRD.md`.

**Epic:** E04-friends-notes · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Owner lists pending/approved/hidden notes per yearbook, approves or hides them, reorders approved notes, deletes a note (and its photos). Only approved notes are exported. Cross-user access returns 404.



#### Leader notes from decision D-21 (2026-10-08)
A note's text is `answers` (JSON object, field id to text), not fixed columns (T-034). The owner list returns `answers` as stored plus the photo ids; the display name of a note is `answers.name` (fallback "Anonymous"). Search or sorting by a field is out of scope.

---

## Full specification (leader, 2026-10-15; written from the inbox design `docs/design/notes/Notes Inbox.dc.html`)

#### Endpoints (v2, namespace `note`; all `x-auth: required`, verified email, CSRF origin guard on POST; another user's yearbook is `ERROR_NOT_FOUND` 404)
| Route | Purpose |
|---|---|
| `GET /api/note/get-list?yearbook_id=&status=&limit=&next_id=` | `status` optional (`pending`, `approved`, `hidden`; omitted = all). `limit` default 50, max 100; `next_id` from `httpx.Cursors` (scope `note/get-list:<yearbook>:<status>`). Order: `status=approved` is **book order** (`sort_order`); every other value is newest first. Items: `id`, `status`, `answers` (object), `photos` (`[{id}]`, content via `media/get-content`), `link_label` (label of the collection it came through, may be empty), `created_at` (Unix ms), `position` (1-based, only for approved, else `null`). Response `data` also has `counts: {all, pending, approved, hidden}` for the whole yearbook (the tabs and the header "3 waiting for you, 4 in your book"). |
| `POST /api/note/batch-update-status` | Body `{yearbook_id, ids: [..], status}`; 1 to 100 ids, `status` one of the named constants. One transaction, all or nothing: any id that is not in that yearbook is `ERROR_NOT_FOUND` for the whole batch. Approving appends to the end of the approved order (a note already approved keeps its place); hiding or setting pending clears its position and closes the gap. One note = a batch of one. Returns `counts`. |
| `POST /api/note/reorder` | Body `{yearbook_id, ids: [..]}`: the approved notes in the wanted order. `ids` must be exactly the current approved set; otherwise `ERROR_ORDER_STALE` (409) and the client reloads (a note was approved or hidden meanwhile). Max 300 ids. |
| `POST /api/note/batch-delete` | Body `{yearbook_id, ids: [..]}`, 1 to 100. Deletes the notes and their photos (the note-photo rows, the `media_tab` rows and the objects, using the existing media purge path; storage errors are retried like the yearbook delete, never a half state in the database). Hard delete, no undo on the server (the web delays the call, see T-017). Returns `counts`. |

#### Requirements (SHALL)
1. Status values are named constants in Go (`pending`, `approved`, `hidden`), validated in the service; unknown is `ERROR_PARAM`. Business error codes: `ERROR_NOT_FOUND`, `ERROR_ORDER_STALE`, `ERROR_TOO_MANY_IDS` (400), documented per route in `x-error-codes`.
2. Layering and conventions as in `docs/go-conventions.md` and `docs/db-conventions.md`: handler -> `notes.Service` (moderation methods) -> store; `apperr`; no `net/http` in the service; **lock order**: lock the `yearbook_tab` row first in every transaction here, then the note rows (concurrency test in AC5).
3. `sort_order` of approved notes is dense (1..n) after every operation; a unique-looking gap or duplicate is a bug. The existing index `idx_yearbook_id_status_sort_order` serves the list; add none speculatively.
4. The owner list returns `answers` as stored; the display name is `answers.name` (fallback is the client's concern). Photo ids only, never object keys.
5. Cross-user access is 404, never 403. Rate limit: reuse the existing Redis limiter, 120 moderation writes per user per minute (`ERROR_RATE_LIMITED`).
6. `api/openapi.yaml` (summary, description, `x-auth`, `x-error-codes`, examples, per-field descriptions), `web/src/api/schema.d.ts` regenerated, Postman collection `postman/notes-moderation.postman_collection.json`, docs (`docs/note-fields.md` or a new `docs/moderation.md` section).

#### Acceptance criteria
- [ ] AC1 — List: filters and `counts` correct with notes in all three statuses; approved list is in `sort_order`; paging with `next_id` is stable and a tampered or foreign cursor is `ERROR_PARAM`.
- [ ] AC2 — Batch status: approve 3 pending notes (they get positions n+1..n+3 in the order given), hide one approved note (positions close up), a batch with one foreign id changes nothing.
- [ ] AC3 — Reorder: valid permutation applies; a stale set is 409 and changes nothing; positions stay dense.
- [ ] AC4 — Delete: notes, note-photo rows, media rows and objects are gone (zero rows asserted); a storage failure leaves the notes recoverable and is retried; another user's ids are 404.
- [ ] AC5 — Concurrency (race detector): two reorders at once, approve while reorder, delete while approve: no deadlock (1213 never a 500), positions dense, exactly the expected winners.
- [ ] AC6 — Contract lint, `check:api`, Newman, `make lint build test test-integration` green; no `.team/` files in the PR.

#### Test plan
- QA should probe: 100 vs 101 ids, duplicate ids in one batch, an approved note re-approved (no move), reorder with a hidden id, deleting the last approved note, a note with three photos, the T-034 public submit still counting hidden and pending notes toward the 300 cap.

