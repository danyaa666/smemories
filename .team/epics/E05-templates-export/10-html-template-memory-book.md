# E05_T-059 — HTML template "memory-book" (from design temp1)

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** feature

#### Description
The pastel Memory Book canvas (`design/canvas/temp1.html`, US Letter) becomes an HTML template component for the renderer of T-058, with cover, profile (All about me), friend page (one note per page) and back (Let's stay in touch), in English and Vietnamese. Replaces the Go-renderer pilot T-040. Decision D-24.

#### Scope
- In: what the acceptance criteria below say.
- Out (do not do): extending the Go renderer (T-010, T-038 stay as fallback), the free-layout editor (E09), new page kinds (D-17).

#### Acceptance criteria
- [ ] AC1 — Components reproduce the design (colours, shapes, illustrations as inline SVG or CSS, tilted frames and tape as CSS transforms) for the four page kinds of D-17; unmapped design pages stay unused. Data binds to profile fields, photos and the note fields `name`, `how_we_met`, `first_impression`, `best_memory`, `wish`, `message` (D-21; T-044 manifest).
- [ ] AC2 — Fonts: Nunito (Vietnamese-capable) plus Vietnamese-capable open-licence lookalikes for Fredoka and Gaegu, self-hosted woff2 with OFL licence files and a row in the fonts README; coverage tested with a Vietnamese pangram.
- [ ] AC3 — Static labels in EN and VI through i18n keys; the Vietnamese table is listed in the pull request for the owner's review before merge.
- [ ] AC4 — Fidelity: a side-by-side image of canvas page and rendered page (Playwright screenshots) shows boxes within 1.5 mm; no text leaves its box with long Vietnamese and emoji samples.
- [ ] AC5 — `make print-check` passes for a Letter book with 24 pages and 30 photos; sample PDFs for owner review in `docs/templates/` (each under 2 MB; photos are synthetic).

#### Design
See ADR `docs/adr/0003-html-print-export.md` and the prototype `web/src/spike/print/` for the measured approach; follow the conventions of the existing web code (typed API client, React-escaped rendering, i18n key parity, 375 px layout).

#### Risk
`low`.

#### Security & performance notes
Owner-only data stays behind the session; no user text is rendered as HTML; photos load in the `print` size; the book renders from one request.

#### Test plan
- Dev: unit and component tests, integration or Playwright where timing or layout matters; say what you could not check.
- QA should probe: Vietnamese long names, emoji notes, 0 and 40 notes, missing photos, A5 A4 Letter, 375 px, Chrome and Edge print to PDF (page size, fonts, backgrounds), slow network.
