# E08_T-040 — Template "memory-book" from temp1

**Epic:** E08-designer-templates · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** feature

#### Description
Pilot 1 (D-15): the pastel "Memory Book" design (`design/canvas/temp1.html`, US Letter) becomes the system template `memory-book` with the four page kinds, in English and Vietnamese, using the import tool (T-039) and the v2 format (T-038).
The owner compares the result with the canvas and approves it before it ships.

#### Scope
- In: `design/map/memory-book.json`; generated and hand-finished `internal/templates/embed/memory-book.json` and assets; Vietnamese-capable fonts for the design; the Vietnamese label table; sample PDF and comparison image; sample data in the pdf sample test.
- Out (do not do): the design's other pages (Our class, Class awards, Friend quiz, Note from my teacher; D-17), A4/A5 versions, changes to the renderer except bug fixes found here (report them as comments), any web or API change.

#### Acceptance criteria
- [ ] AC1 — Mapping as in the PRD table: cover ← Cover (title slot holds the whole yearbook title; school, class and year slots fill the small lines; `cover_photo` sits in the tilted frame, rotated like the frame), profile ← All about me (photo, `full_name`, `nickname`, `quote` as "My motto", `hobbies`, `plans` as "My dreams for the future"; the design's other boxes stay as printed blank fields), notes ← From a Friend (a `flow` with one item per page and `note_fields` name, how_we_met, first_impression, best_memory, wish and message, D-21: `name` in the name field, `how_we_met` in "How we met", `first_impression` in "Your first impression of me", `best_memory` in "Our best memory together", `wish` in "Your wish for me", `message` in the remaining big box, up to three `note_photo_*` in the photo areas; the design's other fields (nickname, birthday, phone, rating) stay as blank printed lines), back ← Let's stay in touch (static page; `motto` and `title` slots where the design has free space, otherwise left out). The dev documents any deviation in the pull request.
- [ ] AC2 — Fonts: the design uses Nunito, Fredoka and Gaegu (the canvas CSS carries no Vietnamese subset for Fredoka and Gaegu). Keep Nunito (check its coverage), and pick open-licence lookalikes with Vietnamese support for the other two (a rounded display face and a handwriting face). Add the TTF files (regular and bold where needed) under `internal/pdf/fonts/<family>/` with their OFL licence files and a row in `internal/pdf/fonts/README.md` (family, source URL, version, licence, the design font it replaces). The registry's coverage test passes for each.
- [ ] AC3 — Static labels: every static string of the design that is on a mapped page is in the template as `{"en","vi"}`. The dev proposes the Vietnamese text in a table in the pull request description (English, Vietnamese, page); **the owner reviews that table before merge**.
- [ ] AC4 — Fidelity: on the side-by-side image (`docs/templates/memory-book-compare.png`) every box matches the canvas within 1.5 mm; colours and decoration are identical (same pixels in the background); no slot text leaves its box with the sample data in either language.
- [ ] AC5 — Sample PDFs `docs/templates/memory-book.pdf` (English sample) and `docs/templates/memory-book-vi.pdf` (Vietnamese sample with long names, diacritics, an emoji note, three photos) are produced by the sample test (add the id to the loop) with synthetic data and no warnings except where the sample deliberately provokes one (documented).
- [ ] AC6 — The template passes the shared tests over `templates.List()` (T-035) and the T-038 validator; embedded assets stay under 8 MiB; `go test ./...` and `make lint` pass; the export time for a 24-page Letter book with 30 photos stays within the T-014 budget (measure and state it).

#### Design
Files: `design/map/memory-book.json`, `design/map/memory-book.report.md`, `internal/templates/embed/memory-book.json`, `internal/templates/embed/memory-book/*.png`, `internal/pdf/fonts/<family>/…`, `internal/pdf/fonts/README.md`, `docs/templates/memory-book*.pdf`, `docs/templates/memory-book-compare.png`, the sample test.
Workflow: run the tool, read its report, correct the mapping, rerun, then hand-edit only what the tool cannot decide, such as which static text is really a slot, line heights and the notes `flow` item height. Keep the final mapping so the tool reproduces the template.

#### Risk
`low`: data and font files plus tests; no endpoint or migration.

#### Security & performance notes
Font and image files are open licence or the owner's own; no network at run time. Check that no personal data appears in the canvas screenshots (the placeholders are generic).

#### Test plan
- Dev: run the tool, tests, sample PDFs in both languages, comparison image; measure the 24-page budget.
- QA should probe: very long names and notes in both languages (truncation with `…`, warning present), Vietnamese tone marks stacked on capitals, emoji notes, a book with 0 notes and one with 40, missing photos (placeholder, warning), profile with every optional field empty; check the PDF opens in two viewers and its page box is Letter.
