# E11_T-095 — Signature storage, validation and API

**Epic:** E11-signatures · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1b · **Risk:** high · **Priority:** P2 · **Type:** feature
**Read first:** the PRD (design decisions L-14), `docs/api-contract.md`, `docs/db-conventions.md`, `docs/go-conventions.md`, `docs/note-fields.md`, `docs/moderation.md`, T-056 spec (`E05-templates-export/07-book-data-endpoint.md`).
**Not before** the go-live phase is complete (D-31); the board keeps it BACKLOG until the leader moves it.

#### Description
Notes can carry one optional handwritten signature, stored as validated vector strokes (L-14), returned to the owner by the inbox and the book-data endpoints, accepted by the public submit endpoint.

#### Requirements (SHALL)
1. **Format v1.** Canonical JSON `{"v":1,"w":1000,"h":400,"s":[[x0,y0,x1,y1,...],...]}`: integers only, `0 <= x <= w`, `0 <= y <= h`, each stroke at least 2 points (4 numbers, even count), at most 12 strokes and 600 points in total, serialised size at most 3072 bytes, and the bounding box at least 5 % of `w` wide or 5 % of `h` high (no empty speck). Anything else (extra keys, other `w`/`h`, floats, negative numbers) is `ERROR_INVALID_SIGNATURE` (400); the message names the rule, never the content. The server parses into a struct and **re-serialises**; the client string is never stored.
2. **Storage.** One expand-only migration adds a nullable `signature_json VARCHAR(3072)` to the notes table (name and conventions per `docs/db-conventions.md`, `_tab`, no foreign keys). Null means no signature. Nothing else is stored (no timing, pressure, device or pointer data).
3. **Template switch.** The template manifest gains an optional `"signature": true` (validator in `internal/templates`, default false, documented in `docs/templates.md`). `GET /api/public-collect/get` adds `data.signature: {enabled, w, h, max_strokes, max_points}` (`enabled` false when the book's template does not declare it). Submitting a signature to a book whose template does not enable it is `ERROR_PARAM`.
4. **Submit.** `POST /api/public-collect/submit-note` accepts an optional multipart field `signature` (the JSON above, at most 4 KiB read; larger is `ERROR_INVALID_SIGNATURE`). Order of checks, limits, honeypot behaviour, the verifier seam and "errors only after the body was read" stay exactly as they are; the signature is validated before any photo is processed.
5. **Reads.** `note/get-list` items and `book-data/get` notes carry `signature` (the object, or `null`). Hidden and pending notes keep following their existing rules (book-data: approved only). **Size bound:** with 300 notes of maximal answers, three photo ids and a maximal signature, measure the book-data response, set the documented bound to that measured size rounded up (at most 2 MiB; lower the 3072-byte cap if it cannot be met) and update the T-056 test and `docs/api-contract.md`.
6. **Lifecycle.** Delete, hide, approve, reorder, yearbook delete and (when it exists) the data export and account deletion treat the signature as part of the note row; add a test per path.
7. **No logging.** Signature content never appears in logs, errors or metrics.

#### Acceptance criteria
- [ ] AC1 — Validator table tests: valid minimal and maximal, odd coordinate count, one-point stroke, 13 strokes, 601 points, 3073 bytes, out-of-range and negative numbers, floats, extra key, wrong `v`/`w`/`h`, empty strokes list, a speck, deeply nested or huge input (bounded read).
- [ ] AC2 — Integration: submit with and without signature; a template without the switch refuses it; the signature round-trips unchanged through inbox and book-data; stored string is the canonical re-serialisation (a client with odd spacing and key order stores the canonical form); hide/delete/yearbook-delete remove it; pending notes never leak into book-data.
- [ ] AC3 — Book-data bound test at the new measured size; query count unchanged (no N+1).
- [ ] AC4 — `api/openapi.yaml`, `web/src/api/schema.d.ts`, Postman (valid, invalid, disabled template; runs twice), `docs/note-fields.md` (section "Signature"), `docs/moderation.md`, `docs/templates.md` updated; migration applies on a database with notes; `make lint test test-integration` green.

#### Risk
`high`: public anonymous input stored and later rendered; the owner approves the merge. The only rendering path is numbers to an SVG path (T-096, T-097); a test asserts the API never returns anything but integers.

#### Test plan
QA probes: fuzzed JSON, 1 MB body, unicode and control characters in the field, duplicate `signature` fields, signature without note text, many submissions per IP (limits unchanged), a hidden note's signature in every read path.
