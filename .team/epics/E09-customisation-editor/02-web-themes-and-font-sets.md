# E09_T-142 — Web runtime: colour themes and font sets (Memory Book pilot)

**Epic:** E09-customisation-editor · **PRD:** [PRD](PRD.md) · **Milestone:** M1b · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-141. **Read first:** PRD "Stage 1 plan", `web/src/book/` (BookPages, templates/memory.tsx and memory.css, env.tsx, ready.ts, fit.tsx), `web/src/book/fonts/` and fonts.css, docs/templates.md, docs/adr/0003-html-print-export.md, `tools/print-check`, the T-116/T-121 lessons (the book is ready only when fonts and every photo have settled).

#### Requirements (SHALL)
1. **Design in the book env.** `BookData.design` (from `book-data/get`, T-141) is provided to the page components through `BookEnvProvider` (or a sibling `DesignProvider`); a missing design means all defaults. A helper `useDesign()` returns `{theme, fonts, text(id, fallback), image(id)}` already resolved.
2. **Themes as CSS custom properties.** Convert `memory.css` so every colour (about 8: `#5e5170`, `#fff1b5`, `#fcd5e2`, `#3b2f4a`, `#fffbf6`, `#ffdccb`, `#e6defa`, `#fff`) comes from `--mb-*` variables defined once per theme under `.mb-page[data-theme="lavender"]` (default = **exactly today's colours**), `peach` and `mint` (designed by you with contrast >= 4.5:1 for text on its background; list the pairs in `docs/templates.md`). Printing keeps `print-color-adjust: exact`.
3. **Font sets.** `playful` = today's stacks. `classic` = a serif display and a clean sans body, both **Vietnamese-capable OFL** families (D-18) that are not already shipped, or already-shipped ones such as Cormorant Garamond and Be Vietnam Pro if they fit (they do exist in `web/src/book/fonts/`: reuse before adding). Any new woff2 comes with its licence file, self-hosted (no request to a font CDN: the print-check asserts it), `font-display: block` as the others, and is loaded only when a book uses that font set. The ready signal waits for the fonts of the active set only.
4. **Default parity.** A book with no design, or the default design, renders pixel-identically to today: the existing print-check cases (69) and snapshots pass unchanged.
5. **Fit.** The text-fit rules still hold under `classic` (it is narrower or wider than `playful`): nothing is cut off beyond the documented summary; add print-check cases for each theme x font set on A5 and A4 with the Vietnamese fixture (long names, long notes).
6. No change to Navy Classic in this task; no new dependency; no API change beyond consuming T-141's field.

#### Acceptance criteria
- [ ] AC1 — Default design: print-check green with unchanged snapshots.
- [ ] AC2 — Each of the 3 themes x 2 font sets renders on A5 and A4 without cut-off text or blank pages; contrast pairs documented and checked by a unit test.
- [ ] AC3 — No request leaves the origin (fonts self-hosted); the font files of the inactive set are not requested.
- [ ] AC4 — Lint, typecheck, build, vitest, i18n parity, print-check (D-37: run it locally or start the full CI by hand and quote the run id).

#### Test plan
QA prints a Memory Book in each combination to PDF in Chrome and compares with the default, checks Vietnamese diacritics in the classic font, and checks the network log.
