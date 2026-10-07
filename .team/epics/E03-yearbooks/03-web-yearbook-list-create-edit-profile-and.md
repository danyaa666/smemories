# T-016 — Web: yearbook list, create/edit, profile and photo upload UI

**Epic:** E03-yearbooks · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Dashboard of the user's yearbooks; create/edit form for book information and the owner profile; photo upload with progress, thumbnails and delete; set cover and profile photo. Slot/form based, no free-form canvas (D-06).

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._

#### Leader notes from the T-009 review (2026-10-08)
`PUT /v1/yearbooks/{id}/profile` replaces every field: a body without `photo_media_id` **clears** the profile photo. The edit form must send the current `photo_media_id` on every save
(the same holds for `cover_media_id`: `PATCH` keeps it when the key is absent, `null` clears it). Add a test for "edit the name, the photo stays".
Uploads: show progress and the error codes `413 payload_too_large`, `415 unsupported_media_type`, `400 invalid_image`, `409 quota_exceeded`, `429 rate_limited` (use `Retry-After`) and `503 busy` (retry after 2 s) in the user's language.
