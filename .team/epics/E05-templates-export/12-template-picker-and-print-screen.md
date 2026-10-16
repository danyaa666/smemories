# E05_T-061 — Web: template picker and print screen with device guidance

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P1 · **Type:** feature

#### Description
The owner picks a template for the book's page size and prints. The print screen explains, per browser and device, the few settings the browser print dialog needs (destination Save as PDF, paper size, margins none, background graphics on) because those cannot be set from the page. Decision D-24.

#### Scope
- In: what the acceptance criteria below say.
- Out (do not do): extending the Go renderer (T-010, T-038 stay as fallback), the free-layout editor (E09), new page kinds (D-17).

#### Acceptance criteria
- [ ] AC1 — v2 API (envelope `{status,data}`, `docs/api-contract.md`, namespace `template`): `GET /api/template/get-list?page_size=` (session) lists the templates available for that page size with id, names (en, vi), supported page sizes, `renderer` (`html` or `go`) and a preview image path; the web picker shows the ones for the book's page size and saves the choice through `POST /api/yearbook/update` with `template_id` (today `BookInput` has no `template_id`: add it, validated in the yearbook Service against the template registry in `internal/templates`; `null` clears it); an unknown template or one that does not support the book's page size is refused with `ERROR_INVALID_TEMPLATE` (400). openapi, `schema.d.ts` and the Postman collection are updated. Canonical-ULID/format rules as in T-013. Note from T-059: the memory-book template exists as an HTML template with the manifest `internal/templates/embed/memory-book.json`; before this task nothing sets `template_id`, so a book without one must still preview with a sensible default (keep that behaviour).
- [ ] AC2 — Print screen: shows the overflow summary from T-058, the page count, and a short guide chosen from the user agent (Chrome and Edge desktop, Safari desktop, Android Chrome, iOS Safari, Firefox) with the exact steps and an "I printed with these settings" help link; a visible warning when the paper size or backgrounds look wrong cannot be detected, so the guide is the control. English and Vietnamese.
- [ ] AC3 — A feedback link after printing ("Does the PDF look right?" yes/no, stored as an anonymous counter per browser family, no personal data) so real-world failures on phones show up early; the counter endpoint is a v2 write (`POST /api/print-feedback/create`, namespace `print-feedback`, no session needed, so it is public: CSRF origin check, strict body, per-IP rate limit like the other public endpoints, only `{verdict: yes|no, browser_family: enum}` accepted, stored as counters only).
- [ ] AC4 — Tests: picker filtering, guide selection per user agent, error states; real-browser check at 375 px.

#### Design
See ADR `docs/adr/0003-html-print-export.md` and the prototype `web/src/spike/print/` for the measured approach; follow the conventions of the existing web code (typed API client, React-escaped rendering, i18n key parity, 375 px layout).

#### Risk
`high`: a small public-ish API (templates list and feedback counter) and template validation on the yearbook.

#### Security & performance notes
Owner-only data stays behind the session; no user text is rendered as HTML; photos load in the `print` size; the book renders from one request.

#### Test plan
- Dev: unit and component tests, integration or Playwright where timing or layout matters; say what you could not check.
- QA should probe: Vietnamese long names, emoji notes, 0 and 40 notes, missing photos, A5 A4 Letter, 375 px, Chrome and Edge print to PDF (page size, fonts, backgrounds), slow network.

#### Added by the leader (2026-10-16, from T-056)
The book-data response has `truncated` (true when the book has more approved notes than the 300-note cap returned) and `approved_total`. When `truncated` is true the preview and the print screen MUST show a visible warning ("X of Y notes are included") and must not silently print the shorter book; add a test for it.

#### Added by the leader (2026-10-16, from the T-058 QA)
T-058 sets four notes per page and shrinks long text down to a floor of 50% (a note over ~850 characters prints under 7 pt on A5; the 2000-character maximum about 4 pt; pathological text is clipped inside its card). The owner currently only sees a count. Required here: the preview/print screen lists WHICH notes were shrunk or cut (author name and how small, e.g. "Linh, 6 pt"), each with a link to that note in the moderation inbox so the owner can hide or ask the friend to shorten it; a test with one 2000-character note. Do not change the capacity rule in this task unless the template needs it.
