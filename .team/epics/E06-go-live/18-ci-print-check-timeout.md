# E06_T-149 — CI: the e2e job's Print check step times out (raise the limits)

**Epic:** E06-go-live · **Milestone:** M2 · **Risk:** low · **Priority:** P2 · **Type:** ci
**Depends on:** none. **Read first:** `.github/workflows/ci.yml` (job `e2e`, steps "Print check" and "Install Chromium"), `docs/e2e.md`, CLAUDE.md "CI scope" (D-37).

#### Why
The manual full CI run 41052503585 (T-146, head 03f42df) went red on `e2e` only because the "Print check" step hit its `timeout-minutes: 6`; no test failed. The print-check suite grew with E09 (94 tests at T-142, 112 at T-143, 163 at T-146) and takes about 319 s locally, so on the slower runner it no longer fits 6 minutes. The job is manual-only (D-37), so this goes unnoticed until someone starts a run, and then every full run is red on a false alarm.

#### Requirements (SHALL)
1. In `ci.yml` job `e2e`: raise the "Print check" step `timeout-minutes` from 6 to 15 and the job `timeout-minutes` from 15 to 25 (the journey keeps its own 240 s limit); update the comment on the job line that quotes "print-check to 6 min".
2. Do not change which tests run, any other job, the `if:` conditions or the paused/manual scheme (D-37, D-38).
3. The step already prints its own duration; keep that line. Add a one-line note to `docs/e2e.md` with the current suite size and time (163 tests, about 5-6 min locally) and the new limit.
4. Prove it: start a manual full run on the task branch (`gh workflow run ci.yml --ref <branch>`), link the run id, and show `e2e` green with the "print-check: N s" line. If it still fails on time, report N and stop (the next step would be splitting the suite by template, a separate task).
5. Nothing under `.team/`; no test code change.

#### Acceptance criteria
- [ ] AC1 — On the task branch, a manual full run has all six jobs green, `e2e` included; the run id and the printed "print-check: N s" are in the PR.
- [ ] AC2 — The diff is limited to the two `timeout-minutes` values, the job comment and the `docs/e2e.md` note; `actionlint` (or the repo's workflow lint) is clean.
- [ ] AC3 — On the PR itself `go`, `web` and `security` pass and the three paused jobs show as skipped.

#### Test plan
QA checks the diff scope, starts or confirms the manual run, and confirms the PR check list.
