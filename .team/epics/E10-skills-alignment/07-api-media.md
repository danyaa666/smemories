# E10_T-069 — API v2: media domain

**Epic:** E10-skills-alignment · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high (uploads) · **Priority:** P1 · **Type:** tech-debt
**Read first:** `docs/api-contract.md`, `docs/go-conventions.md`, T-068 (the pattern), `docs/media.md`.
**Depends on:** T-065, T-068.

#### Description
Move the media endpoints to `/api/media/*`: multipart `create`, `get-list`, binary `get-content`, `delete`. `media.Service` already exists; make the handler thin, adopt `apperr` and the envelope, keep every upload cap and the T-036/T-057 behaviour.

#### Requirements (SHALL)
1. Routes: `POST /api/media/create` (multipart; fields `yearbook_id`, `file`), `GET /api/media/get-list?yearbook_id=&limit=&next_id=`, `GET /api/media/get-content?id=&variant=display|thumb|print` (default `display`), `POST /api/media/delete` (`id`). `/v1/...` media routes removed.
2. `get-content` SHALL return the image bytes with the existing cache and security headers and **no envelope** on success; on failure it SHALL return the v2 error envelope with the right HTTP status (an `<img>` tag only needs the status).
3. `create` SHALL keep the existing body cap, concurrency cap, timeout, decode limits and the multipart origin guard; the per-route timeout from T-063 (`WithTimeout`) replaces any hand-made deadline.
4. List items carry `id`, `yearbook_id`, `content_type`, `bytes`, `width`, `height`, `created_at` (Unix ms), and the content URLs are built by the client from `id`; no storage keys in responses.

#### Acceptance criteria
- [ ] AC1 — Contract lint passes for the four operations; multipart and binary responses documented (content types, `variant` enum values explained).
- [ ] AC2 — All existing media integration tests pass with only path/envelope/code/time changes (listed in the PR); upload caps (size, pixels, concurrency, slow body) unchanged.
- [ ] AC3 — Web: upload (XHR progress), photo library, cover and profile photo pickers use the new paths; `<img src="/api/media/get-content?id=...&variant=thumb">`; no screen changes; vitest updated.
- [ ] AC4 — `media.Service` is free of `net/http`; storage and DB errors are `apperr` (`ERROR_STORAGE` 502 = `BadGateway` family, listed in `x-error-codes`).
- [ ] AC5 — Postman, openapi, `docs/media.md` paths updated.

#### Risk
`high`: upload path and file serving. Owner approves.

#### Test plan
- QA should probe: image request with and without session, another user's media id (404 not 403), `variant=print` before backfill, HEAD request, range request, a multipart body whose `yearbook_id` is missing, a 413 mid-stream, content-type sniffing headers unchanged, a 404 inside `<img>` leaves no JSON cached.
