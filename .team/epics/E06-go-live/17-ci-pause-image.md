# E06_T-137 — CI: pause the image job (owner decision D-38)

**Epic:** E06-go-live · **Milestone:** M2 · **Risk:** low · **Priority:** P1 · **Type:** ci
**Depends on:** none. **Read first:** `.github/workflows/ci.yml` (job `image`, and how T-132 paused `go-integration` and `e2e`), `docs/ci.md`, README CI section, CLAUDE.md "CI scope", decision D-38 in .team/README.md.

#### Why
On 2026-10-29 the `image` job went red on every branch because Trivy learned a new HIGH CVE (T-136 fixed it). The owner decided (D-38) to stop running the job on every push and pull request, as was done for `go-integration` and `e2e` (D-37). The job stays in the file so it can be started by hand.

#### Requirements (SHALL)
1. Give the job `image` the job-level condition `if: ${{ github.event_name == 'workflow_dispatch' }}` with a one-line comment pointing at D-38 (same form as T-132). Do not delete or rename the job, any step, or the Makefile targets `image-smoke`, `image-scan`, `image-sbom`. `workflow_dispatch` is already in the `on:` block.
2. Nothing else is switched off: `go`, `web`, `security` and the two jobs paused by D-37 keep their current conditions. `image` is not a required status check on `develop`; confirm with `gh api repos/danyaa666/smemories/branches/develop/protection/required_status_checks --jq .contexts` (expected `go`, `go-integration`, `web`, `security`) and say so in the PR. If `image` IS required, stop and report.
3. Docs: README CI section, `docs/ci.md` and `docs/e2e.md` (wherever the three paused jobs are listed) say that `image` runs by hand only (`gh workflow run ci.yml --ref <branch>`), that dev and QA run `make image-smoke` and `make image-scan` locally when they change the Dockerfile, `go.mod`/`go.sum` or anything the image copies, and that the deploy pipeline (T-023) must build, smoke-test and scan the image itself.
4. The first line of `ci.yml` still lists the jobs; make it match what runs on PRs now (fixes the cosmetic note from T-132 QA).
5. **Doc-only pushes:** add `paths-ignore` to the `push` trigger only (`.team/**`, `docs/**`, `**/*.md`, `CLAUDE.md`), so a push to `develop` or `main` that changes nothing but those paths starts no run (owner request 2026-10-29). Do NOT add it to `pull_request`: a path filter there would leave the required checks pending and block doc-only PRs. A push that also touches any other path still runs everything that is not paused. Prove it with a doc-only push to your branch (not `develop`) and say what the Actions tab showed.
6. No change under `.team/`; no test or source change.

#### Acceptance criteria
- [ ] AC1 — On the task's own PR, `image`, `go-integration` and `e2e` show as skipped; `go`, `web`, `security` pass; the PR is mergeable under the existing required checks.
- [ ] AC2 — `gh workflow run ci.yml --ref <task branch>` starts a run in which `image` (and the other two) run and pass; link the run in the PR.
- [ ] AC3 — A push that changes only the ignored paths starts no run; a push that changes any other file does; PRs are unaffected.
- [ ] AC4 — The `ci.yml` diff is limited to the `if:` line, its comment and the header comment; docs updated as in requirement 3.

#### Test plan
QA checks the PR's check list, the manual run, the required-checks list, and the diff.
