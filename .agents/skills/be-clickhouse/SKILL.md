---
name: be-clickhouse
description: >
  ClickHouse schema design and query guidelines. Use when designing analytics tables,
  aggregation queries, or time-series reporting. Covers naming, DDL, ENGINE selection,
  ORDER BY strategy, data types, query design, write strategy, and migrations.
---

# ClickHouse Analytics Design

**Role:** Append-only analytics store for aggregation, reporting, and time-series. Never use as transactional source or for mutable records.

---

## Naming & Deployment

- Tables use `_tab` suffix (e.g., `game_transaction_tab`).
- Every table requires **two objects**: `<name>_tab_local_replicated` (local) + `<name>_tab` (distributed).
- **Reserved Words & Built-ins PROHIBITED:** Avoid column names that conflict with reserved keywords or built-in types/functions in MySQL, PostgreSQL, Clickhouse, Elasticsearch, Golang, or Python. 
  - *Bad:* `type`, `key`, `user`, `select`, `order`, `group`, `map`, `list`, `dict`, `str`, `int`.
  - *Good:* `item_type`, `api_key`, `account`, `is_selected`, `sort_order`, `user_group`.
### DDL Template

```sql
CREATE TABLE <db>.<name>_tab_local_replicated ON CLUSTER '{cluster}'
(
    id         UInt64,
    agent_id   UInt32,
    user_id    UInt64,
    -- domain columns --
    created_at Int64  -- Unix ms
)
ENGINE = ReplicatedMergeTree()
ORDER BY (agent_id, <filter_cols>, id);

CREATE TABLE <db>.<name>_tab ON CLUSTER '{cluster}'
    AS <db>.<name>_tab_local_replicated
    ENGINE = Distributed('{cluster}', <db>, <name>_tab_local_replicated, intHash64(user_id));
```

- Shard key: `intHash64(user_id)` for user-centric tables; `cityHash64(id)` otherwise.

---

## ENGINE Selection

| Scenario | ENGINE |
|---|---|
| Standard append-only | `ReplicatedMergeTree()` |
| Deduplication (e.g., login per user+date) | `ReplacingMergeTree(<version_col>)` |
| Pre-aggregated summaries | `ReplicatedSummingMergeTree()` |
| Approximate aggregation | `ReplicatedAggregatingMergeTree()` |

**Best practice:** Use **materialized views** with `AggregatingMergeTree` for frequently-queried aggregations — pre-compute at insert time rather than at query time.

---

## ORDER BY Strategy

`ORDER BY` = primary key = physical sort order. Most impactful schema decision for performance.

### Column Order (highest → lowest priority)

1. **Tenant/partition discriminator** — column every query filters first (e.g., `agent_id`). Prunes shards.
2. **Low-cardinality filters** — enum/status/type columns in `WHERE`. Lower cardinality → earlier position.
3. **Entity FK columns** — medium-cardinality columns used in `GROUP BY` or join filters.
4. **Time/range column** — monotonic timestamp or time-ordered ID. Enables time-range scans within prefix.
5. **Unique row ID** — always last. Guarantees key uniqueness and deterministic merges.

**Principle:** Lower cardinality → earlier in ORDER BY. High-cardinality first = no pruning benefit.

```sql
ORDER BY (tenant_col, low_card_1, low_card_2, entity_id, time_col, id)
```

| Do | Don't |
|---|---|
| Put every-query filter column first | Start with UUID or random high-cardinality ID |
| Order filters low → high cardinality | Put timestamp first when queries also filter type/status |
| End with unique row ID | Omit unique ID (non-deterministic merges) |
| Align with most frequent query shape | Copy ORDER BY from unrelated table |

---

## Data Types

| Kind | Type | Notes |
|---|---|---|
| IDs / PKs | `UInt64` | Time-ordered (`idgen`-style). NO UUID. |
| Composite keys (external) | `String` | |
| All timestamps | `Int64` / `UInt64` | Unix ms or s (consistent per table). **Prohibited:** `DateTime`, `DateTime64`, `DEFAULT now()`. |
| Low-cardinality strings | `LowCardinality(String)` | For columns with <10k distinct values (status, type, country). |

**Best practice:** Use `LowCardinality(String)` for enum-like string columns — dramatically reduces memory and improves compression.

### Time-Range Optimization via ID Boundaries

