# E06_T-132 — CI: pause the go-integration and e2e jobs (owner decision D-37)

**Epic:** E06-go-live · **Milestone:** M2 · **Risk:** low · **Priority:** P1 · **Type:** ci
**Depends on:** none. **Read first:** `.github/workflows/ci.yml` (jobs `go-integration` and `e2e`), `docs/e2e.md`, README (CI section), CLAUDE.md "CI scope".

#### Why
Full CI takes 8-10 minutes. `go-integration` (`make test-integration`, race detector, real MySQL/Valkey/SeaweedFS) takes 8.4-10 min and `e2e` (journey plus print-check in Chrome) takes about 6.2 min. On 2026-10-26 the owner decided (D-37) to stop running both on every push and pull request. The jobs stay in the file so they can be switched back on or started by hand.

#### Requirements (SHALL)
1. Add `workflow_dispatch:` to the `on:` block of `ci.yml`.
2. Give the jobs `go-integration` and `e2e` the job-level condition `if: ${{ github.event_name == 'workflow_dispatch' }}`. Do **not** delete or rename the jobs: `go-integration` is a required status check on `develop`, and a job skipped by `if:` reports as skipped, which GitHub counts as passing, so pull requests stay mergeable with no branch-protection change. (`e2e` is not required.)
3. Nothing else is switched off: `go` (lint, unit tests with `-race`), `web`, `security` (govulncheck, npm audit) and `image` (build, smoke, Trivy) keep running on every push and pull request.
4. Keep every step, script, Makefile target (`make test-integration`, the e2e and print-check commands) and the e2e failure-artifact upload as they are, so `gh workflow run ci.yml` runs the full set and re-enabling is deleting the two `if:` lines. Add a one-line comment above each `if:` pointing at D-37.
5. Docs: `docs/e2e.md` and the README CI section say that the two jobs run by hand only (how: `gh workflow run ci.yml --ref <branch>`), and that QA runs the same commands locally (see CLAUDE.md "CI scope").
6. No change under `.team/`; no change to test code.

#### Acceptance criteria
- [ ] AC1 — On the task's own PR, `go-integration` and `e2e` show as **skipped** and the other jobs run and pass; the PR is mergeable under the existing required checks (`go`, `go-integration`, `web`, `security`).
- [ ] AC2 — `gh workflow run ci.yml --ref <task branch>` starts a run in which `go-integration` and `e2e` do run and pass (link the run in the PR).
- [ ] AC3 — `actionlint` (or the repo's workflow lint, if any) is clean; the rest of `ci.yml` is unchanged (diff limited to `on:`, two `if:` lines and their comments).
- [ ] AC4 — Docs updated as in requirement 5.

#### Test plan
QA checks the PR's check list (two skipped, rest green), triggers the manual run once and confirms both paused jobs pass, and confirms `make test-integration` still passes locally.
