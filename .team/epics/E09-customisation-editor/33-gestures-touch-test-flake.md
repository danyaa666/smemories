# E09_T-174 — Stabilise the editor-gestures touch test (flake: 2 to 3 px against 0.5 allowed)

**Epic:** E09-customisation-editor · **Milestone:** M4 · **Risk:** low · **Priority:** P3 · **Type:** test
**Depends on:** none. **Read first:** `tools/e2e/editor-gestures.spec.mjs` (test "touch at 768 px: the first tap selects, a drag of the selected item moves it, a swipe over an unselected item pans the stage", around line 482; the assertion "the item stayed where it is on the page" around line 505), `docs/e2e.md`, `web/src/book/editor/gestures/useGestures.ts` (touch path, pan versus drag).

#### Evidence
The same assertion failed intermittently in three separate runs: T-165 dev run (2 px, once), T-166 QA full e2e runs 1 and 2 (3 px against the 0.5 px tolerance), and passed in all other runs including 3 isolated ones. Not reproduced on a quiet machine; it appears under load, when a full suite runs back to back. Cause unknown: either the test measures while the stage scroll or momentum has not settled (item and sheet read in separate evaluations), or the product moves the item by a few px during a pan (a drag threshold crossed).

#### Requirements (SHALL)
1. Find the cause with evidence: run the test 30 times in isolation and 10 times at the tail of the full suite on a private stack; capture the item and sheet rects as one atomic read (one `evaluate` returning both) and after the scroll has settled (two animation frames with an unchanged scroll position); report which case produced the drift.
2. If it is a test timing problem, fix the test (atomic measurement after the scroll settles; keep the 0.5 px tolerance, do not widen it). If the product moves an item during a pan, fix the product code (a pan must never change an item's stored box: assert by checking the layout document is unchanged) and keep the strict assertion.
3. The result: 30 isolated and 10 full-suite runs without a failure; the numbers go in the PR. No change to other tests' tolerances.
4. Docs: one line in `docs/e2e.md` if the settle helper becomes a shared helper for other touch tests.
5. No new dependency; nothing under `.team/`.

#### Acceptance criteria
- [ ] AC1 — The cause is stated with evidence (test timing or product), and fixed at its source.
- [ ] AC2 — 30 isolated and 10 full-suite runs pass with the 0.5 px tolerance unchanged.
- [ ] AC3 — Lint and prettier for `tools/e2e` (and `web` if product code changed) green; if product code changed, vitest and the gesture unit tests green.

#### Test plan
QA runs the touch test 20 times in isolation and the full e2e suite three times; if product code changed, repeats the T-160 touch checks by hand.