When table has time-ordered IDs in ORDER BY, convert `[startTime, endTime]` to `[startId, endId]` and filter on `id` — leverages primary key sort, avoids full-column scan.

---

## Query Design

### Mandatory: Filter by `agent_id` First
Every `WHERE` must start with `agent_id = {agentId:UInt32}`. Omitting = full distributed scan.

### Named Parameters Only
Use `{paramName:Type}` syntax (e.g., `{agentId:UInt32}`, `{startTime:Int64}`, `{userIds:Array(UInt64)}`). Never string-interpolate user input. Compile-time constants may use format strings.

### Query Organization
Declare all queries as **named constants** in a dedicated file (e.g., `queries.go`). No inline query construction.

### Useful Functions

| Pattern | Function |
|---|---|
| Approx distinct count | `uniq(col)` |
| Conditional sum/count | `sumIf(amount, cond)` / `countIf(cond)` |
| Time-bucket grouping | `toStartOfDay(toDateTime(ts, {tz:String}))` |
| Day-of-week / date math | `toDate(ts, {tz:String})` |
| Cast to integer | `::Int64` / `::UInt64` |

**Best practices (industry):**
- Use **skip indexes** (`minmax`, `set`, `bloom_filter`) on frequently filtered non-key columns to reduce granule scans.
- Apply **PREWHERE** for highly selective filters — reads less data from disk than `WHERE`.
- Use `FINAL` keyword sparingly with `ReplacingMergeTree` — prefer `argMax` patterns for better performance.
- Monitor with `system.query_log`, `system.parts`, `system.merges` to identify slow queries and merge pressure.

---

## Write Strategy

- ALWAYS batch: `PrepareBatch` → `AppendStruct()` → `Send()`. Never build INSERT strings manually.
- Maximize rows per batch — ClickHouse is optimized for large batches.
- Writes originate from async consumers (Kafka), not API handlers.
- Avoid frequent small inserts — they create excessive parts and merge pressure.

**Best practice:** Target **1 insert per second per table** maximum frequency. Buffer rows in the consumer and flush in large batches (10k–100k rows).

---

## Codec & Compression (Industry Best Practice)

- Default: `LZ4` (fast, good ratio). Use for most columns.
- Use `ZSTD` for columns with high compression benefit and lower query frequency.
- Apply `Delta` + `LZ4` for monotonically increasing columns (timestamps, sequential IDs).
- Specify per-column: `created_at Int64 CODEC(Delta, LZ4)`.

---

## TTL & Data Lifecycle (Industry Best Practice)

- Use `TTL` to auto-expire granular data: `TTL toDateTime(created_at/1000) + INTERVAL 90 DAY`.
- Combine with materialized views: keep detailed data 90 days, aggregated summaries indefinitely.
- Use `TTL ... TO VOLUME 'cold'` for tiered storage (hot → cold) on large clusters.

---

## Migrations

- File naming: `YYYYMMDDHHII_<description>.sql`.
- Each file may contain multiple DDL statements.
- ALTER must run on **both** `_local_replicated` and distributed table:

```sql
ALTER TABLE <db>.<name>_tab_local_replicated ON CLUSTER '{cluster}' ADD COLUMN new_col UInt32 DEFAULT 0;
ALTER TABLE <db>.<name>_tab ON CLUSTER '{cluster}' ADD COLUMN new_col UInt32 DEFAULT 0;
```

- No rollback (Down) scripts for append-only schema changes.

---

## Checklist

- [ ] `_tab` suffix on table name
- [ ] Both `_local_replicated` and `Distributed` tables created
- [ ] Correct ENGINE for access pattern
- [ ] No `DateTime`, `TIMESTAMP`, or server-side time defaults
- [ ] All time fields `Int64`/`UInt64`; IDs `UInt64`; no UUID
- [ ] `LowCardinality(String)` for enum-like string columns
- [ ] Time-range queries use ID-range pruning when time-ordered IDs exist
- [ ] All filter values use named parameters
- [ ] Queries declared as named constants
- [ ] Bulk inserts via prepared batch; writes from async consumers only
- [ ] Migration follows `YYYYMMDDHHII_<desc>.sql` naming
- [ ] ALTER runs on both local and distributed tables
- [ ] Compression codecs specified for large/monotonic columns

