# E01_T-190 — internal/notes: two integration tests fail on develop

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M4 · **Risk:** medium · **Priority:** P2 · **Type:** bug
**Depends on:** none. Found by QA on T-186 (2026-11-19, develop 1579d4e, Windows PC).

#### Description
`make test-integration` fails `internal/notes` `TestConnectionCapAndFlatMemory` and `TestSweepSpoolAtStartup` on develop; every other package passes.
Not known yet whether this is Windows-only (spool paths, file locking, memory measurement) or a real regression.

#### Requirements (SHALL)
1. Find the cause of each failure, with evidence: the failing assertion and output, and whether it also fails on Linux (ubuntu container or manual CI
   `go-integration` run).
2. A product bug (spool sweep, connection cap, memory bound) SHALL be fixed in the product with a test that catches it. A platform-only test artifact
   SHALL be fixed in the test without weakening what it checks on Linux; a skip on Windows needs a reason in the code and in the PR.
3. `make test-integration` green for `internal/notes` on the Windows PC and in a manual CI run (run id in the PR).

#### Test plan
- Before/after output of `go test -tags integration ./internal/notes/...` on Windows and Linux; manual CI run.
