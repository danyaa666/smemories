# E09_T-164 — Palette follow-ups: copy, history edge, distinct cap/quota code (web + one small API change)

**Epic:** E09-customisation-editor · **Milestone:** M4 · **Risk:** low · **Priority:** P3 · **Type:** fix
**Depends on:** none (T-162 is merged). **Read first:** QA's and dev's comments on T-162 (`board.py get T-162`), `docs/editor-palette.md`, `web/src/book/editor/` (palette, delete-in-use code), `internal/media` upload errors, `docs/api-contract.md`.

#### Why
QA_PASS on T-162 left four small findings that are not worth blocking a merge but should not rot.

#### Requirements (SHALL)
1. **Dialog copy**: the delete-in-use dialog says "used on 1 page types"; use a plural form that works in Vietnamese and English ("used on 1 page" / "used on 2 pages"; i18n plural keys, no hand-built plurals). Same check for every other count string added in T-159..T-162 (`grep` the locale files for "{{count}}" strings without plural forms).
2. **Image size message**: the upload error "at most 12000 px a side" is inexact for PNGs (the server caps at 25 megapixels); make the message say what the server enforces for each type (JPEG/WebP by side length, PNG by megapixels) using the numbers the server really uses (read them from the media docs / limits; do not duplicate magic numbers: put them in one constant with a comment pointing at the Go limit).
3. **Undo after deleting an unused image**: an image counts as "in use" if the current document **or any entry of the undo/redo stacks** references it. If only history references it, the dialog says so and, on confirm, the history is cleared (same sequence as for an image used on a page, minus the page removal). Test: add an image, remove the item, delete the image, press undo: the item must not come back (or undo is unavailable).
4. **Distinct error for the 30-image cap**: the media create route answers `ERROR_QUOTA_EXCEEDED` for both the 30-design-image cap and the 500 MiB quota, and the client guesses from its list. Add a separate business code for the per-yearbook cap (`ERROR_DESIGN_IMAGE_LIMIT`, 409, `data: {limit: 30}`) following `docs/api-contract.md` conventions; the quota keeps `ERROR_QUOTA_EXCEEDED`. Update OpenAPI + generated types, docs, Go unit and integration tests, the Postman collection and the web mapping (the web message then no longer depends on the list it holds). Photos keep their existing code for their own cap if they have one.
5. No new dependency; nothing under `.team/`.

#### Acceptance criteria
- [ ] AC1 — Count strings read correctly for 1 and 2 in both languages (a vitest that renders them).
- [ ] AC2 — The PNG and JPEG messages quote the real limits; a test fails if the web constant and the Go limit drift apart (parse the Go constant or a shared JSON if the repo has one; otherwise document the pair and add an integration check that uploads an image just over each limit).
- [ ] AC3 — The undo scenario of requirement 3 is covered by a unit test and the e2e palette spec.
- [ ] AC4 — The 31st design image answers `ERROR_DESIGN_IMAGE_LIMIT` (integration test), the quota still answers `ERROR_QUOTA_EXCEEDED`, and the web shows each message from the code alone.
- [ ] AC5 — Go: `make lint test` and `make test-integration`, `check:api`, newman green; web lint, prettier, tsc, vitest, `lint:i18n`, build, e2e suite and `make print-check` green. QA writes the integration lines (API change).

#### Test plan
Dev: tests as above. QA: uploads the 31st design image and an over-quota file (set a small quota in the test environment if possible), checks both messages, repeats the undo scenario in the UI, and reads the count strings in vi and en.
