# Dev-only shortcuts

Code that exists only to make development and testing easier and would be a backdoor in production (owner rule, CLAUDE.md).
Every line of one carries the comment tag `DEV-SHORTCUT(<name>)`; `grep -rn "DEV-SHORTCUT" --exclude-dir=.git --exclude-dir=node_modules .`
lists them. All of them are deleted before the first production deploy (T-050 does it and adds `scripts/check-no-dev-shortcuts.sh`
to CI and the deploy pipeline). Nobody sets a shortcut variable in a production configuration or in infrastructure code.

## `DEV-SHORTCUT(otp)`: the fixed email code (T-048, D-22)

| | |
|---|---|
| **What** | `SMEM_DEV_FIXED_OTP` (exactly 6 digits, `123123` in `.env.example`) is accepted as the code of every email-code check. For `POST /v1/auth/verify-email` it verifies the signed-in user (no outstanding code needed); for `POST /v1/auth/reset-password` it resets any existing account named by `email` (no `forgot-password` needed; an unknown address is still `invalid_code`). It consumes no attempt and is not single use. Any other 6-digit code is still checked normally. |
| **Where** | `internal/config/config.go` (`DevFixedOTP`, `loadOTP`), `internal/auth/codes.go` (`Codes.fixed`, the guard in `NewCodes`, the early return in `Codes.Check`), `cmd/smemories-api/main.go` (argument and the startup warning), `.env.example`, tests tagged in `internal/auth/*_test.go` and `internal/config/config_test.go`. |
| **Guard** | 1. `config.Load` refuses to start when the variable is set and the raw `SMEM_ENV` is not `dev` or `test` (an empty `SMEM_ENV` counts as not set to dev). 2. `auth.NewCodes` refuses the same combination again, so a caller that skips the config cannot enable it. 3. The API logs `WARN dev fixed otp active` once at startup. The variable is unset by default. |
| **Remove** | Delete every line tagged `DEV-SHORTCUT(otp)` (config field and parsing, the `fixed` field and its two uses in `codes.go`, the two lines in `main.go`, the `.env.example` entry), the tests that mention it, and this entry; then `scripts/check-no-dev-shortcuts.sh` (T-050) must pass. Postman's flow requests that use the code then need real codes. |
