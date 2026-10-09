# E10_T-067 — DB conventions: user and identity tables

**Epic:** E10-skills-alignment · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P1 · **Type:** tech-debt
**Read first:** `docs/db-conventions.md`, T-052 and T-048 specs (they removed `sessions` and `email_tokens`).
**Depends on:** T-066, T-048, T-052.

#### Description
Convert `users` and `user_identities` to `user_tab` and `user_identity_tab`. Nothing else in auth is stored in MySQL after D-23.

#### Requirements (SHALL)
1. `user_tab`: `email_verified_at` BIGINT NULL ms, `created_at`/`updated_at` BIGINT ms, `locale` VARCHAR(8) validated in Go, email stays `utf8mb4_bin` with its comment, `uq_user_tab_public_id`, `uq_user_tab_email` kept.
2. `user_identity_tab`: `provider` VARCHAR(16), ms timestamps, `updated_at` added, no FK, `idx_user_id` kept, `uq_user_identity_provider_subject` kept.
3. Code that used the user FK cascades (none deleted users) SHALL be re-checked: there is no user-deletion feature; add the line "delete the user's yearbooks through `yearbook.Service.Delete`, their identities, then the user, in one transaction; extend the zero-rows test" to the **T-025 spec** (leader does that, dev only verifies the note exists).
4. The `last_seen` / session concepts are gone (Redis); nothing in MySQL stores them.

#### Acceptance criteria
- [ ] AC1 — Migration with Up/Down and round-trip test (verified and unverified users, social-only user with NULL password hash, boundary timestamps, an email with an accent to prove the binary collation survives).
- [ ] AC2 — `auth.Store` returns ms; every other behaviour (login, Google linking and its retry logic from T-047, verification, rate limits) unchanged; existing tests pass with only SQL-name changes.
- [ ] AC3 — `dbtest` truncation order/table lists updated; the full-book zero-rows test from T-066 still passes.
- [ ] AC4 — `grep -rn "FOREIGN KEY\|DATETIME\|TIMESTAMP\|ENUM(" migrations internal` finds nothing outside already-merged migration files and migration tests; the PR description shows the output of `SHOW CREATE TABLE` for all remaining `*_tab` tables.

#### Risk
`high`: auth tables, data migration. Owner approves.

#### Test plan
- QA should probe: Google first-login race (T-047 stress test still green), re-register of an existing email with different case/accents, Down/Up with identities present, `\d`-style comparison of every table against `docs/db-conventions.md`.
