# Security audit, 2026-10-20

Scope: the whole repository at `develop` (Go API, React web app, Dockerfile, compose, CI, migrations, repo and GitHub settings).
Method: manual read of the security-relevant code, grep sweeps for risky APIs, `govulncheck`, `npm audit`, secret scans of the tree and the git history, and read-only `gh api` checks of the GitHub settings.
Nothing was run against a live stack, so every finding below is **from reading code or config**, not from an exploit. Where a finding depends on a runtime behaviour (MySQL collation), the evidence is the code's own comment.

**Result: no critical or high finding.** The auth, upload and rate-limit code is carefully built. There are 4 medium and 7 low findings, mostly "secure by default" gaps and go-live items that the board does not yet track.

## Findings

| # | Sev | Finding | Where | Fix size |
|---|-----|---------|-------|----------|
| M1 | Medium | Rate limits are keyed by the full client address, so IPv6 rotation walks around all of them; no per-account login cap | `httpx/middleware.go:238`, `auth/service.go:196-216` | S |
| M2 | Medium | `SMEM_ENV` defaults to `dev`: a task that forgets the variable boots in the weakest mode | `config/config.go:116`, `Dockerfile`, `mailer/mailer.go:87` | S |
| M3 | Medium | Object-key spelling is client-chosen, so a user can orphan S3 objects, reset their own quota and defeat "delete my yearbook" | `media/service.go:86-97`, `yearbook/service.go:128` | S |
| M4 | Medium | "Only the owner promotes to `main`" is policy, not enforcement: agents act as the repo admin and admins bypass branch protection | GitHub settings, `.claude/settings.json` | owner decision |
| L1 | Low | `/readyz` is unauthenticated and pings MySQL and Redis on every request | `httpx/ready.go:30-43` | S |
| L2 | Low | The code emails greet with the registrant's display name before the address is verified (phishing text in a genuine email) | `auth/emails/en.tmpl:2,7`, `auth/mail.go:81` | S |
| L3 | Low | Sessions slide forever: no absolute lifetime, no way to list or revoke other sessions | `auth/service.go:288-297` | S |
| L4 | Low | Web edge headers (CSP, HSTS, frame-ancestors) are not specified anywhere yet | `docs/deploy.md`, T-023 | S |
| L5 | Low | The public link token travels in the query string, so ALB/CloudFront access logs will record it | `notes/handler.go:67-68` | S |
| L6 | Low | Dependabot alerts and security updates are disabled; `npm audit` is non-blocking | GitHub settings, `ci.yml` | XS |
| L7 | Low | `dev`/`test` use a public OTP key constant; a staging run as `test` is open to anyone who reads the repo | `config/config.go:82,359` | XS |

### M1. Rate limits can be bypassed from one IPv6 prefix; no per-account cap
- `ResolveClientIP` returns the address string as is. Every limiter (`login_pair`, `login_ip`, `register`, `forgot_ip`, `reset_tries`, `google_*`, `public_*`, `submit_ip_*`) keys on it. A host with an IPv6 /64 has 2^64 addresses, so each per-IP budget is effectively unlimited.
- The login lockout is per **(IP, email)** pair (10 per 15 min). There is no limit per account alone. One attacker rotating addresses can guess passwords for one account at argon2 speed. The only other brake is the password rule (10+ characters, no breached-password check).
- Same root cause gives: mass registration plus mass OTP emails to arbitrary addresses (5/h per IP, so unlimited), which burns the SES sender reputation; and exhaustion of the 4 argon2 slots (`SMEM_AUTH_MAX_CONCURRENT_HASHES`) so real users get 503 `busy`.
- Not covered by T-031 (it lists edge limits and NAT bursts, not IPv6).
- **Fix:** mask IPv6 to /64 inside `ResolveClientIP` (one function, all limiters inherit it); add a generous per-account failed-login limiter (for example 50 per 15 min, fail closed) instead of a hard lockout; WAF rate rule at the edge (T-031).

