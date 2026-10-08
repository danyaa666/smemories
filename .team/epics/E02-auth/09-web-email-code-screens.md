# T-049 — Web: code entry screens for email verification and password reset

**Epic:** E02-auth · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P1 · **Type:** feature

#### Description
The web side of D-22: after registering, the student enters the 6-digit code from the email; password reset becomes "enter your email, then the code and a new password". Replaces the link pages of T-015 (`/verify-email?token=…`, `/reset-password?token=…`) and the token-in-URL handling that existed only for links.

#### Scope
- In: `/verify-email` code screen (signed-in users), `/forgot-password` and `/reset-password` as a two-step flow, resend with a cooldown, error mapping, EN and VI strings, tests; removal of the token capture code and its tests.
- Out (do not do): API changes (T-048), SMS, magic links, a different layout for other pages.

#### Acceptance criteria
- [ ] AC1 — After a successful registration the student lands on `/verify-email` showing "we sent a code to <email>" and a code field. The field is a single input with `inputmode="numeric"`, `autocomplete="one-time-code"`, `maxlength` 6, digits only (pasting "123 456" or a code with spaces works), a visible label, and submits automatically when the sixth digit is entered or on Enter. The account page still shows a "verify your email" prompt linking to this screen while the email is unverified.
- [ ] AC2 — Errors from T-048 are shown in the user's language next to the field: `invalid_code` ("wrong code"), `code_expired` (offer "send a new code"), `code_locked` (too many wrong attempts, "send a new code"), `rate_limited` (use `Retry-After`), network error. No error text reveals whether an email exists (the reset flow shows the same message for unknown emails).
- [ ] AC3 — "Send a new code" calls the resend endpoint; the button is disabled for 60 seconds after each send with a visible countdown, and shows the 429 message when the API refuses.
- [ ] AC4 — Password reset: `/forgot-password` asks for the email and always moves to `/reset-password` with a neutral message ("if the account exists, we sent a code"); `/reset-password` asks for email (prefilled and editable), code and new password with the T-006 rules shown, calls `POST /v1/auth/reset-password`, and on `204` sends the student to the login page with a success message.
- [ ] AC5 — The old link handling is gone: `captureUrlToken`, `useUrlToken`, their tests and the `?token=` routes are removed; no page reads a secret from the URL. The `referrer` meta tag stays.
- [ ] AC6 — All strings exist in EN and VI (key parity check passes), the screens work at 375 px (no horizontal overflow with a long email), keyboard-only use works, and focus moves to the error or the field sensibly. Vitest tests cover paste, auto-submit, each error code, the resend cooldown (fake timers) and the two-step reset.

#### Design
Files: `web/src/pages/{VerifyEmail,ForgotPassword,ResetPassword}.tsx`, a small `CodeField` component, `web/src/api/auth.ts`, `web/src/auth/errors.ts`, locales, tests; delete `web/src/auth/useUrlToken.ts` and its use in `main.tsx`.
In dev, `123123` works (T-048), so QA and the owner can test without reading the log.

#### Risk
`high`: authentication UI (verification and reset flows). The owner approves the merge.

#### Security & performance notes
Never log or store the code in the browser (no localStorage, no URL). Do not echo the code in error messages. Autofill from the email app works through `autocomplete="one-time-code"`.

#### Test plan
- Dev: Vitest as in AC6; run the app against the real API with `SMEM_DEV_FIXED_OTP=123123` and say what was checked.
- QA should probe: paste with spaces and a trailing newline, letters, 7 digits, rapid double submit, back button after success, reload on `/reset-password` (the email field survives only if safe; no secret is lost because there is none in the URL), VI layout at 375 px, a screen reader pass if possible.
