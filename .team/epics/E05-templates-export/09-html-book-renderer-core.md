# E05_T-058 — Web: HTML book renderer core and print preview

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature

#### Description
Core of D-24: the web app renders the whole book as fixed-size HTML pages (exact mm boxes, `@page` size from the book's page size), previews it on screen, and prints it through the browser. Reuses and hardens the prototype of spike T-054 (`web/src/spike/print`, ADR 0003). Decision D-24.

#### Scope
- In: what the acceptance criteria below say.
- Out (do not do): extending the Go renderer (T-010, T-038 stay as fallback), the free-layout editor (E09), new page kinds (D-17).

#### Acceptance criteria
- [ ] AC1 — Route `/yearbooks/:id/book` loads `GET /v1/yearbooks/{id}/book` (T-056) and renders the pages of the chosen template (components registered by template id; until T-059 a minimal built-in layout proves the pipeline). Page sizes A5, A4 and Letter via CSS variables; screen view scales pages to the viewport, print view is exact (`@page { size; margin: 0 }`, `print-color-adjust: exact`, `break-after: page`).
- [ ] AC2 — Pagination: the notes page repeats; the number of notes per page comes from the template's declared capacity or a measured fit; a book with 0, 1 and 40 notes paginates correctly (tests).
- [ ] AC3 — Text fit: a hook shrinks text in 0.5 px steps down to a per-element minimum and reports overflow; the owner sees a summary ("3 notes were shortened") before printing.
- [ ] AC4 — Print readiness: the Print button stays disabled until fonts and all images (the `print` size, T-057) have loaded; images load lazily on screen but eagerly for print; a book of 24 pages with 30 photos prints in the spike's measured time class on Chromium.
- [ ] AC5 — Fonts are self-hosted woff2 with Vietnamese coverage (no third-party request); emoji handled as in the spike notes.
- [ ] AC6 — Automated check: `tools/print-check` (Playwright, from the spike) renders a fixture book to PDF headless and asserts page count, page size, embedded fonts and that Vietnamese sample text is extractable; it runs in CI as an optional job or `make print-check`.
- [ ] AC7 — The spike route and fixtures are removed or folded into this feature; ADR 0003 is marked accepted with the owner's decision (D-24).

#### Design
See ADR `docs/adr/0003-html-print-export.md` and the prototype `web/src/spike/print/` for the measured approach; follow the conventions of the existing web code (typed API client, React-escaped rendering, i18n key parity, 375 px layout).

#### Risk
`low`: front end only on top of an existing API.

#### Security & performance notes
Owner-only data stays behind the session; no user text is rendered as HTML; photos load in the `print` size; the book renders from one request.

#### Test plan
- Dev: unit and component tests, integration or Playwright where timing or layout matters; say what you could not check.
- QA should probe: Vietnamese long names, emoji notes, 0 and 40 notes, missing photos, A5 A4 Letter, 375 px, Chrome and Edge print to PDF (page size, fonts, backgrounds), slow network.

#### Added by the leader (2026-10-16, from T-056)
The book-data response has `truncated` (true when the book has more approved notes than the 300-note cap returned) and `approved_total`. When `truncated` is true the preview and the print screen MUST show a visible warning ("X of Y notes are included") and must not silently print the shorter book; add a test for it.

#### Added by the leader (2026-10-16, from the T-056 QA)
Do not assume the book-data response is under 1 MiB: measured 693 KB (ASCII, 300 notes x 3 photos), 1.02 MiB (dense Vietnamese), 2.48 MiB (emoji), 3.7 MiB (text full of `<` and `&`, escaped by Go). Parse it as a normal fetch with no size assumption, show a loading state for slow networks, and test with a 300-note fixture. Compression is an E06 deployment matter (load balancer / CDN gzip), not part of this task.
