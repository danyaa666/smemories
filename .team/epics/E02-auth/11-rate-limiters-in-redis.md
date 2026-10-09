# E02_T-053 — Rate limiters move to Redis (shared limiter)

**Epic:** E02-auth · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P1 · **Type:** tech-debt

#### Description
Every limiter today is an in-memory sliding window per process (`internal/ratelimit`, with a `ponytail` note: with more than one API task the effective limit multiplies, and a restart resets all counters). All of them are time-limited data and move to Redis (D-23), which also makes them correct behind several tasks at go-live. This covers the T-031 "shared limiter" item.

#### Scope
- In: a Redis-backed limiter with the same semantics (`Take`, `Refund`, retry-after) behind an interface; switching every current user of `internal/ratelimit`; tests; docs.
- Out (do not do): new limits, changing any limit value, edge or WAF limits (T-031), per-user quotas stored in MySQL (media quota), the OIDC cookie state.

#### Acceptance criteria
- [ ] AC1 — `ratelimit.Limiter` becomes an interface (`Take(ctx, key) (ok bool, retryAfter time.Duration, err error)` and `Refund(ctx, key) error`) with a Redis implementation: one sorted set per key (`smem:<env>:rl:<name>:<key>`), a Lua script that removes entries older than the window, counts, and adds the hit atomically, using the Redis `TIME` so several API tasks agree; the key expires with the window. `Refund` removes the most recent hit atomically. The in-memory implementation stays for unit tests only (clearly named, not used by `main.go`).
- [ ] AC2 — Every current limiter uses it with unchanged values: register, login pair and IP, forgot-password (IP and email), verify resend and verify attempts, Google start and callback, collection lookups, public note submission (IP, day, collection), media uploads. A table in `docs/redis.md` lists each limiter name, limit, window and fail policy.
- [ ] AC3 — Failure policy: when Redis is unavailable, limiters fail open (the request proceeds, one ERROR log per interval, a counter in the existing log fields) **except** the OTP attempt and lockout limits of T-048 and the login lockouts, which fail closed (`503 limiter_unavailable`) because they protect against guessing. State this in the code comment of each call site.
- [ ] AC4 — Keys use only hashes or numeric ids: emails are SHA-256 hashed (case-normalised as today) before being put in a key; IPs are stored as is. A test proves no email address appears in any key.
- [ ] AC5 — Correctness tests on real Redis with a tiny window: limit exactly N then refuse, retry-after is accurate, refund restores one slot, 50 parallel `Take` calls on one key admit exactly N, two limiter instances (two "tasks") share the count, entries expire (memory returns to baseline).
- [ ] AC6 — Behavioural tests of the existing endpoints (register 6th attempt 429, login lockout, forgot-password limits, notes 40 students from one IP) pass unchanged; the Postman collections run twice; their documented "restart the API between runs" workaround is replaced by flushing the limiter keys (a `make reset-limits` target) and the docs say so.

#### Design
Files: `internal/ratelimit/*` (interface, redis implementation, memory implementation for tests), all call sites in `internal/auth`, `internal/notes`, `internal/media`, `cmd/smemories-api/main.go`, `Makefile`, docs, tests.

#### Risk
`high`: rate limiting protects authentication and public endpoints.

#### Security & performance notes
One script call per limited request (sub-millisecond locally). Hot keys (one campus IP) are one sorted set with at most the limit entries; the script trims it on each call, so memory stays bounded.

#### Test plan
- Dev: as AC5 and AC6; benchmark 1000 sequential `Take` calls.
- QA should probe: clock skew between two API processes (use `TIME`), Redis restart in the middle of a window (AOF keeps counts; flush resets them), 200 parallel registrations from one IP, the fail-closed paths with Redis stopped, key inspection for personal data.
