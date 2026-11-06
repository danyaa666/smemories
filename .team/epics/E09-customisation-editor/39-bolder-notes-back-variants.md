# E09_T-180 — Make the notes and back template choices a real choice (Memory Book and Navy Classic)

**Epic:** E09-customisation-editor · **Milestone:** M4 · **Risk:** low (content, manifest ids and tests; no runtime or server change allowed here) · **Priority:** P3 · **Type:** feature
**Depends on:** T-177 (merged), T-178 (Navy Classic templates, same limitation). **Read first:** specs 36 and 37, `internal/templates/embed/pagetemplates/*/`, `internal/booklayout` (what a page template may override), `docs/templates.md` ("Layout block and layout document"), `docs/images/page-templates-memory-book.png`, QA's T-177 comment.

#### Why
QA on T-177 (2026-11-06): covers and profiles are a real choice, but **notes-\* and back-\* are the same page with one or two corner stickers**; notes-airy vs notes-doodle differ mainly in white vs paper colour, and back-doodle and back-pact are near twins. The notes and back pages flow their content (the notes list, the back text block), so only the heading and decoration can differ today. A student who opens the Templates panel on a notes page sees four cards that look alike. The leader accepted this for the first merge; this task fixes it without touching the runtime.

#### Requirements (SHALL)
1. **Inventory first** (dev's first deliverable, a table in the PR body): for the notes and back page of each template, which element ids a page template may override or add today (heading box, content flow area margins, decoration items, bg) and the free area left on A5, A4 and Letter.
2. **Bolder variants within today's rules**: for each of notes and back, in Memory Book and Navy Classic, at least **four looks that differ at a glance on the contact sheet**, using only what step 1 allows: larger decoration clusters in the free margins (corner, edge strip, footer band), a repositioned or restyled heading where the layout allows it, shapes and stickers from the shipped registry, background colour from the contrast-safe list. Keep the existing gates: contrast at least 4.5:1, no prose, no image items, boxes inside the margin, print-check on A5/A4/Letter, distinctness.
3. **A stronger distinctness gate**: the Go gate rejects two templates of the same kind whose decoration boxes overlap by more than a stated share (propose it, e.g. IoU of the union of item boxes below 0.6) or that differ only by `bg.color`. A test proves the gate fails on a copy with a changed colour only.
4. **If step 1 shows that real variety needs the content blocks to move** (heading above/below, notes list in two columns, back text block placement), do **not** change the runtime or validator here: write the finding and a proposed design on the board, set this task to `NEEDS_DECISION`-style comment for the leader, and ship whatever variety fits today's rules.
5. Updated contact sheets (`docs/images/page-templates-*.png`) and `docs/templates.md` paragraph; nothing under `.team/`; no new dependency.

#### Acceptance criteria
- [ ] AC1 — The inventory table is in the PR; each notes and back page has four looks that a reviewer can tell apart in the contact sheet without reading the labels.
- [ ] AC2 — The distinctness gate catches colour-only twins and heavy overlap (mutation proof), and all existing gates pass.
- [ ] AC3 — `make test-integration`, print-check (A5, A4, Letter, every template) and the e2e suite pass with result lines and full head SHA in the QA comment.

#### Test plan
Dev: gates and print-check; contact sheet. QA: runs everything on the PR head; looks at every notes and back template on A4 with a long-name and a no-photo student; mutation-checks the new gate; checks that the profile and cover templates are unchanged byte for byte.
