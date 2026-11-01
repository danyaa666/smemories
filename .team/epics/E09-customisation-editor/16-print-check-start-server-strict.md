# E09_T-157 — print-check: refuse a foreign server on its port; refresh the test counts in docs

**Epic:** E09-customisation-editor · **Milestone:** M4 · **Risk:** low · **Priority:** P3 · **Type:** test
**Depends on:** T-154 (merged first). **Read first:** `tools/print-check/` (`startServer`, `lib.mjs`), `docs/e2e.md`, `docs/templates.md`.

#### Why
QA found on T-154 (2026-11-01) that `startServer` in print-check accepts any server already listening on its port. A stale vite from an old worktree sat on :5199, so a default `make print-check` tested the wrong code (5 of 7 layout tests failed until `PRINT_CHECK_PORT` was changed). A check that can silently test the wrong build is worse than no check. Also `docs/e2e.md` and `docs/templates.md` still say 170 print-check tests; there are now 180.

#### Requirements (SHALL)
1. `startServer` must prove the server on its port is the one it needs: either start its own server on a free port it picks (preferred), or, if it reuses a running server, verify it serves the current working tree (for example a build id or a file hash endpoint) and otherwise fail with a clear message naming the port and the process.
2. Never kill a process it did not start.
3. Update the test counts in `docs/e2e.md` and `docs/templates.md` (or remove the hard-coded number in favour of "run `make print-check`").
4. A test (or a documented manual check) shows a foreign server on the default port makes the run fail or be avoided.
5. Nothing under `.team/`; no change to product code.

#### Acceptance criteria
- [ ] AC1 — With a foreign server (any static server) holding the default print-check port, `make print-check` either passes on its own server or fails fast with the clear message; it never tests the foreign server.
- [ ] AC2 — Normal `make print-check` still passes (180 tests or the current number).
- [ ] AC3 — Docs updated as in requirement 3.

#### Test plan
Dev starts a dummy server on the default port and runs print-check; QA repeats.
