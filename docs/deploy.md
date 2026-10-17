# Deploying and migrating the schema

How a release reaches production without the API and the database disagreeing (T-022 slice 3, T-088). The image and its tasks are in
[container.md](container.md); every variable is in [production-config.md](production-config.md).

## Deploy order

1. **Run the migrate task first**: the same image, command `smemories-migrate up` (ECS `RunTask` overriding the command), with the database
   variables only. Wait for it to stop; a non-zero exit code stops the release (codes below). Do not roll the API until it exits 0.
2. **Then roll the API** to the new image. While it rolls, old and new API tasks run side by side against the new schema.
3. If the release has a backfill (`smemories-media-backfill --dry-run`, then without), run it after step 2.

The API **never migrates on start**. At start-up it only reads `goose_db_version` and compares it with the migrations embedded in its own binary
(`db.CheckSchema`, writes nothing):

| Database | The API |
|---|---|
| a migration this build embeds is not applied (behind, or never migrated) | refuses to start, exit 1: `database schema is at version X, this build expects Y: N migrations pending; run smemories-migrate up first` |
| equal | starts |
| up to `db.MaxSchemaAhead` (3) applied migrations this build does not know | starts and logs **one** WARN at start-up (`database_version`, `build_version`, `migrations_ahead`): this is the previous release running against the next schema, which is allowed (rule below) |
| a gap: a migration this build embeds is not applied although a later one is | refuses to start, exit 1: `database schema is missing migration X (the next applied one is Y, ...)`; see the runbook below |
| more than 3 unknown migrations | refuses to start: `... N migrations ahead (at most 3 are compatible); deploy the newer image` |

The check runs once, at start-up. `/readyz` does not repeat it (database and Redis ping only), so a migration that lands while the API runs does not
take it out of the load balancer. A refused start is a failed task: ECS retries it, so starting the API before the migrate task finished is
harmless (it keeps failing until the schema is there) but noisy; keep the order.

## `smemories-migrate`

| Command | What it does |
|---|---|
| `up` | applies every pending migration under the lock, logs one JSON line `migration applied` per migration (`version`, `file`, `duration`), or `schema is up to date` |
| `status` | one line per embedded migration: `applied <time>` or `pending` |
| `version` | prints the highest applied version (`0` on an empty database) |
| `down --allow-down` | rolls back one migration. **Refused with `SMEM_ENV=prod`** (with or without the flag); `down` without the flag is a usage error. For dev and test only. |

**Lock.** `up` and `down` take the MySQL advisory lock (`GET_LOCK`, name derived from the database name) on the connection that runs the migrations. A second
task started at the same time waits (up to 5 minutes), then finds nothing left to apply and exits 0; the lock is released when the session ends, so a killed task
never leaves it stuck. A task that cannot get the lock in 5 minutes exits 3. Each migration runs in its own statement group; MySQL commits DDL implicitly, so a failed
migration can leave a half-applied table (runbook below). Never edit an applied migration; a bug in one is fixed by a new migration.

**Exit codes.**

| Code | Meaning |
|---|---|
| `0` | success (including nothing to do) |
| `1` | configuration, connection or migration failure |
| `2` | usage error: unknown command or flag, a bare `down` (it needs `--allow-down`), `up --allow-down`; in prod too, a bare `down` is a usage error, not a refusal |
| `3` | the lock was still held after 5 minutes |
| `4` | **only** `down --allow-down` with `SMEM_ENV=prod` (refused before any connection) |

`cmd/smemories-migrate/main_test.go` runs the command in prod mode and checks 2 for `down` and 4 for `down --allow-down`; `make image-smoke` checks the same on the image.

The pool needs two connections (one holds the lock); the command raises `SMEM_DB_MAX_OPEN` to 2 if it is lower.

## A half-applied migration (runbook)

MySQL commits every DDL statement at once and cannot roll it back. A migration with several statements, killed in the middle (SIGKILL, an
OOM kill, a task stopped by ECS, a lost connection), keeps the statements that already ran and never records its version in
`goose_db_version`. The next `smemories-migrate up` runs the migration from its first statement and stops at the first one that already
took effect, with MySQL error 1060 (`Duplicate column name`), 1050 (`Table ... already exists`), 1061 (`Duplicate key name`) or 1091
(`Can't DROP ...; check that column/key exists`). `up` then prints the hint `this migration may be half-applied ...` and exits 1; **do not
roll the API**, it is still correct against the previous schema.

