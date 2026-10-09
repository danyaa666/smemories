# E05_T-056 — Book data endpoint for rendering

> **E10 contract note (2026-10-09, D-25..D-27):** this task is built on the v2 API contract and database conventions, not the v1 ones written below. Read `docs/api-contract.md`, `docs/db-conventions.md` and `docs/go-conventions.md` first. Wherever this spec names a `/v1/...` path, an old error code, an RFC 3339 time, a `cursor`, a foreign key or a `DATETIME` column, use the v2 equivalent (route map: `docs/api-contract.md` section 8). New tables are `_tab` tables with BIGINT ms timestamps and no foreign keys. Epic: `.team/epics/E10-skills-alignment/PRD.md`.

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P1 · **Type:** feature

#### Description
The HTML renderer in the browser (D-24) needs the whole book in one owner-only response: yearbook info, owner profile, the approved notes in order with their answers and photo ids, and the template and page size. No PDF is produced on the server. Replaces the export job T-014. Decision D-24.

#### Scope
- In: what the acceptance criteria below say.
- Out (do not do): extending the Go renderer (T-010, T-038 stay as fallback), the free-layout editor (E09), new page kinds (D-17).

#### Acceptance criteria
- [ ] AC1 — `GET /v1/yearbooks/{id}/book` (session) answers `200 {"yearbook":{title,school_name,class_name,graduation_year,motto,language,page_size,template_id,cover_media_id},"profile":{full_name,nickname,quote,hobbies,future_plans,photo_media_id},"notes":[{id,answers,photo_ids}],"note_count"}`; only notes with status `approved`, in `sort_order` then creation order; hidden and pending notes never appear. Another user's or a missing yearbook is `404 not_found`.
- [ ] AC2 — Bounded: at most 300 notes (the collection cap) and 3 photos each; one query for notes and one for photo links (no N+1, shown by a query-count test); response size stays under 1 MiB for 300 notes with maximal answers.
- [ ] AC3 — No storage keys, hashes, emails or internal ids in the response; media are referenced by public id and fetched through `GET /v1/media/{id}/content`.
- [ ] AC4 — `api/openapi.yaml`, `web/src/api/schema.d.ts` and the Postman collection are updated (owner, other user, pending and hidden notes excluded, ordering); the collection runs twice.

#### Design
See ADR `docs/adr/0003-html-print-export.md` and the prototype `web/src/spike/print/` for the measured approach; follow the conventions of the existing web code (typed API client, React-escaped rendering, i18n key parity, 375 px layout).

#### Risk
`high`: a new API contract that exposes personal data of third parties (notes); owner-only and approved-only filtering are the controls.

#### Security & performance notes
Owner-only data stays behind the session; no user text is rendered as HTML; photos load in the `print` size; the book renders from one request.

#### Test plan
- Dev: unit and component tests, integration or Playwright where timing or layout matters; say what you could not check.
- QA should probe: Vietnamese long names, emoji notes, 0 and 40 notes, missing photos, A5 A4 Letter, 375 px, Chrome and Edge print to PDF (page size, fonts, backgrounds), slow network.
