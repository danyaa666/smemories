# T-018 — Web: public anonymous notes form

**Epic:** E04-friends-notes · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Mobile-first public page opened from the shared link: name, relationship, message, photo picker; clear success and error states; EN and VI; works without an account or cookies; no personal data of the owner beyond the book title.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._


#### Leader notes from decision D-21 (2026-10-08)
The form is generated from the `fields` array returned by `GET /v1/public/collect/{token}` (T-034 AC11): one input per field (`short_text` is a single-line input, `long_text` a textarea) with the label and hint in the page language, required marks, a live character counter from `max_length`, and the submission carries `answers` as JSON in a multipart part next to the photos. Do not hard-code name, relationship or message. Show `unknown_field`, `missing_answer` and `invalid_answer` next to the field named in the error.
