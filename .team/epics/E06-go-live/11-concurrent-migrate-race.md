# E06_T-108 — Concurrent `smemories-migrate up` on an empty database fails with "missing zero version migration"

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** bug
**Read first:** `internal/db/migrate.go`, T-088 spec (`08-migrate-lock-schema-check.md`), `docs/deploy.md`, `scripts/image-smoke.sh` (the "concurrent smemories-migrate up" check).

#### Description
The CI `image` job flakes: in `make image-smoke`, two `smemories-migrate up` processes started together on an empty database sometimes end with `migrate up failed: missing zero version migration` (`SMOKE FAIL: concurrent smemories-migrate up (2) failed`). QA of T-092 saw it on develop runs (39198267128 and four on 2026-10-17) and locally about 1 in 17 runs on a loaded machine. The T-088 advisory lock should make the second process wait; the error suggests goose creates or reads its version table **before** the lock is held (one process creates `goose_db_version`, the other reads it before the version-0 row exists).

#### Requirements (SHALL)
1. Reproduce deterministically in an integration test (two migrators on an empty database, the first paused right after creating the version table, through a test seam), and show it failing on today's code.
2. Fix the root cause: the lock is taken before any goose call that touches the version table (including its creation), on the same connection that runs the migrations, and released after; the second process then sees a complete table and has nothing to do (exit 0). No retries or sleeps as the fix.
3. The smoke check stays as it is and passes 50 runs in a row locally (state the loop in the PR).
4. `docs/deploy.md` (concurrency guarantee) updated if the wording changes.

#### Acceptance criteria
- [ ] AC1 — The race test fails before and passes after; `go test -race -count=20` on it is green.
- [ ] AC2 — `make image image-smoke` 50 times in a loop with no failure; CI `image` green on the PR.
- [ ] AC3 — The T-088 behaviours (lock timeout exit 3, prod refuses `down`, schema check) unchanged; their tests pass.

#### Risk
`high`: migrations in production; the owner approves the merge.