### M2. `SMEM_ENV` defaults to `dev`
- `config.go:116`: `Env: get("SMEM_ENV", "dev")`. The Dockerfile sets no `ENV`. `docs/production-config.md` says "must be `prod`" but nothing enforces it, and `scripts/image-smoke.sh` tests the refusals only with the variable set.
- In `dev` the process: uses the **public** OTP key constant; sends mail through the **log mailer, which prints every code to stdout** (so into CloudWatch); omits the cookie `Secure` flag (`main.go:135`); does not require TLS to MySQL, Redis or S3; and skips the prod-only config checks.
- **Fix:** make `SMEM_ENV` mandatory (refuse an empty value; `.env.example`, `make up` and compose already set it), or put `ENV SMEM_ENV=prod` in the image so that dev has to opt out. Add the "empty env refuses" case to `image-smoke.sh`.

### M3. Client-chosen object-key spelling
- `public_id` columns are `utf8mb4_0900_ai_ci`, so a lookup by id matches any case spelling (the notes package knows this and rejects non-canonical ids, `notes/moderation.go:123-125`). The yearbook and media handlers do not (`media/handler.go:196-206` checks length only).
- `media.Service.Upload` builds the S3 prefix from the **caller's spelling** (`media/service.go:86,97`). `yearbook.Service.Delete` purges `yearbooks/<caller's spelling>/` (`yearbook/service.go:128`) instead of the stored `y.ID`.
- Attack on oneself with real cost to us: upload photos with a lower-case `yearbook_id`, delete the yearbook with the canonical id. The prefix purge finds nothing, the rows go, the per-user 500 MiB quota (counted from rows) resets, the objects stay in S3. Repeat. Upload is limited to 60 per 10 minutes, so this is slow but unbounded.
- Same flaw breaks erasure: "delete my yearbook" leaves the photos behind (relevant to T-025, user data deletion).
- **Fix:** one shared `ulid.Valid` (26 upper-case Crockford characters) used by every handler that takes a public id, and use the stored `y.ID` for the prefix. Add a regression test with a lower-case id.

### M4. Owner-only promotion to `main` is not enforced
- `gh api` shows `main` and `develop` protected by the 4 required checks, but `enforce_admins=false`, required approvals 0. The agents run with the owner's `gh` login (an admin), and `.claude/settings.json` allows `git push*` (only force-push is denied). An agent can push or merge straight to `main`, bypassing the checks.
- This is a process decision, so it is yours: (a) a separate bot account for dev/qa with write, not admin, and a ruleset on `main` whose bypass list is the owner only; or (b) at minimum `enforce_admins=true` on both branches so the checks cannot be skipped.

### L1. `/readyz`
Each probe takes a connection from the 20-connection MySQL pool and a Redis round trip. The web client's comment (`web/src/api/client.ts`) says the edge forwards `/readyz`, so it is probably reachable by anyone and a flood competes with real traffic for pool slots. Cache the result for 1-2 s (singleflight) and, in T-023, restrict it to the ALB health check.

### L2. Display name in the code email
`Hi {{.Name}},` is rendered before the address is verified. Anyone can register `victim@example.com` with a 100-character display name that reads as a message; the victim gets it in a genuine email from our domain. Use a generic greeting for the verification email (the reset email is only sent for existing accounts, but the name is still user-chosen).

### L3. Sessions
A stolen cookie lives as long as the thief keeps using it (30 days sliding, no ceiling). Add an absolute maximum (for example 90 days) in `Authenticate`, and a "sign out everywhere" action (the `DeleteAll` script already exists). T-031 already covers the per-user session cap.

### L4 and L5. Edge items for T-023
- The API sets `nosniff`, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`, `Cache-Control: no-store`. The SPA host needs: `Content-Security-Policy` (`default-src 'self'; img-src 'self' blob: data:; frame-ancestors 'none'; base-uri 'none'`), `Strict-Transport-Security`, `Referrer-Policy`, `X-Content-Type-Options`. React escapes output and the code has no `dangerouslySetInnerHTML`, `innerHTML` or `eval` (grep), so this is defence in depth for friends' untrusted text.
- `GET /api/public-collect/get?token=…` and `POST …/submit-note?token=…` put the bearer token in the query. The app never logs it, but ALB access logs and CloudFront standard logs do. Turn those logs off, drop the query string from them, or move the token to a header.

