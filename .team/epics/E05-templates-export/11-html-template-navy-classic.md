# E05_T-060 — HTML template "navy-classic" (from design temp2)

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** feature

#### Description
The navy and gold canvas (`design/canvas/temp2.html`) as the second HTML template: cover, profile (the welcome-letter layout with an oval photo), autographs as the notes page, and a back page derived from the cover frame. Replaces the Go-renderer pilot T-041. Decision D-24.

#### Scope
- In: what the acceptance criteria below say.
- Out (do not do): extending the Go renderer (T-010, T-038 stay as fallback), the free-layout editor (E09), new page kinds (D-17).

#### Acceptance criteria
- [ ] AC1 — Same method and review as T-059: components for the four page kinds, note fields `name`, `relationship`, `message` for the autograph lines, Cormorant Garamond plus a Vietnamese-capable lookalike for Jost, EN and VI static labels with an owner-reviewed table, side-by-side fidelity image, `make print-check`, sample PDFs.
- [ ] AC2 — The oval photo mask works with portrait and landscape photos (cover fit); the notes page flows 1, 4 and 9 notes.

#### Design
See ADR `docs/adr/0003-html-print-export.md` and the prototype `web/src/spike/print/` for the measured approach; follow the conventions of the existing web code (typed API client, React-escaped rendering, i18n key parity, 375 px layout).

#### Risk
`low`.

#### Security & performance notes
Owner-only data stays behind the session; no user text is rendered as HTML; photos load in the `print` size; the book renders from one request.

#### Test plan
- Dev: unit and component tests, integration or Playwright where timing or layout matters; say what you could not check.
- QA should probe: Vietnamese long names, emoji notes, 0 and 40 notes, missing photos, A5 A4 Letter, 375 px, Chrome and Edge print to PDF (page size, fonts, backgrounds), slow network.
