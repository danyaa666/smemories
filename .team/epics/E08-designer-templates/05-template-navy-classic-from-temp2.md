# T-041 — Template "navy-classic" from temp2

**Epic:** E08-designer-templates · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** feature

#### Description
Pilot 2 (D-15): the navy and gold "Class of 2026" design (`design/canvas/temp2.html`, US Letter) becomes the system template `navy-classic`. It exercises what memory-book does not: an oval photo mask, serif typography, and a back page derived from the cover.
Same method and review as T-040.

#### Scope
- In: `design/map/navy-classic.json`; the template JSON and assets; fonts; label table; sample PDFs and comparison image.
- Out (do not do): Graduate portraits, Class honors, Year in review (D-17), A4/A5 versions, renderer changes except bug fixes found here.

#### Acceptance criteria
- [ ] AC1 — Mapping as in the PRD table: cover ← Cover (title, school, class and year slots in the gold-on-navy frame; the "Class of 2026" wording becomes `class` and `year` data, not fixed text); profile ← Welcome letter (oval `photo`; the three paragraphs are the `quote`, `hobbies` and `plans` slots; the handwritten signature is `full_name`, with `nickname` under it; the heading "Dear Class of 2026," becomes static localised text such as "A few words from me"/"Vài lời từ mình", dev proposes, owner reviews); notes ← Autographs (the signing lines become the note items of a `flow` with `note_fields` name, relationship and message, D-21: `name` at the line, `message` above it); back ← the Cover's navy frame without the title, with `motto` and `title` slots (two template pages from one canvas page, T-039 AC6).
- [ ] AC2 — Fonts: Cormorant Garamond (Vietnamese-capable per the canvas CSS) is kept; Jost has no Vietnamese subset in the canvas CSS, so choose a Vietnamese-capable open-licence lookalike for it. Files, licences and README rows as in T-040 AC2.
- [ ] AC3 — Static labels in EN and VI with the owner-reviewed table (as T-040 AC3).
- [ ] AC4 — Fidelity within 1.5 mm and identical decoration on `docs/templates/navy-classic-compare.png`; the oval mask works with a portrait and with a landscape photo (cover fit).
- [ ] AC5 — Sample PDFs `docs/templates/navy-classic.pdf` and `navy-classic-vi.pdf`, shared tests, size and budget checks as in T-040 AC5 and AC6.

#### Design
Files: as T-040 with the id `navy-classic`.

#### Risk
`low`.

#### Security & performance notes
As T-040.

#### Test plan
- Dev: as T-040.
- QA should probe: as T-040 plus a notes page with 1, 4 and 9 notes (the autograph lines flow and paginate), and the back page without a motto (empty slot, nothing drawn, no warning).
