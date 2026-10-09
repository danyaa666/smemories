---
name: be-rldb
description: >
  Relational database schema design standards. Use when designing or reviewing DB schemas
  in design docs or implementation. Covers naming, PKs, IDs, timestamps, indexes,
  soft deletes, and migration rules.
---

# Relational DB Design Standards

**Principle:** Consistency, auditability, application-level integrity.

---

## Naming
- All tables use `_tab` suffix (e.g., `staff_tab`, `user_session_tab`).
- **Reserved Words & Built-ins PROHIBITED:** Avoid column names that conflict with reserved keywords or built-in types/functions in MySQL, PostgreSQL, Clickhouse, Elasticsearch, Golang, or Python. 
  - *Bad:* `type`, `key`, `user`, `select`, `order`, `group`, `map`, `list`, `dict`, `str`, `int`.
  - *Good:* `item_type`, `api_key`, `account`, `is_selected`, `sort_order`, `user_group`.

## Primary Keys
- ALWAYS `BIGINT UNSIGNED AUTO_INCREMENT`.
- UUID only for external system compatibility (must document justification).

## ID Strategy
- Internal: `AUTO_INCREMENT`.
- Client-facing / partitioned: ordered unique ID via [`idgen`](https://github.com/whitelabel6688/go-common/tree/master/idgen).
- NO UUID unless clear technical requirement.

## Timestamps
All time fields: **Unix ms as `BIGINT`**.

| Column | Type | When |
|---|---|---|
| `created_at` | `BIGINT NOT NULL` | Set on insert |
| `updated_at` | `BIGINT NOT NULL` | Set on insert + update |
| `created_by` | `BIGINT UNSIGNED` | (Optional) Actor who created |
| `updated_by` | `BIGINT UNSIGNED` | (Optional) Actor who last updated |

**Prohibited:** `TIMESTAMP`, `DATETIME`, `ON UPDATE CURRENT_TIMESTAMP`.

## Foreign Keys
- DB-level FK constraints **PROHIBITED**.
- Enforce referential integrity in Manager/Repository code.
- ALWAYS index virtual FK columns: `INDEX idx_user_id (user_id)`.

## Soft Deletes
- Use `deleted_at BIGINT DEFAULT NULL` (non-null = deleted, value = deletion timestamp ms).
- NOT boolean `is_deleted`.

---

## Table Template

```sql
CREATE TABLE example_tab (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id    BIGINT UNSIGNED NOT NULL,
  name       VARCHAR(255)    NOT NULL,
  status     TINYINT         NOT NULL DEFAULT 0,
  created_at BIGINT          NOT NULL,
  updated_at BIGINT          NOT NULL,
  deleted_at BIGINT          DEFAULT NULL,
  PRIMARY KEY (id),
  INDEX idx_user_id (user_id),
  INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

---

## Index Design (Industry Best Practices)

- Create **covering indexes** for frequent queries — include all `SELECT` columns to avoid table lookups.
- Use **composite indexes** following leftmost-prefix rule — order columns by selectivity (most selective first for point queries, least selective first for range scans).
- Avoid over-indexing: each index adds write overhead. Target only proven query patterns.
- For large tables (>10M rows), consider **partitioning** by time range (`PARTITION BY RANGE`) to improve query pruning and maintenance operations.
- Monitor slow queries and use `EXPLAIN` to validate index usage.

## Connection Pool Guidelines (Industry Best Practice)

- Set **max open connections** based on `(CPU cores * 2) + effective_spindle_count` as baseline.
- Set **max idle connections** = max open connections (avoid connection churn).
- Set **connection max lifetime** to < DB server's `wait_timeout` (typically 5–10 min).
- Use connection pool metrics (active, idle, wait count) for capacity monitoring.

---

## Migrations

- Tool: Goose.
- File naming: `{YYYYMMDDHHMMSS}_{description}.sql`.
- Always include both `-- +goose Up` and `-- +goose Down` blocks.

---

## Checklist

- [ ] `_tab` suffix on all tables
- [ ] PKs are `BIGINT UNSIGNED AUTO_INCREMENT`
- [ ] No `TIMESTAMP`, `DATETIME`, or `ON UPDATE CURRENT_TIMESTAMP`
- [ ] `created_at` and `updated_at` on every table
- [ ] No DB-level foreign key constraints
- [ ] All virtual FK columns indexed
- [ ] Soft delete uses `deleted_at BIGINT DEFAULT NULL`
- [ ] Client-facing IDs use `idgen`, not UUID
- [ ] Migration has both Up and Down blocks
- [ ] Composite indexes follow leftmost-prefix rule
