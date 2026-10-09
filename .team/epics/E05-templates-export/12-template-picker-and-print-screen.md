# E05_T-061 — Web: template picker and print screen with device guidance

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P1 · **Type:** feature

#### Description
The owner picks a template for the book's page size and prints. The print screen explains, per browser and device, the few settings the browser print dialog needs (destination Save as PDF, paper size, margins none, background graphics on) because those cannot be set from the page. Decision D-24.

#### Scope
- In: what the acceptance criteria below say.
- Out (do not do): extending the Go renderer (T-010, T-038 stay as fallback), the free-layout editor (E09), new page kinds (D-17).

#### Acceptance criteria
- [ ] AC1 — API `GET /v1/templates?page_size=` lists templates with id, names (en, vi), supported page sizes, `renderer` (`html` or `go`), and a preview image path; the web picker shows the ones for the book's page size and saves the choice (`PATCH` yearbook `template_id`); an invalid combination is refused with `400 invalid_template`.
- [ ] AC2 — Print screen: shows the overflow summary from T-058, the page count, and a short guide chosen from the user agent (Chrome and Edge desktop, Safari desktop, Android Chrome, iOS Safari, Firefox) with the exact steps and an "I printed with these settings" help link; a visible warning when the paper size or backgrounds look wrong cannot be detected, so the guide is the control. English and Vietnamese.
- [ ] AC3 — A feedback link after printing ("Does the PDF look right?" yes/no, stored as an anonymous counter per browser family, no personal data) so real-world failures on phones show up early; the counter endpoint is rate limited.
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
