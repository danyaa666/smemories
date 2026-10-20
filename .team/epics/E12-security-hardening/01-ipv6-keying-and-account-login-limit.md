# E12_T-098 — IPv6 /64 keying and a per-account login limiter

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** security
**Read first:** [docs/security-audit-2026-10-20.md](../../../docs/security-audit-2026-10-20.md) (finding M1), `docs/redis.md` (limiter fail policy, D-23), `docs/production-config.md`, `internal/httpx/middleware.go`, `internal/auth/service.go`, `.team/epics/E02-auth/05-auth-hardening-for-go-live-edge-rate-limits.md` (T-031, the NAT concern).

#### Description
Every per-client limit (`login_pair`, `login_ip`, `register`, `forgot_ip`, `reset_tries`, `google_*`, `public_*`, `submit_ip_*`, `media_upload` is per user and unaffected, `print_feedback_ip`, the public upload slots) keys on the address string from `httpx.ResolveClientIP`. One IPv6 host owns a /64, 2^64 addresses, so each budget is unlimited for it. Login also has no limit per account alone: only per (IP, email). Close both.

#### Requirements (SHALL)
1. **/64 keying.** `ResolveClientIP` returns, for an IPv6 address, the first 64 bits in canonical text form (for example `2001:db8:1:2::`; zone id dropped). An IPv4-mapped IPv6 address (`::ffff:a.b.c.d`) returns the IPv4 text. An IPv4 address is unchanged. An unparsable `RemoteAddr` keeps today's fallback. The same function serves `WithClientIP`, `ClientIP` and `auth.Handler.ClientIP`, so every limiter and the upload slot map inherit it with no other change. The client address is never stored or logged (grep before you start: it must stay that way), so nothing needs the full address.
1b. **Canonical text and proxy warning (audit run 1).** Every returned address is canonicalised (`net.ParseIP(...).String()` before masking), so two spellings of one address share a key. When `SMEM_TRUST_PROXY=true` and the direct peer of a request is not a private or loopback address, log one WARN per minute (a deployment that trusts a header it should not).
2. **Per-account failed-login limiter.** New limiter `login_account`, key `hashKey(normalised email)` with the same oversize guard as `pairKey`, default `SMEM_RATE_LOGIN_FAILS_PER_ACCOUNT=50` per 15 minutes, **closed** (503 `limiter_unavailable` when Redis is down), counted up front and refunded on success like the other two. It is taken after `login_pair` and `login_ip`; a refusal at any step refunds the earlier ones (follow the existing code). Answer is the same `ERROR_RATE_LIMITED` with `Retry-After`.
3. **Config and docs.** The variable is read in `internal/config` (minimum 1), documented in `docs/production-config.md` (`TestEveryVariableIsDocumented` fails otherwise), `.env.example` and the README variable list if there is one. `docs/redis.md` lists the new limiter name and policy.
4. **A successful password reset clears the account counter (leader addition 2026-10-20).** Without this, an attacker who keeps the account at 50 failures can keep the real owner out even after the owner proved control of the email: the reset works, the next login does not. After `reset-password` succeeds, delete the `login_account` entry for that email (the limiter package may need a small `Reset(key)`; the key is the same `hashKey(normalised email)`). Google sign-in and reset stay outside this limiter either way.
5. **Accepted trade-off (L-15).** An attacker who fails 50 times locks that account's *login* for up to 15 minutes. Password reset and Google sign-in are not affected. Say so in the `docs/auth-otp.md` or the auth doc section on limits.

#### Acceptance criteria
- [ ] AC1 — Table test of `ResolveClientIP`: IPv4; IPv4-mapped IPv6; two IPv6 addresses in one /64 give one key; two /64s give two; a zone id; `[::1]`; an `X-Forwarded-For` last hop that is IPv6 with `trustProxy`; a garbage hop falls back to RemoteAddr.
- [ ] AC2 — Integration (real Redis): with `SMEM_TRUST_PROXY=true` semantics (or a test seam that sets the IP), 51 failed logins for one email from 51 different /64s answer 429 on the 51st; 50 failures plus one success never lock; a different email is unaffected; `login_pair`/`login_ip` behaviour is unchanged.
- [ ] AC3 — Fail-closed: with Redis down the login answers 503 `limiter_unavailable` at the new limiter too.
- [ ] AC4 — Existing limiter tests and Postman (`reset-limits`) still pass; `make test-integration`, lint and CI green.
- [ ] AC5 — Config test: default 50, invalid and zero values name the variable.
- [ ] AC6 — After 50 failures lock an account, a successful `reset-password` for that email lets the next correct login through at once; another account's counter is untouched.

#### Known residual (leader note)
/64 keying stops a single-host rotation but not an attacker with a delegated /56 or /48 (256 or 65,536 buckets). The per-account limiter bounds password guessing regardless of addresses; mass registration and mail volume across many prefixes is T-031 (WAF rate rule), not this task.

#### Risk
`high` (authentication). /64 grouping can rate-limit two students on one campus prefix; the limits were already shared per NAT address, and T-031 owns the measurement of real class bursts. Nothing here weakens an existing limit.

#### Test plan
QA probes: IPv6 rotation inside one /64 (must share a budget), across /64s (must not), IPv4-mapped spelling of the same host, 50 vs 51 failures, success refund, concurrent attempts (no slip past the limit), Redis stopped mid-test, oversize email (key stays bounded).
