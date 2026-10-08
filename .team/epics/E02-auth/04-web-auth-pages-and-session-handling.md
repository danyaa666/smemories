# T-015 — Web: auth pages and session handling

> **Superseded in part (2026-10-08, D-22):** the emailed link tokens of this task were replaced by 6-digit codes: see T-048 (API) and T-049 (web). The mailer, rate-limit and cleanup parts stay.

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

#### Leader notes from the T-011 review (2026-10-08)
"Sign in with Google" is a plain link (full page navigation, not fetch) to `/api/v1/auth/google/start?return_to=<current path>`; show it only when the API has Google enabled
(add a small public config flag if needed, or hide the button behind a Vite env switch for now). The login page reads `?error=` and shows a translated message for
`oidc_state` ("the sign-in took too long or was opened in another browser, try again"), `oidc_denied`, `oidc_failed` and `email_unverified`; unknown codes get a generic message.
On success the API redirects to `return_to` with the session cookie already set: call `GET /v1/me` on load. A social-only account has no password: the "forgot password" flow is how it sets one.
