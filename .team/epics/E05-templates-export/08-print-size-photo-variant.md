# E05_T-057 — Media print-size variant (1800 px) for fast, light printing

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P1 · **Type:** feature

#### Description
Printing a 24-page book with 30 full 3000 px photos produced a 63 MB PDF and took 10.6 s in the spike (ADR 0003). The media pipeline adds a third stored size, `print` (long edge 1800 px, JPEG quality 85, PNG with transparency stays PNG), used by the print renderer. Decision D-24.

#### Scope
- In: what the acceptance criteria below say.
- Out (do not do): extending the Go renderer (T-010, T-038 stay as fallback), the free-layout editor (E09), new page kinds (D-17).

#### Acceptance criteria
- [ ] AC1 — Upload (owner and contributor, T-009 and T-034) stores a `print` object next to `display` and `thumb` (`yearbooks/<yearbook>/<media>-print.<ext>`); `GET /v1/media/{id}/content?size=print` serves it with the same headers and authorisation as the other sizes; the DB records its key (migration, next free number) and deleting a photo or yearbook removes it too.
- [ ] AC2 — A backfill command (`cmd/smemories-media-backfill`, idempotent, batches, `--dry-run`) creates `print` for existing media from `display`; a test runs it twice.
- [ ] AC3 — Memory stays within the T-036 budget (the extra resize reuses the banded scaler); the upload timing grows by less than 25%.
- [ ] AC4 — OpenAPI, schema, Postman (`size=print`), docs/media.md updated; existing media tests pass unchanged.

#### Design
See ADR `docs/adr/0003-html-print-export.md` and the prototype `web/src/spike/print/` for the measured approach; follow the conventions of the existing web code (typed API client, React-escaped rendering, i18n key parity, 375 px layout).

#### Risk
`high`: a database migration and a change of the upload path.

#### Security & performance notes
Owner-only data stays behind the session; no user text is rendered as HTML; photos load in the `print` size; the book renders from one request.

#### Test plan
- Dev: unit and component tests, integration or Playwright where timing or layout matters; say what you could not check.
- QA should probe: Vietnamese long names, emoji notes, 0 and 40 notes, missing photos, A5 A4 Letter, 375 px, Chrome and Edge print to PDF (page size, fonts, backgrounds), slow network.
