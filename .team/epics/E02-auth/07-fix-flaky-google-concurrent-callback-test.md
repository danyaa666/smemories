# T-047 — Fix the flaky concurrent Google callback (retry with backoff)

**Epic:** E02-auth · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P1 · **Type:** bug

#### Description
CI on `develop` failed once (run 37755309890, merge commit of T-038, `go-integration`): `TestGoogleConcurrentCallbacksCreateOneAccount` got `302 /login?error=oidc_failed` for one of six parallel callbacks. Root cause: `Store.googleUser` (`internal/auth/store.go`) retries a deadlocked or duplicate-key transaction at most 3 times with no pause, so on a loaded CI runner six transactions that lock the same missing row (`SELECT … FOR UPDATE` on a non-existent email takes gap locks) keep colliding and the losers run out of attempts. In production the same race means a student signing in twice quickly can see a failed sign-in. QA passed the task because the race did not show locally; the flake is real.

#### Scope
- In: a bounded retry with jittered backoff for the Google sign-in transaction; the retryable MySQL errors; a stress test; the same helper reused if another auth transaction has the same pattern (check, do not widen).
- Out (do not do): changing the linking rules, the pre-hijacking defence, the lock strategy to named locks, or any API or schema change.

#### Acceptance criteria
- [ ] AC1 — `googleUser` makes up to 6 attempts. Between attempts it waits a random time in a growing window (about 5 to 25 ms, then 10 to 50, doubling, capped at 200 ms), stops at once when the request context ends, and returns the last error after the final attempt. The wait helper takes a clock or sleeper so tests need no real sleeping.
- [ ] AC2 — Retryable errors: MySQL 1213 (deadlock), 1205 (lock wait timeout) and 1062 (duplicate entry, the other writer won), as today plus 1205. A test table covers each number and a non-retryable error (returned at once, no second attempt).
- [ ] AC3 — A stress test runs the six-callback scenario and the four-callback link scenario at least 30 times in a loop under `-race` (a `-count` flag or an internal loop of fresh emails; the whole thing stays under 30 s) and must pass every time on a machine limited to 2 CPUs (`GOMAXPROCS=2`). The dev runs it 20 times in a row locally and states the result.
- [ ] AC4 — The existing Google integration tests, the auth Postman collection (twice) and `make lint build test test-integration` pass; no change to responses or error codes.

#### Design
Files: `internal/auth/store.go` (retry loop and helper), `internal/auth/google_integration_test.go` (stress loop), unit test for the backoff helper.
```mermaid
sequenceDiagram
    participant C as 6 callbacks
    participant S as Store.googleUser
    participant D as MySQL
    C->>S: googleUserTx (parallel)
    S->>D: SELECT … FOR UPDATE (gap lock), INSERT user and identity
    D-->>S: one wins; others 1213 or 1062
    S->>S: wait random 5-25 ms (doubling), retry up to 6 times
    S->>D: retry finds the winner's rows (known identity)
    S-->>C: all 302 to /
```

#### Risk
`high`: authentication code path (sign-in transaction). The change is small and behaviour-preserving apart from more patient retries.

#### Security & performance notes
Retries are bounded and context-aware, so a request cannot hang; the existing per-IP rate limit still applies. Do not log tokens or codes while retrying.

#### Test plan
- Dev: as AC3; unit test for the helper with a fake sleeper (window growth, cap, context cancel).
- QA should probe: 12 parallel callbacks for one new email; a callback with an already-cancelled context; the run time of the loop under `GOMAXPROCS=1`.
