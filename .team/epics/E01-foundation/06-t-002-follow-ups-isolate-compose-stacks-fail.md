# T-030 — T-002 follow-ups: isolate compose stacks, fail fast on auth errors, test-DB grants

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M0 · **Risk:** low · **Priority:** P2 · **Type:** tech-debt

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Description
Findings from the T-002 QA run. The compose project name is fixed (`name: smemories`), so two checkouts on one machine share containers and volumes: QA's `make up` recreated the dev's running MySQL container. The other items are small reliability and dev-experience fixes in the same area.

#### Scope
- In: per-checkout compose isolation; fail-fast on permanent DB errors; test-DB grants on existing volumes; clear error for an empty DSN in the API binary; README.
- Out (do not do): replacing MinIO (T-029), new tables, CI (T-004).

#### Acceptance criteria
- [ ] AC1 — Two checkouts in different directories can run `make up` at the same time without touching each other's containers or volumes: `make up` sets `COMPOSE_PROJECT_NAME` from the checkout directory name when it is not already set (the environment variable overrides the compose file's `name:`), and the README explains how to choose free host ports (`MYSQL_PORT`, `MINIO_PORT`, `MINIO_CONSOLE_PORT`). Demonstrate by running two stacks and stopping one.
- [ ] AC2 — `db.Open` stops retrying at once on errors that cannot fix themselves (MySQL error 1045 access denied, 1049 unknown database) and reports them naming the address and user, never the password; connection refused and timeouts still retry for up to 10 s. Unit-tested with a fake `*mysql.MySQLError`.
- [ ] AC3 — `make test-integration` works on a MySQL volume created before the grants script existed: `make up` idempotently ensures the `smem_test_%` grants (for example with `docker compose exec mysql mysql ...`), instead of relying on first-init only.
- [ ] AC4 — `smemories-api` with an empty `SMEM_DB_DSN` fails at startup with a clear message in every `SMEM_ENV` (including `test`), instead of dialling the default `127.0.0.1:3306`.
- [ ] AC5 — The README "Development" section documents AC1 and AC3.

#### Design
Files: `Makefile`, `docker-compose.yml` (only if needed), `internal/db/db.go`, `internal/db/db_test.go`, `internal/config/config.go` (and its test), `README.md`.

#### Risk
`low`

#### Security & performance notes
Never print the DSN or password in new messages. Keep compose ports on loopback.

#### Test plan
- Dev: unit tests for AC2 and AC4; manual two-stack demonstration for AC1; a run of AC3 against a volume created from the pre-T-030 compose file.
- QA should probe: wrong password (fails in well under 10 s), nonexistent database, MySQL down at start (still retries 10 s), two checkouts side by side, `make down` in one leaving the other running.
