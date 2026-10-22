# E11_T-097 — Print: signatures in the memory-book and navy-classic templates

**Epic:** E11-signatures · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1b · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-095 (and the print code of T-058..T-061, merged). **Read first:** `web/src/book/` (renderer, `contain:size` fix, page-fit rules of T-058), `docs/templates.md`, `make print-check`.

#### Description
The printed book shows each approved note's signature next to or under the friend's message, as vectors, in the template's ink colour, without ever overlapping text.

#### Requirements (SHALL)
1. **Shared component** in `web/src/book/` (`BookSignature`): an SVG with the stroke polylines (`stroke-linecap/linejoin: round`, stroke width proportional to the box, `fill: none`, `vector-effect` not used, colour from the template's ink token), built only from validated integers; renders nothing for `null` or invalid data. Box size set by the template.
2. **Memory-book** (one friend per page): the signature sits at the bottom right of the message card, above the page footer, 40 mm x 16 mm on A5 (scaled by page size like the other measures), never over the message lines: the card reserves the space (message area shrinks, the existing text-fit rule applies).
3. **Navy-classic** (four notes per page): the signature sits at the right end of the name line, at most 24 mm x 9 mm; the per-page capacity rule of T-058 stays at four notes and its font floor stays at 50 %: the reserved height is part of the note card, so text fitting and the overflow summary (T-058/T-061) stay correct. A note without a signature keeps today's layout.
4. **Manifests.** Both manifests get `"signature": true` (T-095 switch); the Go fallback renderers are untouched.
5. **Print screen and book-data.** The preview and the browser print show the signature (same DOM); the print-size photo rules are unrelated. A book with 0 signatures renders byte-identically to today (snapshot test).
6. **Print check.** Extend `tools/print-check` with a fixture book containing signatures (a maximal one, a tiny one, none): the PDF page count and size are unchanged, text of the message is still extractable and not overlapped (compare text boxes with the signature box), and a rendered page (`pdftoppm`) has ink pixels inside the signature box. `make print-check` stays green and the owner's hands-on checklist (`docs/spikes/print/OWNER-TESTS.md`) gets one line to look at a signature on paper.
7. **Text export.** The plain-text export (T-093) lists no signature; add one line to `docs/book-export.md` saying so (no code).

#### Acceptance criteria
- [ ] AC1 — Component and snapshot tests for both templates: with and without signature, maximal signature, A5/A4/Letter.
- [ ] AC2 — `make print-check` and `make e2e` green; the e2e print step still reports the right page count with a signed note.
- [ ] AC3 — Real-browser screenshots (Chrome print preview) of both templates with signatures in the PR; no overlap at 4 notes per page with the longest note (2000 characters) plus a signature.
- [ ] AC4 — Lint, types and unit tests green.

#### Risk
`low`: print layout; a wrong layout is visible in the preview and the overflow summary.

#### Test plan
QA probes: Vietnamese long names next to the signature, 2000-character note with signature, A5/A4/Letter, Chrome and Edge print to PDF, a signature at the edge of the box, 0 and 40 notes, backgrounds off in the print dialog (ink only).

From QA on T-095, to settle when this task switches signatures on:
- Keep one Postman step for a template with signatures off, which must refuse a signature field.
- Make the oversize-signature error code the same whatever the template state, or document why it differs.
