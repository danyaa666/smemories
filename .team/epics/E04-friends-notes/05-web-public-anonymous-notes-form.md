# T-018 — Web: public anonymous notes form

**Epic:** E04-friends-notes · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Mobile-first public page opened from the shared link: name, relationship, message, photo picker; clear success and error states; EN and VI; works without an account or cookies; no personal data of the owner beyond the book title.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._


#### Leader notes from decision D-21 (2026-10-08)
The form is generated from the `fields` array returned by `GET /v1/public/collect/{token}` (T-034 AC11): one input per field (`short_text` is a single-line input, `long_text` a textarea) with the label and hint in the page language, required marks, a live character counter from `max_length`, and the submission carries `answers` as JSON in a multipart part next to the photos. Do not hard-code name, relationship or message. Show `unknown_field`, `missing_answer` and `invalid_answer` next to the field named in the error.

#### Leader notes from the T-034 rework (2026-10-08)
The API (T-034) now requires the multipart part `answers` to come **before** the `photos` parts, otherwise `400 invalid_body`: append `answers` to the `FormData` first. A photo-bearing submission can answer `503 busy` + `Retry-After` immediately, because only a few photo uploads are processed at once (2 per client IP, 4 in total by default): retry it automatically after `Retry-After` (2 s if absent) up to 3 times with a friendly "still sending" message, as the owner photo upload in `web/src/components/Photos.tsx` does, then show the error. A stalled connection is dropped by the server after 10 s without data (`408 request_timeout`): show a retry button and keep the typed answers and chosen photos. Show the upload progress.
