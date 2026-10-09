# E06_T-021 — Transactional email provider and domain setup

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** infra

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Replace LogMailer with a real provider (SES is the natural fit on AWS) including SPF/DKIM/DMARC runbook and bounce/complaint handling. Paid service: raise an owner question before specifying.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._

#### Leader notes from the T-007 review (2026-10-08)
When the real mailer replaces the `LogMailer` (`internal/mailer`, chosen in `cmd/smemories-api/main.go`):
- `POST /v1/auth/verify-email/resend` returns 500 `internal_error` when the mailer fails; map a provider outage to `503` with `Retry-After` and a clear code.
- Registration sends the verification email on the request path with a 10 s timeout; move it to the background (as forgot-password already does, `Service.background`) or accept the latency explicitly.
- Emails are plain text only; add an HTML part (`Message.HTML` exists) with the provider's template rules, and set `List-Unsubscribe`-style headers only if the provider requires them.
- Bounces and complaints: stop sending to an address that hard-bounced (provider suppression list).
