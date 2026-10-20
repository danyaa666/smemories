# E12_T-101 — Small hardening: readyz cache, generic code-email greeting, absolute session lifetime

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P3 · **Type:** security
**Read first:** [docs/security-audit-2026-10-20.md](../../../docs/security-audit-2026-10-20.md) (findings L1, L2, L3), `internal/httpx/ready.go`, `internal/auth/mail.go` and `emails/*.tmpl`, `internal/auth/service.go` (`Authenticate`), `internal/auth/sessions.go`, `docs/redis.md`, `docs/email.md`.

Three independent changes in three files; one PR, three commits.

#### Requirements (SHALL)
1. **`/readyz` cache (L1).** The probe result is cached for 2 seconds and concurrent probes share one check (singleflight or a mutex with a timestamp). A cached result never hides the drain: the `Draining` dependency is evaluated on every request, before the cache. The log line on failure stays one per check, not per request. Body, status codes and `ReadyTimeout` unchanged.
2. **Generic greeting (L2).** The verification and reset emails no longer contain the user's display name before the address is proven: the `hello` part becomes `Hello,` / `Xin chào,` in both locales. The text and HTML bodies stay built from the same sentences. `renderEmail`'s signature may drop the name; update `docs/email.md` and the golden or snapshot tests.
3. **Absolute session lifetime (L3).** A session has a ceiling of `SessionMaxAge` = 90 days from the `created_at` that `createScript` already stores. `Lookup` returns it; `Authenticate` answers `ErrUnauthenticated` once `created_at + SessionMaxAge` has passed; a sliding refresh never sets the expiry past the ceiling (Redis `PEXPIRE` and the cookie `Expires` both use `min(SessionTTL, ceiling - now)`). Constant, not a variable. `docs/redis.md` and the auth doc state the two numbers (30 days sliding, 90 days absolute).
4. **Out of scope (L-15).** "Sign out everywhere" and a session list: new API and screen; the `DeleteAll` script already exists for when the Account page needs it. The per-user session cap stays in T-031.

#### Acceptance criteria
- [ ] AC1 — Test with a fake pinger: 100 probes in 1 s call the database ping once; after 2 s a new check runs; a failing dependency is cached as failing for 2 s only; cancelling the context (drain) turns the next probe to 503 immediately even inside the cache window.
- [ ] AC2 — Email tests: no rendered subject, text or HTML contains the display name (use a name like `Click evil.example`), in en and vi, verify and reset; the code and the HTML escaping are unchanged.
- [ ] AC3 — Session tests with the injected clock: a session used every day is valid on day 89 and refused on day 91; the refresh near the ceiling shortens the cookie `Expires` to the ceiling; a new login after the ceiling works; sessions younger than the ceiling behave as before (existing tests unchanged).
- [ ] AC4 — `make build test lint test-integration` and CI green; docs updated.

#### Risk
`high` (sessions). A user active for 90 days signs in again once. Nothing else changes for users.

#### Test plan
QA probes: hammer `/readyz` while stopping Redis (503 within about 2 s) and during shutdown (503 at once); register with a hostile display name and read the mail; a session at day 89, 90 and 91 with the clock moved; cookie flags and `Expires` after a near-ceiling refresh.