1. **Find the migration.** `smemories-migrate version` prints the highest applied version; `smemories-migrate status` lists every embedded
   migration, and the first one shown as `pending` is the dirty one (its file is in `migrations/`). The `up` log names no `migration applied`
   line for it.
2. **See how far it got.** Read its `-- +goose Up` block and compare each statement with the live schema (`SHOW CREATE TABLE ...`,
   `SHOW INDEX FROM ...`, `information_schema.columns`). The statements before the failing one ran; the failing one and the rest did not.
3. **Repair by hand, as the application user**: either undo the applied statements (`DROP TABLE`, `DROP COLUMN`, `DROP INDEX`)
   and run `up` again, or run the missing statements yourself and record the version
   (`INSERT INTO goose_db_version (version_id, is_applied) VALUES (<version>, 1)`). Undoing is the safer one when the table has no data yet;
   finishing is the only one when it holds data or the rest of the migration is a backfill. Take a snapshot first (RDS) for a table with data.
4. **Check**: `smemories-migrate up` prints `schema is up to date`, `status` shows no `pending`, then roll the API.
5. **Write down what happened** and make the next migration safer (below).

MySQL 8.4 has no `ADD COLUMN IF NOT EXISTS`, `ADD INDEX IF NOT EXISTS` or `DROP COLUMN IF EXISTS`; only `CREATE TABLE IF NOT EXISTS` and
`DROP TABLE IF EXISTS`. So: write a table creation with `IF NOT EXISTS`; give a migration **one** `ALTER TABLE` with all its changes
(MySQL 8 DDL is atomic per statement, so a kill leaves one `ALTER` either done or not done, and a half-applied state needs two or more
statements); put a second DDL statement into its own migration file, so a kill between them leaves a cleanly recorded version instead.
Never mix a backfill and the DDL it depends on in one file.

A gap error (`missing migration X (the next applied one is Y)`) means a version was recorded as applied while an older one was not (a
migration merged with a timestamp older than one already deployed, or a hand-edited `goose_db_version`). Apply the missing one by hand as in
step 3 and record it, or re-time the file to a version later than the newest applied one before it ships.

## Expand then contract

From now on every migration must work with **the previous API version** as well as the new one, because the migrate task runs before the API rolls (and a
rollback of the API leaves the new schema in place). Split a breaking change over two releases:

- **Expand** (release N): add the column, table or index in a way the old code ignores: a new column is `NULL` or has a `DEFAULT`; new tables are free; add
  indexes, do not drop them. Release N's code writes both shapes.
- **Contract** (release N+1 or later, after N is the only version running): drop the old column, the old table, the old index.

Counter-example, already shipped: T-081 added `user_tab.username` as `NOT NULL` **without a default** (after a backfill). Once that migration ran, a previous API
version, which does not know the column, can no longer insert users: every registration fails with MySQL error 1364. T-081 had no previous API in production, so
it was safe; the same change from now on is two releases: add `username NULL` (or with a default) and write it, then in the next release backfill what is left
and `MODIFY ... NOT NULL`.

`db.MaxSchemaAhead` (3) is the number of migrations one release may carry before the previous release is refused by this check; keep a release to a few migrations,
and never put an expand and its contract in the same release.

## Reviewer checklist for a migration

- [ ] Does the previous API version still work after only this migration ran? (new columns `NULL` or defaulted; nothing the old code reads is dropped, renamed or retyped)
- [ ] Is every drop, rename or `NOT NULL` tightening in a later release than the code that stopped using the old shape?
- [ ] Is the `-- +goose Down` honest? (It is for dev and test; prod never runs it.)
- [ ] Table and column conventions of [db-conventions.md](db-conventions.md): `_tab`, BIGINT ms timestamps, no foreign keys.
- [ ] Locking and duration on a big table: does `ALTER TABLE` rebuild it? Use `ALGORITHM=INPLACE, LOCK=NONE` where MySQL allows, else say how long it takes and when to run it.
- [ ] A backfill in the same migration must be bounded and idempotent; a long one belongs in `smemories-media-backfill`-style task, not in a migration.
- [ ] The file name is a timestamp version later than every existing one (goose refuses an older version added after a newer one was applied: "missing migrations").
- [ ] An integration test runs the migration on a database that already has rows.
- [ ] Safe to run twice after a kill? `CREATE TABLE IF NOT EXISTS`; one `ALTER TABLE` with all changes of a table (all or nothing); a second DDL statement in its own file; see "A half-applied migration".
