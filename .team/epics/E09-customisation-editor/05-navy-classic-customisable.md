# E09_T-145 — Navy Classic becomes customisable

**Epic:** E09-customisation-editor · **PRD:** [PRD](PRD.md) · **Milestone:** M1b · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-144. **Read first:** PRD "Stage 1 plan", the T-141..T-144 specs and PRs (the pattern you repeat), `web/src/book/templates/navy.tsx` and `navy.css` (about 4 colours: `#c9a86a`, `#1b2a4a`, `#7a5c2b`, `#f7f3ea`, `#dcd6c9`), `internal/templates/embed/navy-classic.json`, docs/templates.md (Navy Classic section), `docs/templates/navy-classic-*`.

#### Requirements (SHALL)
1. **Manifest `design` block** for `navy-classic` (themes: today's navy and gold as the default plus two designed ones; font sets: today's Cormorant/Be Vietnam Pro as default plus one; sections: cover (fixed first), profile (hideable, movable), notes (movable), back (fixed last, hideable); texts for the headings it prints, e.g. "Autographs"; images: only if the design has any, else none).
2. **Theme tokens** as CSS custom properties with the default equal to today's look; contrast checked; font sets loaded on demand; sections/texts via `useDesign()` as in T-143.
3. **Default parity**: the existing Navy Classic print-check cases and snapshots (four notes per page, 50 % font floor, signature rules, long-name cases) pass unchanged; add print-check cases for each non-default theme and font set and for hidden/reordered sections on A5 and A4.
4. The customise screen (T-144) works for Navy Classic with no code change beyond the template's own i18n names (if it needs more than that, fix the screen generically).
5. docs/templates.md updated; no new dependency; no API change.

#### Acceptance criteria
- [ ] AC1 — Default design parity (snapshots and print-check unchanged).
- [ ] AC2 — Every theme x font set x size case renders without cut-off text or blank pages; signatures still fit.
- [ ] AC3 — The customise screen drives Navy Classic end to end (vitest with the manifest fixture) and the saved design prints.
- [ ] AC4 — Lint, typecheck, build, vitest, i18n parity, check:api, print-check (D-37: local or manual full CI with run id).

#### Test plan
QA customises a Navy Classic book through the screen, saves, prints to PDF, and compares with the preview.
