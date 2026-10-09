# E01_T-062 — CI: get the newest Go patch straight from go.dev

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P3 · **Type:** tech-debt

#### Description
On 2026-10-09 Go 1.26.9 was released with fixes for ten standard-library vulnerabilities (GO-2026-6603 to 6617, net/http, net/textproto, crypto/tls). govulncheck (blocking by L-10) failed the `security` job on `develop` because `actions/setup-go` with `check-latest` resolved 1.26.8: GitHub's Go manifest lags a new release by hours. A red `security` check also blocks merging pull requests (D-14). This task makes CI independent of that lag.

#### Scope
- In: the `go`, `go-integration` and `security` jobs of `.github/workflows/ci.yml`, docs/ci.md.
- Out (do not do): changing the `go` directive of go.mod, adding a toolchain line to go.mod, disabling or weakening govulncheck.

#### Acceptance criteria
- [ ] AC1 — A small step (shared through a composite action or repeated with the same script) reads the newest patch of the go.mod minor version from `https://go.dev/dl/?mode=json`, verifies the result looks like `go1.26.N`, and sets `GOTOOLCHAIN=go1.26.N` for the job so the Go command downloads and verifies that toolchain itself (checksum database); if go.dev is unreachable it falls back to the `setup-go` result and prints a warning.
- [ ] AC2 — The job log prints the Go version used; the three jobs use the same version in one run.
- [ ] AC3 — `docs/ci.md` explains the mechanism and what to do when it fails; proven by a run on a pull request that shows the resolved patch.
- [ ] AC4 — No third-party action is added; any new action is pinned to a full commit SHA.

#### Design
`go env GOTOOLCHAIN` semantics: `GOTOOLCHAIN=go1.26.9` makes `go` fetch that toolchain from the module proxy when the local one is older; CI jobs currently set `GOTOOLCHAIN: local` in places, adjust accordingly.

#### Risk
`high`: CI workflow change. The owner approves the merge.

#### Security & performance notes
The toolchain is verified through the Go checksum database; a download adds a few seconds per job.

#### Test plan
- Dev: a pull request run showing the resolved version in all three jobs.
- QA should probe: go.dev unreachable (simulate with a bad URL: warning, fallback), a malformed JSON answer, the cache keys of setup-go still working.
