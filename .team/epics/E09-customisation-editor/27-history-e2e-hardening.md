# E09_T-168 — History e2e hardening: friendly mysql-missing message, QA probe cases (tests only)

**Epic:** E09-customisation-editor · **Milestone:** M4 · **Risk:** low · **Priority:** P3 · **Type:** test
**Depends on:** none (T-165 merged). **Read first:** `tools/e2e/editor-history.spec.mjs`, `tools/e2e/playwright.config.mjs`, `docs/e2e.md`, `docs/editor-history.md`, the QA probe `.team/epics/E09-customisation-editor/attachments/editor-history-qa.spec.mjs.txt` (copy it from the main checkout; it is a QA draft, not reviewed code).

#### Requirements (SHALL)
1. When the `mysql` client is not installed, `editor-history.spec.mjs` (and any helper it uses to back-date snapshots) fails or skips with one clear message ("the history e2e needs the mysql client on PATH; see docs/e2e.md") instead of a raw `ENOENT`. Prefer a skip with the message when running outside CI so a missing client does not look like a product bug; the decision and the reason go in `docs/e2e.md`, which also lists the client as a prerequisite (it may already; check).
2. Turn QA's probe into a committed spec (or extra tests in the same file) covering: 33 back-dated saves give 30 rows on the server and in the panel; three restores in a row keep 30; a restore with a pending autosave saves the pending edit first and shows it as the newest "Before restore" row. Keep the runtime of the whole history spec under about 60 s (the probe takes ~50 s alone: seed snapshots by direct insert instead of 33 real saves where the point is the list, keep real saves for the pending-autosave case). Add the file to `testMatch` in `playwright.config.mjs` if the config lists files.
3. No product code change; nothing under `.team/`; no new dependency.

#### Acceptance criteria
- [ ] AC1 — Without a `mysql` client the spec prints the friendly message and does not report a failure caused by ENOENT.
- [ ] AC2 — The three probe cases pass in `scripts/e2e.sh` (two consecutive runs), and the full e2e suite stays green; total history spec time documented in the PR.
- [ ] AC3 — Lint and prettier for `tools/e2e` green.

#### Test plan
QA runs the e2e suite twice on a private stack and once with the `mysql` client hidden from PATH.

#### Added 2026-11-05 (leader, from the T-170 QA)
QA pushed `tools/e2e/editor-flow-stale-qa.spec.mjs` on PR #166 (T-170): a probe that opens the manage page, approves a note through the API, clicks 'Edit notes pages' and expects the editor on the notes page with a book-data cache that predates the note (fails on the old EditorShell, passes after the T-170 fix). It is not in `testMatch` (QA cannot edit the config). Add it to `testMatch` in `tools/e2e/playwright.config.mjs` (or fold its case into `editor-flow.spec.mjs`), keep it green twice, and add a line to `docs/e2e.md`. Optional, product, tiny: in `EditorShell`, when the Done navigation is cancelled by a Back pressed inside the ~20 ms between the URL change and React's commit, the editor stays with Done disabled and the leave guard released; reset `finishing` on a cancelled navigation if that is a few lines (otherwise leave it and say so).
