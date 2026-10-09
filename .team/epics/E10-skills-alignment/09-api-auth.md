# T-071 — API v2: auth and user domain; remove the /v1 paths and the web shim

**Epic:** E10-skills-alignment · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high (authentication) · **Priority:** P1 · **Type:** tech-debt
**Read first:** `docs/api-contract.md`, `docs/go-conventions.md`, `docs/auth-otp.md` (from T-048), T-068 (the pattern).
**Depends on:** T-067, T-070, T-053.

#### Description
Last domain. Move auth to `/api/auth/*` and `/api/user/get-me`, adopt `apperr`/envelope, split `auth/handler.go` so it only decodes and maps, and delete everything that existed only for the transition: the `/v1` prefix handling in the dev proxy rewrite, the web shim branch for the old envelope, and the `RequireUser` old-envelope text.

#### Requirements (SHALL)
1. Routes per `docs/api-contract.md` section 8: `register`, `login`, `logout`, `verify-email`, `resend-verification`, `forgot-password`, `reset-password`, `google-start`, `google-callback` under `/api/auth/`, and `GET /api/user/get-me`. (T-048 may have renamed the OTP endpoints; apply the same naming rule to whatever it delivered and record the final names in the contract doc.) No `/v1` route remains.
2. The session cookie name, attributes, CSRF guard, Redis session behaviour, rate limits, argon2id parameters, OTP rules and the dev-shortcut guards SHALL NOT change.
3. Google: `google-start` still redirects (302); `google-callback` keeps its error redirects to the web app; the redirect URI built from `PUBLIC_BASE_URL` becomes `.../api/auth/google-callback` (README and `.env.example` updated). **Owner action:** register the new URI in the Google console; say so in the PR description.
4. `get-me` returns `{id, email, email_verified, display_name, locale, created_at}` with `created_at` Unix ms; the `RequireUser` rejection is `ERROR_UNAUTHORIZED` (401).
5. Error codes: `invalid_credentials` -> `ERROR_INVALID_CREDENTIALS`, `email_taken` -> `ERROR_EMAIL_TAKEN`, `rate_limited` -> `ERROR_RATE_LIMITED` with `Retry-After`, `weak_password`, `invalid_email`, `invalid_display_name`, `invalid_token`/`invalid_code` ... all in `x-error-codes` per endpoint; anti-enumeration behaviour (same answer for unknown email on login/forgot) unchanged and stated in each description.

#### Acceptance criteria
- [ ] AC1 — Contract lint passes for all auth operations; every `/api/` operation in the file now passes; the lint test runs without a skip list.
- [ ] AC2 — Existing auth integration tests (register through reset, Google incl. the concurrency stress, Redis session tests) pass with only path/envelope/code/time changes (listed in the PR).
- [ ] AC3 — Web: all auth pages and the session bootstrap (`get-me`) use new paths; `client.ts` loses the old-envelope branch and the `/v1` rewrite is removed from `vite.config.ts`; `normaliseCode` stays (locale keys unchanged); vitest + the i18n parity check green.
- [ ] AC4 — No `/v1` route is registered any more: a test asserts `GET /v1/auth/login` and `GET /v1/me` answer 404 (the old envelope is still produced until T-072 removes the switch; assert the status only).
- [ ] AC5 — Postman and README/docs (`docs/auth-otp.md`, `docs/redis.md`, `docs/dev-shortcuts.md` paths) updated; `scripts/check-no-dev-shortcuts.sh` (T-050) unaffected.

#### Risk
`high`: authentication, session and OAuth redirect. The owner approves the merge and does the Google console step.

#### Test plan
- QA should probe: login rate limit headers, logout idempotence, cookie flags, CSRF with wrong Origin on every POST, Google flow end to end with the OIDC test provider, state/PKCE tampering, open redirect attempts on the callback, expired session mid-request, enumeration timing on login/forgot.
