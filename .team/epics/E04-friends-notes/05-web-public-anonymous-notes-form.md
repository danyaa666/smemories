# E04_T-018 — Web: public anonymous notes form

> **From T-034 round 2 (2026-10-09):** above the connection caps the server answers `503 busy` **without draining the request body**, so a browser can see a network error instead of the 503. The form must treat a network error on submit exactly like a 503 (keep the answers and photos, show the retry message, retry after a random 2-6 s). The `website` honeypot part wins wherever it appears in the multipart body; `answers` after the photos is `400 invalid_body`.

> **E10 contract note (2026-10-09, D-25..D-27):** build on the v2 API contract and database conventions (`docs/api-contract.md`, `docs/db-conventions.md`, `docs/go-conventions.md`); where this spec names `/v1/...` paths, old error codes, RFC 3339 times or `cursor`, use the v2 equivalents (route map: `docs/api-contract.md` section 8). Epic: `.team/epics/E10-skills-alignment/PRD.md`.

**Epic:** E04-friends-notes · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Mobile-first public page opened from the shared link: name, relationship, message, photo picker; clear success and error states; EN and VI; works without an account or cookies; no personal data of the owner beyond the book title.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._


#### Leader notes from decision D-21 (2026-10-08)
The form is generated from the `fields` array returned by `GET /v1/public/collect/{token}` (T-034 AC11): one input per field (`short_text` is a single-line input, `long_text` a textarea) with the label and hint in the page language, required marks, a live character counter from `max_length`, and the submission carries `answers` as JSON in a multipart part next to the photos. Do not hard-code name, relationship or message. Show `unknown_field`, `missing_answer` and `invalid_answer` next to the field named in the error.

#### Leader notes from the T-034 rework (2026-10-08)
The API (T-034) now requires the multipart part `answers` to come **before** the `photos` parts, otherwise `400 invalid_body`: append `answers` to the `FormData` first. A photo-bearing submission can answer `503 busy` + `Retry-After` immediately, because only a few photo uploads are processed at once (2 per client IP, 4 in total by default): retry it automatically after `Retry-After` (2 s if absent) up to 3 times with a friendly "still sending" message, as the owner photo upload in `web/src/components/Photos.tsx` does, then show the error. A stalled connection is dropped by the server after 10 s without data (`408 request_timeout`): show a retry button and keep the typed answers and chosen photos. Show the upload progress.


#### Leader note (2026-10-12, from T-068 review)
The public form SHALL show the localised "this link is full" message when the GET lookup (`/api/public-collect/get`) says the link is full or closed, **before** the friend fills the form, and SHALL treat a network error on submit as a generic "try again" message. T-080 makes the full-link submit answer readable by the browser; do not rely on it alone.

#### Design source and known differences (leader, 2026-10-15)
Visual source: `docs/design/notes/Public Note Form.dc.html` (screens N1 to N11), tokens from T-083 (`docs/design/notes/tokens.css`). Build with the repo's React and CSS variables and the T-083 base components; do not start before T-083 is merged. Where the design and this spec differ, **the spec wins**; record any further difference in the PR. Known differences to resolve exactly like this:
1. **Fields come from the API**, not from the design: render the form from `GET /api/public-collect/get` `data.fields` (catalogue ids, EN/VI labels, hints, limits; `docs/note-fields.md`). The design shows the default set (name, relationship, message); its message limit "1000" and long label "Nickname or how you know Linh" are examples: use the catalogue limit (message 2000) and label/hint.
2. **No number in the "full" screen** (design says 50 notes; the real cap is 300 and internal): "This link has reached its limit. Ask <name> to open a new link."
3. **Revoked and unknown links are the same 404** (by design, no oracle): N9 is the screen for any 404; it never shows the owner's name or photo. Expired (410 `ERROR_COLLECTION_CLOSED`) is N7, full (409 `ERROR_COLLECTION_FULL`) is N8.
4. **Photos.** The server takes JPEG, PNG or WebP up to 10 MiB each, at most 3, in one multipart request, all or nothing, and the error does not say which photo failed. So: the copy says "JPG, PNG or WebP, up to 10 MB" (not HEIC); validate type, size and count in the browser before sending; progress is one bar for the whole request (XHR/fetch upload progress), not per photo; on a server rejection show the N10 panel without naming a photo, with Try again, Remove photos, Send without photos. If the API later names the photo, highlight it.
5. **"Saved on this phone" (N4):** keep the text answers in `localStorage` while the form is open (never photos), clear on success, wrap every access in try/catch; if storage is unavailable drop the sentence.
6. Slow connection (N4): no auto-retry that could double-post; the single Send is disabled while sending; after a network error the user presses Send again (the server rate-limits per link and per IP).
7. The desktop live preview (N11) is `aria-hidden` decoration and may ship last; the success screen (N5) and "Make your own yearbook" link (to the register page) are required.
8. Language: EN/VI switcher as in the design; the Vietnamese strings from N6 are a draft for the owner's review.
