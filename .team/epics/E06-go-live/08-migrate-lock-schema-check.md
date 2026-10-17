# E06_T-088 — smemories-migrate lock, API schema-version check and deploy order (T-022 slice 3)

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** infra
**Depends on:** T-086 (slice 2a merged). **Source of truth for the text:** `02-dockerfile-production-config-and-migrations.md`, requirement 6 and AC5, plus `docs/production-config.md` and `docs/container.md`.

#### In scope
Requirement 6 / AC5 of the T-022 spec: `smemories-migrate` takes a MySQL advisory lock (`GET_LOCK`) so two tasks cannot migrate at once, is idempotent, supports `up`, `status`, `version`, logs each applied migration, exits non-zero on failure and never runs `down` in prod (`--allow-down` only with `SMEM_ENV` not prod); the API checks at start-up that the schema is at the version it expects (refuse to serve when the DB is behind, or ahead beyond the compatible range; the message names both versions) and never migrates on start; the deploy order (migrate task first, then roll the API) and the "expand then contract" rule are documented, with the `user_tab.username` (T-081) example (a column without a default breaks the previous API version) and a short checklist for reviewers of migrations.

#### Acceptance criteria
- [ ] AC1 - An integration test runs two `smemories-migrate up` at once: one waits for the lock or exits cleanly, the schema is correct, nothing is applied twice.
- [ ] AC2 - The API refuses to start when the schema is behind or too far ahead, with a message naming both versions; with the schema at the expected version it starts; `/readyz` does not depend on this check after start-up.
- [ ] AC3 - `down` is refused in prod; `--allow-down` is refused in prod too.
- [ ] AC4 - Docs: the deploy order, the expand/contract rule, the reviewer checklist, and the exit codes of `smemories-migrate`.
- [ ] AC5 - `make image-smoke` still passes and now also proves: a second concurrent migrate task is safe, and the API refuses to start against an un-migrated database.