### L6. Dependency hygiene
Dependabot alerts and security updates are off (the weekly version-update PRs from `dependabot.yml` still work). Enable both (free for public repos). CI already fails on a reachable Go vulnerability and warns on npm advisories.

### L7. `test` is a local-only environment
With `SMEM_ENV=test` and no `SMEM_OTP_KEY`, codes and page cursors are keyed with the constant in the public repo; with `SMEM_DEV_FIXED_OTP` set, anyone who knows an email address can verify it and reset its password. Document that `dev`/`test` are never deployed, and have the image-hygiene or deploy check refuse a non-loopback `SMEM_HTTP_ADDR` or ALB target with those values (T-050 covers the shortcut, not the key).

## Checked and sound

- **Passwords:** argon2id (19 MiB, t=2, p=1, the OWASP minimum; raise if memory allows), NFKC normalisation, constant-time compare, equal work for unknown email and Google-only accounts, bounded stored-hash parameters, bounded concurrency.
- **Email codes:** 6 digits from `crypto/rand`, stored only as HMAC, check and attempt count in one Lua script (5 tries per code, 3 issues per hour per address), single use, reset signs out all sessions before the commit. Residual guess odds are about 1.5e-5 per hour per target.
- **Sessions and CSRF:** 256-bit random token, only its SHA-256 reaches Redis, `HttpOnly`, `SameSite=Lax`, `Secure` outside dev, old cookie revoked at login, Origin/Referer check on every cookie-carrying POST/PUT/PATCH/DELETE, JSON-only bodies, no CORS.
- **Google sign-in:** state, nonce and PKCE bound to a signed cookie, `email_verified` must be a real boolean `true`, pre-hijacking defence for unverified local accounts, `return_to` limited to a same-site path.
- **Injection:** every SQL statement binds its values; the dynamic ones concatenate constants and `?` lists only. Unknown JSON fields and trailing data are rejected.
- **Authorisation:** every media, yearbook, notes and book-data query is scoped by `owner_id`; foreign ids answer not found; public ids and page cursors are AES-GCM sealed and scope-bound.
- **Uploads:** magic-byte sniff, header-size and decoded-memory limits before decode, re-encode strips EXIF, client file name and type never used, server-made keys, per-IP and global slot caps, disk spool with pace limits, honeypot.
- **Supply chain and infra:** actions pinned by SHA, least-privilege `permissions`, base images pinned by digest, distroless non-root, Trivy and SBOM, `govulncheck` (0 reachable), `npm audit --omit=dev` (0), prod config contract (TLS to DB/Redis/S3, https origins, private metrics bind).
- **Secrets:** no secret patterns in the tree or the git history; `.env` is ignored and was never committed; GitHub secret scanning and push protection are on.
- Note: the local Go toolchain is 1.26.1, which `govulncheck -show verbose` lists with stdlib advisories fixed in 1.26.6-1.26.9 (not reachable from this code). The image pins 1.26.9; do not ship a binary built on this machine.

## Not covered

Read only in part or not at all: `notes/moderation_store.go`, `notes/store.go`, `yearbook/validate.go`, `templates/validate.go`, `internal/pdf`, `mailer/ses.go`, `redis/`, `db/`, `metrics/`, the `postman/` and `tools/` scripts, and most of the web pages. No dynamic testing, no infrastructure review (T-023 has no code yet), no load test of the limits.

## Suggested board tasks

1. M1: IPv6 /64 keying plus per-account login limiter (security, P2).
2. M2 and L7: mandatory `SMEM_ENV`, smoke-test case, deploy guard for dev/test values (security, P2).
3. M3: canonical public-id validation and `y.ID` prefix, with regression tests (security, P2, before T-025).
4. L1, L2, L3: readiness cache, generic greeting, absolute session lifetime (hardening, P3).
5. L4, L5, L6: add to the T-023 checklist (edge headers, log hygiene) and switch on the Dependabot settings.
6. M4: owner decision on agent identity and `enforce_admins`.
