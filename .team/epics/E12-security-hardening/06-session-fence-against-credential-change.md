# E12_T-103 — Sessions issued during a password reset or Google merge must not survive it

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** security
**Source:** audit run 1, [docs/security-audit-skill-2026-10-20/](../../../docs/security-audit-skill-2026-10-20/) (PR #86; until it merges read it on branch `docs/security-audit-skill-run-1`): lead 2 `auth/session-create-not-fenced-against-credential-purge` (needs validation; the full trace and the deterministic test plan are in `NEEDS-VALIDATION.md` section 2). **Read first:** `internal/auth/service.go` (`Login`, `issue`, `Authenticate`), `internal/auth/reset.go`, `internal/auth/store.go` (`googleUserTx`), `internal/auth/sessions.go`, `docs/redis.md`, `docs/db-conventions.md`.

#### Description
Login reads the password hash without a lock, may wait up to 2 s for a hashing slot, then creates a session unconditionally. Password reset and the Google pre-hijack merge purge the user's sessions **before** their transaction commits. A login that read the old hash (an attacker who knows the old password and logs in in a loop) can create a session after the purge that survives the reset for 30 days. A reset is exactly the moment the owner wants every old session gone.

#### Requirements (SHALL)
1. **Prove it first.** Add the deterministic integration test from the audit plan (one hashing slot held, Login started, reset or Google merge run, slot released, `get-me` with the new cookie) for both variants plus a control. It must fail on today's code; say in the PR that it did.
2. **Credential epoch.** One expand-only migration adds `auth_epoch INT UNSIGNED NOT NULL DEFAULT 0` to the user table (D-26 conventions). The reset transaction and the Google merge transaction increment it. Login reads it in the same SELECT as the hash; the session stores it (`epoch` field in the session hash); `Authenticate`, which already reads the user row on every request, refuses a session whose epoch differs (`ErrUnauthenticated`, same 401 as an expired session, and deletes it).
3. **Order.** Keep the purge, but run it **after** the commit (with the epoch it is a clean-up, not the guarantee); a purge failure after commit is logged once and does not fail the reset, because the epoch already revokes everything.
4. **Existing sessions** (no `epoch` field) are treated as epoch 0 so nobody is signed out by the deploy.
5. **Docs.** `docs/redis.md` (session fields), `docs/auth-otp.md` (reset revokes sessions by epoch).

#### Acceptance criteria
- [ ] AC1 — The two race tests fail before the change and pass after; the control passes in both.
- [ ] AC2 — Reset and Google merge: every session issued before the commit, including one created during the window, answers 401 at the next request; the new login after the reset works.
- [ ] AC3 — No extra query per request (the epoch comes with the user row `Authenticate` already reads); query-count test.
- [ ] AC4 — Migration applies on a database with users; `make lint test test-integration` green.

#### Risk
`high`: sessions and authentication; the owner approves the merge.
