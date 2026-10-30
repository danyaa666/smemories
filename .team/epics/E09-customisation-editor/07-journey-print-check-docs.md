# E09_T-147 — Customisation journey, print-check coverage and docs

**Epic:** E09-customisation-editor · **PRD:** [PRD](PRD.md) · **Milestone:** M1b · **Risk:** low · **Priority:** P2 · **Type:** test+docs
**Depends on:** T-146. **Read first:** `tools/e2e` (the M1 journey, `class.spec.mjs`, helpers, scrub-trace), `tools/print-check`, docs/e2e.md, docs/templates.md, the PRD "Stage 1 plan" and exit criteria.

#### Requirements (SHALL)
1. **Journey** `tools/e2e/design.spec.mjs` (runs in the e2e job, by hand per D-37): a student with a Memory Book collects and approves one friend note, opens the customise screen, picks a theme and a font set, hides the profile page, edits the cover title, saves, reloads and sees the same choices; opens the print route and asserts the page count, that the hidden section is absent, the title is in the cover and the theme variable is applied; resets to the template and sees the default look; switches the template to Navy Classic and sees the design reset. Credentials come from the existing helpers (SMEM_ENV=test rules in CLAUDE.md); no token or code printed. Under 2 extra CI minutes.
2. **Print-check** coverage summary: list in docs/templates.md every theme x font set x size case that print-check covers for both templates (generated or hand-written table) and how to add a theme.
3. **Docs**: docs/templates.md "Design block and book designs" (already started in T-141) is complete: manifest keys, options document, limits, how a template author adds a theme/font set/section/text/image, the contrast rule; README product description gets two lines about customising the design; PRD exit criteria of stage 1 checked off in the board comment (the leader updates the PRD).
4. **Exit check** (stage 1 "stable"): a student customises colours, fonts, section order and visibility, sees the preview, resets, and the printed PDF matches the preview; notes and photos survive a template change. State in the PR which of these the journey covers and which QA verified by hand.
5. **Carry-overs (2026-10-30).** (a) `docs/e2e.md` does not mention T-146's new class journey step 7b (the design reaches the class admin only: students A and B get 403): add it. (b) T-149 raised the print-check CI step limit to 15 min; this task adds journey time, so quote the new journey duration and keep it within its 240 s limit.

#### Acceptance criteria
- [ ] AC1 — The journey passes in a manual full CI run and locally; a deliberate break (the print route ignores the design) makes it fail.
- [ ] AC2 — The docs let a new template author add a theme without reading the code.
- [ ] AC3 — Lint, build, vitest, print-check, check:api; CI jobs that run are green.

#### Test plan
QA runs the journey, follows the docs to add a throwaway theme to a template in a scratch branch, and verifies the exit criteria by hand in Chrome.
