# T-037 — US Letter page size, end to end

**Epic:** E08-designer-templates · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P2 · **Type:** feature

#### Description
The owner's designs are US Letter (D-16, decided in chat 2026-10-08, against the leader's recommendation to re-export at A5/A4). A yearbook can today be A5 or A4 only (`yearbooks.page_size ENUM('A5','A4')`).
This task adds `Letter` (215.9 × 279.4 mm) as a third page size across the database, the API, the template validator and the PDF page geometry. It builds no design and no web UI.

#### Scope
- In: migration; API validation and OpenAPI; page dimensions in `internal/pdf`; template declaration of a reference page and its allowed sizes; docs.
- Out (do not do): the web page-size selector (T-016 reads the new enum from the API schema), any design template (T-040, T-041), changing A5/A4 behaviour, other paper sizes.

#### Acceptance criteria
- [ ] AC1 — Migration `NNNN_page_size_letter.sql` (next free number at the time, `ls migrations/`; expected 0009): `ALTER TABLE yearbooks MODIFY page_size ENUM('A5','A4','Letter') NOT NULL DEFAULT 'A5'`. The Down part sets every `Letter` book to `A5` and restores the old enum; it runs on a database that holds a Letter book (test).
- [ ] AC2 — `POST /v1/yearbooks` and `PATCH /v1/yearbooks/{id}` accept `"page_size":"Letter"`; any other value still answers `400 invalid_page_size`. Existing books are untouched. `api/openapi.yaml`, `web/src/api/schema.d.ts` (`npm run gen:api`) and the Postman yearbooks collection (create Letter, patch to Letter, reject `"Legal"`) are updated and the collection runs twice back to back.
- [ ] AC3 — Page geometry: `internal/pdf` knows Letter as 215.9 × 279.4 mm; `Render` with Letter produces a PDF whose MediaBox is 612 × 792 pt (±0.5 pt). A test checks A5, A4 and Letter boxes.
- [ ] AC4 — Template spec gains `"reference": "A5"` (default) or `"Letter"`, the page the mm coordinates refer to. `page_sizes` must contain only sizes with the same aspect ratio as the reference (A5 reference allows A5 and A4; Letter reference allows Letter only); anything else fails validation naming the template. Elements must lie inside the reference page. `classic` and `modern` are unchanged and keep passing.
- [ ] AC5 — `templates.ForPageSize(size)` (or equivalent) returns the templates that support a size, for the picker (T-019) and the export check (T-014). A template whose `page_sizes` lacks the book's size must not be usable: a test calls the export-facing helper with Letter and gets neither `classic` nor `modern`.
- [ ] AC6 — `docs/templates.md` documents `reference`, the three sizes and the rule.

#### Design
Files: `migrations/NNNN_page_size_letter.sql`, `internal/yearbook/validate.go`, `internal/yearbook/*_test.go`, `internal/pdf/render.go` (page sizes), `internal/templates/{spec,validate,load}.go`, `api/openapi.yaml`, `postman/yearbooks.postman_collection.json`, `web/src/api/schema.d.ts`, `docs/templates.md`.

Scaling: for a template with reference R and target size S of the same ratio the renderer scales every coordinate by `S.width / R.width` (as today for A5 to A4). Letter has its own ratio (0.773) so a Letter template only prints on Letter.

#### Risk
`high`: a database migration on a table that holds user data (additive enum value; Down converts rows). The owner approves the merge.

#### Security & performance notes
Only a fixed enum value is accepted from the client. Nothing here reads files or runs queries on user input beyond the existing parameterised ones.

#### Test plan
- Dev: validator table tests (A5 reference with A4, Letter reference with A5 fails, a coordinate outside the reference page fails); migration up/down/up on a database with an A5, an A4 and a Letter book; page-box test for all three sizes; Go integration test for create/patch with Letter; Postman collection twice.
- QA should probe: patch an A4 book to Letter and back; create with `"letter"` (lower case) and `"LETTER"` (both rejected); export-facing helper with each size; migrate down with a Letter book present.
