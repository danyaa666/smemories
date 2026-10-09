# Database conventions (MySQL)

Standard for every schema change. Source skill: `.agents/skills/be-rldb`, adapted by decision **D-26** (2026-10-09).
Existing tables are converted domain by domain (epic E10, tasks T-064..T-067); **new tables follow this file from the first commit**.

## Rules

| Topic | Rule |
|---|---|
| Table names | singular noun + `_tab`: `user_tab`, `yearbook_tab`, `note_collection_tab`. The goose table `goose_db_version` is not ours. |
| Primary key | `id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT`. No UUID. |
| Public id | rows that clients can address have `public_id CHAR(26) NOT NULL` (ULID, `internal/ulid`), `UNIQUE KEY uq_<table>_public_id`. This is our ordered unique id for client-facing use (L-05); `idgen` of the skill is not available to us. Internal ids never appear in the API. |
| Timestamps | every table has `created_at BIGINT NOT NULL` and `updated_at BIGINT NOT NULL`, Unix **milliseconds** (UTC by definition). Insert sets both; updates set `updated_at`. Other instants (`deadline_at`, `used_at`, `revoked_at`, `expires_at`) are `BIGINT NULL`. No `TIMESTAMP`, `DATETIME`, `ON UPDATE CURRENT_TIMESTAMP`. Go sets them from the injected clock (`now().UnixMilli()`), never `NOW()` in SQL. |
| Calendar dates | a date that is not an instant (birthday) stays `DATE`. Documented exception. |
| Foreign keys | **no DB-level `FOREIGN KEY`**. Integrity lives in the Go service: check the parent exists, delete children explicitly in one transaction (section "Deleting"). Every referencing column is indexed. |
| Indexes | `idx_<columns>` (plain), `uq_<table>_<columns>` (unique). Composite indexes follow the leftmost-prefix rule and match a real query. No speculative indexes. Check hot queries with `EXPLAIN` in the PR. |
| Enums | no MySQL `ENUM`. Use `VARCHAR(n)` (text values, validated in Go against a constant list) or `TINYINT` (small fixed codes). Adding a value then needs no `ALTER`. |
| Booleans | `TINYINT UNSIGNED NOT NULL DEFAULT 0`. |
| Column names | must not be reserved words or built-ins of MySQL, PostgreSQL, Go or Python (`type`, `key`, `user`, `order`, `group`, `map`, `list`, `str`, `int` ...). Use `item_type`, `api_key`, `sort_order`. Check new names against `SELECT WORD FROM information_schema.KEYWORDS WHERE RESERVED = 1`. |
| Soft delete | not used today (user deletion is a hard delete, privacy, D-26). If a feature ever needs it: `deleted_at BIGINT DEFAULT NULL` (never `is_deleted`) plus `idx_deleted_at`, and a decision record first. |
| Charset | `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`, collation as today; a case-sensitive key (email, OIDC subject) says so in a comment. |
| Joins | the skill asks for application-layer joins: read the parent, then the children/related rows with one `WHERE id IN (...)` per relation. No query inside a loop (N+1). A join stays only where it is 1:1, owner-scoped and measured; say why in the code. |
| Writes | batch writes of 2000-5000 rows; transactions short; lock order is documented when a transaction takes several locks. |

## Deleting without foreign keys

The service that owns the parent exposes the delete and calls every child owner through a small interface defined **in the parent's package**
(consumer side), each child implementing `DeleteByYearbook(ctx, tx *sql.Tx, yearbookID uint64) error`. One transaction covers the parent and
all children; object-storage deletes (not transactional) run first and are retried (existing `Purger` behaviour). Each conversion task adds an
integration test that deletes a fully populated parent and asserts **zero rows** remain in every table that refers to it. There is no user
deletion yet; T-025 must delete a user the same way and extend that test.

## Migrations

- Tool: goose, SQL files embedded in `migrations/`. New files are named `{YYYYMMDDHHMMSS}_{description}.sql` (UTC). The existing `0001`..`0009` files stay as they are (renaming applied migrations breaks version tracking); goose orders by number, and a 14-digit timestamp is always larger.
- Every file has `-- +goose Up` and `-- +goose Down`; Down must restore the previous schema **and data** (round trip tested for conversions).
- Conversions use the add-column, backfill, drop, rename pattern in one file per table so a failure leaves a visible, resumable state; backfill is time-zone independent: `TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', col) DIV 1000` (the old DATETIME(6) values are UTC).
- Never edit a migration that is merged.

## Connection pool

`SetMaxOpenConns` from config (default 20); `SetMaxIdleConns` equals max open by default (no churn); `SetConnMaxLifetime` below the server `wait_timeout` (default 5 minutes). Pool numbers are logged at start-up and checked by `/readyz`.

## Checklist for a schema PR

- [ ] `_tab` suffix, `BIGINT UNSIGNED AUTO_INCREMENT` id, `public_id` if clients address it
- [ ] `created_at` / `updated_at` BIGINT ms; no DATETIME/TIMESTAMP
- [ ] no FOREIGN KEY; every referencing column indexed; delete path in Go and its zero-rows test
- [ ] no ENUM; no reserved column names
- [ ] Up and Down, Down tested for conversions
- [ ] queries read without joins or loops
