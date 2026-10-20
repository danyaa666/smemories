# E12_T-106 — Auth and repository hygiene from audit run 1

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P3 · **Type:** security
**Source:** audit run 1, [docs/security-audit-skill-2026-10-20/](../../../docs/security-audit-skill-2026-10-20/) (PR #86; until it merges read it on branch `docs/security-audit-skill-run-1`), `HARDENING-NOTES.md` (hunter observations: confirm each with a test before changing code; if one turns out wrong, say so in the PR and skip it).

#### Requirements (SHALL)
1. **Cookie prefix.** The session cookie becomes `__Host-smem_session` outside `dev` (`Secure`, `Path=/`, no `Domain`); `dev` keeps `smem_session` because it is not `Secure`. Pre-production: everyone signs in once. Update the web if it reads the name anywhere (it should not), the e2e and Postman.
2. **OIDC.** Pin the ID-token algorithm (`SupportedSigningAlgs: []string{"RS256"}`); refuse to link a second Google subject to a user that already has one (`ERROR_FORBIDDEN` redirect to the web error page), with a test.
3. **Owner name shown to link holders.** `public-collect/get` returns the owner's display name; when that name was derived from the email address (if any path does that), return the yearbook title only. Confirm which paths set `display_name` first.
4. **Repository.** `.gitignore` adds `*.tfstate`, `*.tfstate.*`, `*.tfvars`, `*.p12`, `*.pem`; `.github/dependabot.yml` covers `tools/e2e` and `tools/print-check`; `tools/e2e/guard.mjs` also refuses a DSN whose host is not loopback or a private address (with a test).

#### Acceptance criteria
- [ ] AC1 — Tests for each item (cookie attributes per env, RS256 pin, second-subject refusal, guard DSN host).
- [ ] AC2 — `make lint test test-integration`, `make e2e` and Postman green; docs (`docs/redis.md` cookie name, `docs/auth-otp.md` if affected) updated.

#### Risk
`high`: session cookie and Google sign-in.
