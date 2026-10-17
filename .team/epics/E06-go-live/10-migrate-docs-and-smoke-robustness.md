# E06_T-090 — Migrate docs fixes, half-applied migration guidance and image-smoke robustness

**Epic:** E06-go-live · **Milestone:** M2 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt
**Depends on:** T-088 (merged) and T-089 (merged; both touch `scripts/image-smoke.sh` and `docs/deploy.md`/`docs/container.md`). Source: the non-blocking findings of the T-088 QA.

#### Description
Five small leftovers of the migration-lock work:
1. **Half-applied migration after a SIGKILL.** A process killed in the middle of a DDL statement leaves a half-applied migration, and the next `smemories-migrate up` fails loudly with "Duplicate column" or "Table already exists". `docs/deploy.md` only says "fix forward", which does not cover this. Write the recovery runbook (how to see which migration is dirty with `status`/`version`, how to inspect and repair by hand, why every migration should be written idempotently where MySQL allows: `CREATE TABLE IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS` is not available in MySQL 8.4 so say what to do instead) and add a reviewer-checklist line. Optionally have `up` print this hint when it fails with those two error numbers (1060, 1050, 1061, 1091).
2. **Silent "ahead" start.** An API started against a schema 1 to 3 migrations ahead starts without any log line. Log one WARN naming both versions (it is the normal state during a rolling deploy, so once at start-up only).
3. **Confusing gap message.** The message for a gap in the middle of the applied versions names the same version twice. Fix the text so it names the missing version and the next applied one.
4. **Wrong exit code in the docs.** `docs/deploy.md` says a bare `down` is refused with exit 4 in prod; it actually exits 2 (only `down --allow-down` in prod exits 4). Fix the doc and add the exit-code table row.
5. **`make image-smoke` can hang and can clash.** It has no timeout on the API run (it hung 20 minutes when another process overwrote the shared `smemories:local` tag). Give the smoke test its own image tag per run (for example `smemories:smoke-<pid>` or the git SHA), a hard timeout on every wait, and clean up the tag at the end; the CI `image` job keeps working.

#### Acceptance criteria
- [ ] AC1 - `docs/deploy.md` has the half-applied-migration runbook and the checklist line; if the failure hint is implemented, a test provokes error 1060 and 1050 and sees it.
- [ ] AC2 - A start against a schema 1-3 ahead logs exactly one WARN naming both versions (test); nothing else about start-up behaviour changes.
- [ ] AC3 - The gap message names two different versions (unit test).
- [ ] AC4 - `docs/deploy.md` exit codes match the code (a test or a script that runs `down` and `down --allow-down` in prod mode and checks 2 and 4).
- [ ] AC5 - `make image-smoke` uses its own tag and a timeout on each wait, still passes in about 25-40 s, and two runs started at the same time do not interfere; CI `image` job green.
- [ ] AC6 - `make lint build test`, `go test -race -tags integration ./...` and `make image-smoke` pass.
