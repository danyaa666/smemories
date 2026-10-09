# E06_T-050 — Remove dev-only shortcuts before production (delete the fixed OTP)

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P1 · **Type:** security

#### Description
Owner instruction (2026-10-08): the dev-only fixed OTP `123123` (T-048, `SMEM_DEV_FIXED_OTP`) must be deleted when we go to production. This task is the go-live gate for every dev-only shortcut registered in `docs/dev-shortcuts.md`: remove them from the code and make the pipeline fail if one comes back. It must be done before the first production deploy (T-023) and is a prerequisite of it.

#### Scope
- In: delete the fixed-OTP code path, its tests and its `.env.example` line; a check script and a CI step; the docs update.
- Out (do not do): removing the LogMailer (it stays for dev and test, guarded by `SMEM_ENV`), removing test helpers that live only in `_test.go` files.

#### Acceptance criteria
- [ ] AC1 — Every line tagged `DEV-SHORTCUT` in non-test Go and web sources is removed, together with its tests and the `SMEM_DEV_FIXED_OTP` entries in `.env.example`, README, Postman and docs. Dev flows that used `123123` keep working through the real code path (read the code from the LogMailer output) or a test-only helper outside the shipped binary; the dev says which.
- [ ] AC2 — `scripts/check-no-dev-shortcuts.sh` fails (non-zero, lists file and line) if `DEV-SHORTCUT`, `SMEM_DEV_FIXED_OTP` or the literal `123123` appears in non-test source, configuration or Dockerfiles; it passes on the cleaned tree. It runs in CI as part of the `security` job and as a required step of the deploy pipeline (T-023).
- [ ] AC3 — A startup test proves the production binary has no code path that reads `SMEM_DEV_FIXED_OTP` (grep of the binary strings in the test or in the script is acceptable) and that setting it in the environment no longer changes anything or refuses to start (it is simply ignored, with a startup warning if set).
- [ ] AC4 — `docs/dev-shortcuts.md` says "no shortcuts present" with the date, and keeps the rule for adding one in the future (tag, guard, registry line, removal task).

#### Design
Files: `internal/auth/*` (remove the tagged blocks), `internal/config/config.go`, `cmd/smemories-api/main.go`, `.env.example`, `README.md`, `postman/auth.postman_collection.json`, `scripts/check-no-dev-shortcuts.sh`, `.github/workflows/ci.yml`, `docs/dev-shortcuts.md`.

#### Risk
`high`: authentication code and CI. The owner approves the merge.

#### Security & performance notes
This is the control that stops a test backdoor reaching real users; the check must be part of the deploy gate, not only of CI.

#### Test plan
- Dev: run the script on the dirty tree (fails) and the clean tree (passes); full auth test suite.
- QA should probe: re-adding a tagged line (the script catches it), the literal `123123` in a comment, a Dockerfile `ENV`, case variants.
