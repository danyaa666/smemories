# E09_T-150 — Spike: layout editor architecture and ADR 0004

**Epic:** E09-customisation-editor (stage 2) · **Milestone:** M4 · **Risk:** low · **Priority:** P2 · **Type:** spike
**Depends on:** none. **Read first:** E09 PRD "Stage 2 plan", `docs/adr/0003-html-print-export.md`, `docs/templates.md` (units, design block), `web/src/book/templates/memory.tsx` and `memory.css`, `web/src/book/BookPages.tsx`, `docs/spikes/print/` (how the print spike was kept), owner decision D-43.

#### Why
The owner chose a DOM editor on our own book renderer (D-43). Before the layout model and the editor are specified, five questions need measured answers, because each changes the data model or the UI. This spike answers them with a throw-away prototype and writes the decisions down. It adds **no dependency to `web/`**; the prototype lives in its own folder.

#### Questions to answer (each with a measurement or an experiment, not an opinion)
1. **Units and page sizes.** A5 (148 x 210), A4 (210 x 297) and Letter (215.9 x 279.4) have different aspect ratios (0.705, 0.707, 0.773). Choose how a layout stores boxes so one layout prints well on all three: percentages of the page width and height, millimetres on the A5 reference page with uniform scaling, or something else with an anchor rule. Show the Memory Book cover with one sticker and one text box on all three sizes for each candidate and pick one. Say what happens to an item near the bottom edge on Letter.
2. **Drag, resize, rotate, snap.** Try `react-moveable` (MIT) and an in-house pointer-events implementation on the Memory Book cover rendered by the real components. Compare: lines of glue code, bundle size, touch behaviour on a 768 px viewport, keyboard control, snapping to page centre, edges, safe margin and other items, maintenance status and licence. Pick one and say why. Measure frame time while dragging with 100 items under Chrome's 4x CPU throttle (target: 30 frames per second or better).
3. **Text editing in place.** Compare a `contentEditable` element with an overlaid `textarea` for a text box that is part of the printed page. Prove Vietnamese input: Telex and VNI composition (Playwright with `compositionstart/update/end` events, plus a short manual note on macOS Vietnamese Telex), caret behaviour, paste as plain text, a maximum of 500 runes, line breaks. Decide how text that is too long behaves (fixed box with shrink-to-fit down to a minimum size, like the guided templates, or clip) and prove that the editor and the print show the same lines (same CSS, same fonts).
4. **Undo and redo.** For a document of at most 128 KiB, compare a snapshot stack with a command stack (memory, complexity, grouping a drag into one step, autosave interplay). Recommend one.
5. **Print parity.** With the prototype layer on the cover, print to PDF (the print-check method) on A5, A4 and Letter and compare each page with the editor view at the same scale (pixel difference, report the largest). Confirm that fonts, rotation and the safe margin match.
Also note (no full proof needed): what works with touch on a 375 px phone (pan, tap to select, move up/down buttons), and the accessibility path for keyboard move, resize and rotate (arrow keys, announcements).

#### Requirements (SHALL)
1. Write `docs/adr/0004-layout-editor.md` (status "proposed" until the leader accepts it): the five answers with the numbers, the chosen library and licence, the chosen unit and rule, the text-editing technique, the undo technique, the risks left, and a list of changes to the stage 2 task sketch (E09 PRD) that the findings imply.
2. Keep the prototype under `docs/spikes/editor/` with its own `package.json` and a `README.md` saying how to run it and reproduce the numbers; nothing in `web/`, `internal/` or CI changes.
3. If a candidate fails a question (for example Vietnamese IME breaks in-place editing), say so plainly and give the fallback; do not hide it.
4. At most two dev wake-ups; if a question cannot be answered in that time, report what is missing instead of guessing.

#### Acceptance criteria
- [ ] AC1 — ADR 0004 answers questions 1 to 5, each with a measurement, and states the recommended unit, library, text technique and undo technique.
- [ ] AC2 — The prototype runs from its README on a clean checkout and reproduces the frame-time and parity numbers.
- [ ] AC3 — The Vietnamese input proof shows a real composed word (for example "Việt Nam", "đường") typed through composition events and committed correctly in the chosen technique.
- [ ] AC4 — No change outside `docs/adr/0004-*`, `docs/spikes/editor/` and the PR text.

#### Test plan
QA runs the prototype from the README, checks the numbers are reproducible within noise, repeats the Vietnamese proof by hand, and checks that the diff touches only the paths above.
