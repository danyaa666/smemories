# E06_T-021 — Transactional email provider and domain setup

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** infra

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Replace LogMailer with a real provider (SES is the natural fit on AWS) including SPF/DKIM/DMARC runbook and bounce/complaint handling. Paid service: raise an owner question before specifying.

#### Decision (owner, 2026-10-17, D-29)
Amazon SES through the SESv2 API, sent from a verified subdomain (the domain itself is still to be given by the owner, Q-037). This task builds and tests the mailer **without an AWS account or a domain**: against a local stub of the SESv2 HTTP API. The owner-assisted steps (verify the domain, DNS records, leave the SES sandbox) are written as a runbook; the infrastructure (IAM role, identity, configuration set) is T-023.

#### Description
Replace the log-only mailer with a real one for production, keep the log mailer for dev/test, and make the three places that send mail behave well when the provider is slow, throttled or refuses an address. Verification and reset codes are the only mail (D-22: six-digit codes typed by the student, no links).

#### Requirements (SHALL)
1. **`mailer.NewSES`** implements `mailer.Mailer` with the SESv2 `SendEmail` call (`github.com/aws/aws-sdk-go-v2/service/sesv2`, same vendor/major version as the SDK modules already added in T-086; record nothing new in the decision log beyond that): simple content with a subject, a plain-text body and an HTML body; `FromEmailAddress` from `SMEM_MAIL_FROM` (display name allowed, e.g. `SMemories <no-reply@mail.example.com>`), optional `ConfigurationSetName` from `SMEM_SES_CONFIGURATION_SET`; region from the AWS default configuration (`AWS_REGION`/`SMEM_AWS_REGION`); credentials from the default chain (the task role); the explicit dial/TLS/response-header/operation timeouts and at most 3 attempts used for S3 in T-086.
2. **Selection and config contract.** `SMEM_MAIL_PROVIDER=log|ses` (default `log`). With `SMEM_ENV=prod` the API refuses to start unless the provider is `ses` and `SMEM_MAIL_FROM` is set to a well-formed address on a non-example domain; the existing refusal of the log mailer in prod stays. Add the variables to `internal/config`, `docs/production-config.md` (the doc test enforces it) and the prod-refusal table tests (T-086).
3. **Failure mapping.** Throttling, 5xx and timeouts become `mailer.ErrUnavailable`; "message rejected / address on the suppression list / account sending paused" become `mailer.ErrRejected`. Endpoints that send inline (`POST /api/auth/resend-verification`) answer `503 ERROR_MAIL_UNAVAILABLE` with `Retry-After` for `ErrUnavailable` and `400 ERROR_EMAIL_UNDELIVERABLE` for `ErrRejected` (the address is the caller's own, so this leaks nothing). Registration and forgot-password send in the background (never on the request path; forgot-password still answers the same 200 for every address: anti-enumeration is unchanged); a background failure is retried at most twice with backoff, then logged once (ERROR, with the request id and error class) and counted in a metric `mail_send_failures_total{class}`.
4. **Content.** A small, accessible HTML template per message kind and locale (en, vi): the code in large monospace, its validity, "if you did not ask for this, ignore this email"; plain-text alternative with the same content; no links with tokens, no tracking pixels, no remote images; subject and body localised by the user's locale; Vietnamese strings reviewed by the owner (list them in the PR description).
5. **Privacy in logs.** Never log the recipient address (log a salted short hash if a correlation is needed), the code, or the body; the SES stub test asserts no log line contains them.
6. **Bounces and complaints without a webhook.** Use the SES **account-level suppression list** (reasons BOUNCE and COMPLAINT): SES then rejects sends to those addresses, which the mailer maps to `ErrRejected`. No SNS endpoint is built in this task (a public signed-message endpoint is more attack surface than this volume justifies); reputation alarms (bounce rate, complaint rate) are CloudWatch alarms in T-024. Say so in `docs/email.md`.
7. **Runbook `docs/email.md`** (owner-assisted steps): create the domain identity for the subdomain; Easy DKIM (three CNAME records); custom MAIL FROM domain with its MX and SPF TXT records; a DMARC record starting at `p=none` with a reporting address and the path to `quarantine`; enable the account-level suppression list for BOUNCE and COMPLAINT; create a configuration set with reputation metrics; **ready-to-paste text for the sandbox-exit request** (use case: transactional sign-up/reset codes, expected volume, bounce handling); how to test while in the sandbox (verified recipient addresses only); the least-privilege IAM statement for the task role (`ses:SendEmail` limited by the identity ARN and a `ses:FromAddress` condition) and the environment variables for T-023; how to check deliverability before launch.
8. **Local development unchanged.** `SMEM_MAIL_PROVIDER` defaults to `log`; the dev fixed code (`SMEM_DEV_FIXED_OTP`) and the dev-shortcut guards are untouched.

#### Acceptance criteria
- [ ] AC1 - Unit/integration tests against a local `httptest` stub of the SESv2 API prove the request shape (`FromEmailAddress`, `Destination.ToAddresses`, `Content.Simple.Subject`, `Body.Text`, `Body.Html`, `ConfigurationSetName`), that requests are SigV4-signed with the default chain (use env credentials in the test), and the error mapping of 429/throttling and 5xx to `ErrUnavailable`, of `MessageRejected`/`AccountSuspendedException`/`SendingPausedException` to `ErrRejected`, and that a blackholed endpoint is cut by the timeouts (bounded, attempts <= 3).
- [ ] AC2 - `resend-verification` answers 503 `ERROR_MAIL_UNAVAILABLE` with `Retry-After` and 400 `ERROR_EMAIL_UNDELIVERABLE` as in 3; registration and forgot-password never wait for the provider (a test with a stub that sleeps 5 s shows the HTTP answer returns immediately) and forgot-password answers identically for every address, including a rejected one.
- [ ] AC3 - `SMEM_ENV=prod` refuses to start without `SMEM_MAIL_PROVIDER=ses` and a valid `SMEM_MAIL_FROM` (table-driven test, no secret echoed); `docs/production-config.md` lists the new variables and its test passes; the log mailer still refuses prod.
- [ ] AC4 - The HTML and text bodies exist for both locales and every message kind; a golden-file test pins them; the HTML passes a simple accessibility/size check (under 20 KB, one `<h1>`, `lang` attribute, no external resource), and no body or log line contains the recipient address or a code outside the message itself (assert on captured logs).
- [ ] AC5 - `docs/email.md` contains every runbook item of 7 and the least-privilege IAM statement, and `docs/dev-shortcuts.md` is unchanged.
- [ ] AC6 - `openapi.yaml`, `schema.d.ts`, Postman (the auth collection) and the web error texts (en, vi) know the two new error codes; the web shows a clear message for both on the resend screen (strings listed for the owner).
- [ ] AC7 - `make lint build test`, `go test -race -tags integration ./...`, `make image-smoke` and the e2e job stay green; the image size stays under 60 MB.

#### Out of scope
Marketing or bulk mail, inbound mail, SNS/webhook endpoints, a template editor, other providers, creating any AWS resource (T-023), the CloudWatch alarms (T-024).

#### Risk
`high`: authentication depends on this mail and it is a new outbound integration; the owner approves the merge. No cloud account needed for this task.

#### Test plan
- Dev: stub-based tests (AC1, AC2), config-contract tests (AC3), golden files (AC4); say what you could not check (a real SES send).
- QA should probe: the stub with throttling, 5xx, slow and blackhole behaviours; registration latency with a slow provider; anti-enumeration (same answer and similar timing for known, unknown and rejected addresses on forgot-password); logs for addresses/codes; the Vietnamese email in a real mail client rendering (save the .eml from the stub and open it); the prod start refusals; image size.


#### Leader notes from the T-007 review (2026-10-08)
When the real mailer replaces the `LogMailer` (`internal/mailer`, chosen in `cmd/smemories-api/main.go`):
- `POST /api/auth/resend-verification` (v2 name of the old `/v1/auth/verify-email/resend`) returns 500 when the mailer fails; map a provider outage to `503` with `Retry-After` and a clear code (requirement 3).
- Registration sends the verification email on the request path with a 10 s timeout; move it to the background (as forgot-password already does, `Service.background`) or accept the latency explicitly.
- Emails are plain text only; add an HTML part (`Message.HTML` exists) with the provider's template rules, and set `List-Unsubscribe`-style headers only if the provider requires them.
- Bounces and complaints: stop sending to an address that hard-bounced (provider suppression list).
