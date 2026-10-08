# T-054 — Spike: HTML templates and browser print-to-PDF instead of server rendering

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** spike

#### Description
Owner idea (2026-10-08): build the yearbook as an HTML page and let an "Export PDF" button open the browser's own print dialog ("Save as PDF"), with no server-side rendering. The owner's Claude Design canvases are already HTML and CSS, so templates could be React components with data props and CSS (`@page`), which would replace the Go renderer path, the import pipeline of E08 and the export job of T-014.
This spike proves or disproves it before more is built on the Go renderer. It decides nothing; the owner decides from the ADR (D-24).

#### Scope
- In: a throwaway prototype under `web/src/spike/print/` (route `/spike/print`, not linked from the UI, removed or kept behind a dev flag at the end); two designs: temp1 Memory Book (cover, profile, one friend page, back) and temp2 navy classic (cover, a letter or profile page, autographs); synthetic data (Vietnamese names, long text, emoji, 30 photos from fixtures); page sizes A5, A4 and Letter; a print button with an on-screen instruction panel; measurements; the ADR.
- Out (do not do): any backend change, production templates, replacing T-014 or T-019, deleting the Go renderer, a headless-Chromium service.

#### Acceptance criteria
- [ ] AC1 — The prototype renders the two designs' pages from data with CSS only for layout: fixed page boxes in mm, `@page { size: … ; margin: 0 }` per page size, `break-after: page`, `print-color-adjust: exact`, web fonts self-hosted as woff2 with Vietnamese coverage (no request to Google Fonts), photos loaded at print resolution, text that shrinks to fit its box (JS or CSS; state which). A "Save as PDF" button calls `window.print()` and shows the instructions (destination, paper size, margins none, background graphics on).
- [ ] AC2 — Test matrix with the result for each cell (works, works with caveats, fails, not tested and why): Chrome, Edge, Firefox and Safari on desktop; Android Chrome; iOS Safari. For each: resulting page size and orientation, whether margins and headers or footers appear by default, backgrounds, rotation, gradients and SVG, Vietnamese diacritics and emoji, embedded image resolution, page breaks across a 24-page book with 30 photos, time to open the print preview, peak memory if measurable, PDF file size. Save each browser's resulting PDF under `docs/spikes/print/` (synthetic data only, each under 5 MB).
- [ ] AC3 — Automated check: a Playwright script generates the same PDF headless with `page.pdf()` (Chromium) from the prototype and a test asserts page count, page size and that the Vietnamese sample text is extractable; state whether the output matches the interactive Chrome print.
- [ ] AC4 — Effort estimate for turning a Claude Design page into a data-bound component (hours per page, what had to change in the JSX) and what the pagination of friends' notes needs; compare with the E08 pipeline (T-039 to T-041).
- [ ] AC5 — `docs/adr/0003-html-print-export.md`: context, the matrix summary, risks that remain (phones, dialog settings, large books), what would be retired or kept if adopted (T-010/T-038 Go renderer, T-014, T-019, E08), a recommended decision, and an exit path (a headless-Chromium render of the same templates on the server). The leader brings it to the owner as D-24.

#### Design
Reuse the canvas pages from `design/canvas/` as the starting markup (unpack as in the E08 notes; the nested bundles contain the React source and CSS). Keep the data model of notes as answers by field id (D-21).

#### Risk
`low`: an isolated prototype and a document. Not merged to production paths.

#### Security & performance notes
User text is rendered by React (escaped); no `dangerouslySetInnerHTML`. Photos come from fixtures. Fonts are self-hosted (privacy and offline printing).

#### Test plan
- Dev: run the matrix as far as the machine allows and say honestly which cells could not be tested; attach PDFs.
- QA should probe: independently re-run Chrome and the Playwright script, check page size in a PDF inspector, check that fonts are embedded and Vietnamese text is selectable, check the claims about Firefox and Safari where tools exist.
