# E10_T-085 — Lint exclusions anchored and a guard for auth.UserFrom

**Epic:** E10-skills-alignment · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt
**Depends on:** T-072 (merged). Both items come from the T-072 QA note.

#### Description
1. `.golangci.yml`: the forbidigo and mnd exclusion regexes exempt any package whose directory name merely ends in `test` (for example `internal/latest`). Anchor them to the real helper packages: `dbtest`, `redistest`, `storagetest`, `oidctest` (and `internal/media/image.go` stays for mnd). Prove it with a scratch package named like `internal/latest` that must now be flagged.
2. `auth.UserFrom` returns `(User, bool)` and handlers use `u, _ := auth.UserFrom(...)`: on a route mounted without `RequireUser` that would hand out a zero/nil user. No route does that today. Add a small guard so a missing user answers `ERROR_UNAUTHORIZED` instead of continuing (for example a `MustUser(w, r)` helper used by the handlers, or a check in the shared handler wrapper) and a test that mounts a route without `RequireUser` and expects 401.

#### Acceptance criteria
- [ ] AC1 - the scratch package `internal/latest` with a `fmt.Errorf` and a magic number is flagged by `make lint`; the real helper packages stay exempt; no new `//nolint`.
- [ ] AC2 - every handler that reads the signed-in user goes through the guard; a route without `RequireUser` answers 401 in a test.
- [ ] AC3 - `make lint build test` and the integration tests are green; no behaviour change on the existing routes.
