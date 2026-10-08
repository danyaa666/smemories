# T-015 — Web: auth pages and session handling

**Epic:** E02-auth · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Register, login, logout, verify-email, forgot/reset password pages; session bootstrap via GET /v1/me; protected-route wrapper; all strings in EN and VI; accessible forms with error messages mapped from API error codes.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._

#### Leader notes from the T-007 review (2026-10-08)
- The emailed links are `/verify-email?token=…` and `/reset-password?token=…`. Each page reads the token, then removes it from the address bar
  (`history.replaceState`) **before** any other work or network call, and loads no third-party resource (the API already sends `Referrer-Policy: no-referrer`).
- Forgot-password shows the same confirmation whether or not the account exists (the API answers 202 either way). Show a friendly message for `429` (use `Retry-After`) and
  for `400 invalid_token` ("link expired or already used, ask for a new one"). Resend verification is limited to 3 per hour: show it.
