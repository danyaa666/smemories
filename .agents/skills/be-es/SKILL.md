---
name: be-es
description: >
  Elasticsearch index design and query guidelines. Use when designing search indexes,
  filtered listings, full-text search, or event display. Covers naming, mappings,
  field types, document IDs, denormalization, pagination, write strategy, and migrations.
---

# Elasticsearch Design

**Role:** Denormalized read-store for filtering, listing, and full-text search. NOT a transactional source. Data written asynchronously (via message consumer) after RDBMS source-of-truth update. Optimize for query performance.

---

## Index Naming

- `snake_case`, no suffixes (e.g., `order`, `user`, `audit_event`).
- One index per logical entity. Concise, lowercase.
- No version numbers in name unless managing aliases for zero-downtime reindexing.
- **Reserved Words & Built-ins PROHIBITED:** Avoid column names that conflict with reserved keywords or built-in types/functions in MySQL, PostgreSQL, Clickhouse, Elasticsearch, Golang, or Python. 
  - *Bad:* `type`, `key`, `user`, `select`, `order`, `group`, `map`, `list`, `dict`, `str`, `int`.
  - *Good:* `item_type`, `api_key`, `account`, `is_selected`, `sort_order`, `user_group`.

**Best practice:** Use **index aliases** (e.g., `order` → `order_v1`) to enable zero-downtime reindexing and blue-green deployments.

---

## Index Mapping

Store as `<index_name>.json` alongside migration scripts.

```json
{
  "settings": {
    "number_of_shards": 3,
    "number_of_replicas": 2
  },
  "mappings": {
    "dynamic": false,
    "properties": {}
  }
}
```

- **`dynamic: false`** — ALWAYS. Prevents uncontrolled field creation and mapping explosions.
- Increase `index.max_result_window` to `11000` only when deep-pagination or large batch retrieval required.

### Field Types

| Data kind | ES type | Notes |
|---|---|---|
| Small integer IDs | `integer` | |
| Large IDs / PKs | `long` | |
| Enum / status / code (exact match) | `integer` or `keyword` | `integer` for numeric enums; `keyword` for string codes |
| Free-text (search) | `text` | Only when full-text/fuzzy search needed |
| Unix timestamp (ms/s) | `long` | Never use ES `date` type for numeric timestamps |
| Boolean | `boolean` | |
| Array of objects with sub-queries | `nested` | Use sparingly — increases query complexity |

**Best practice:** For `keyword` fields with very high cardinality (>1M unique values), consider `eager_global_ordinals: false` to reduce heap pressure.

---

## Document ID Strategy

IDs must be **deterministic and idempotent** (safe re-indexing = upsert semantics).

| Pattern | ID format |
|---|---|
| Single-entity (1 doc per record) | Record's primary key |
| Composite-key entity | `tenantId:userId:entityId` (delimiter-separated discriminators) |
| Event-type (deduplicated) | `tenantId:userId:eventType:objectId` |

**Prohibited:** Random UUIDs or ES auto-generated IDs.

---

## Denormalization Rules

- Keep documents flat; use `nested` only for independently-queried sub-objects (e.g., multilingual arrays).
- Every document MUST include a **tenant/scope identifier** — first filter in every query.
- Always include `created_at` (and `updated_at` when applicable) as `long`.
- Do not embed frequently-changing sub-documents queried independently — use separate index or denormalize only stable subset.
- Mirror enum values as integers matching source system.

---

## Query Design

### Filtering
- Scope every query with tenant/scope ID as first `must` term clause.
- `term` for single exact match; `terms` for multi-value; `must_not` + `terms` for exclusion.
- Default sort: `created_at` DESC.
- Separate `count` query for totals — do not rely on bounded `size` search total.

### Full-Text / Fuzzy
- `text` fields only for content needing fuzzy match (titles, descriptions, tags).
- Multilingual nested arrays: use `nested` query targeting specific language sub-object.
- Apply `fuzziness` only when typo-tolerance required — adds query cost.

**Best practices (industry):**
- Use `search_after` instead of `scroll` API for stateless deep iteration in modern ES (7.10+).
- Apply **query-time boosting** (`boost` param) rather than index-time boosting for flexible relevance tuning.
- Use `filter` context (not `must`) for non-scoring clauses — leverages query cache and skips scoring overhead.
- Monitor `_nodes/stats` and cluster health; set **circuit breaker** thresholds to prevent OOM.

---

## Pagination — Cursor (Next-ID) Based

Do NOT use `from`/`size` offset beyond shallow pages.

**Pattern:**
1. Request `limit + 1` documents.
2. If `limit + 1` returned → extra doc's ID (+ `created_at` tie-breaker) = `next_id` cursor.
3. Return only `limit` docs to caller.
4. For time-ordered entities, carry `next_created_at` + secondary tie-breaker alongside `next_id`.

---

## Write Strategy

- All writes MUST be **bulk** — never one doc per HTTP call.
- Use **upsert** (`doc_as_upsert: true`) — safe to replay without existence checks.
- Set `updated_at` to current time before every bulk-update.
- Writes from async consumers only, not API handlers.

**Best practice:** Use `refresh_interval: "30s"` (or higher) for write-heavy indexes to reduce segment merge overhead. Only use `refresh: "true"` for near-real-time requirements.

---

## Index Lifecycle Management (Industry Best Practice)

- Apply **ILM policies** for time-based indexes: hot → warm → cold → delete phases.
- Use **rollover** for high-volume indexes: auto-create new index when size/age threshold reached.
- Define `max_primary_shard_size` or `max_age` rollover conditions.
- Combine with aliases so application code always targets a consistent name.

---

## Migrations

- Mapping files: `<index_name>.json` (idempotent desired state, no timestamps).
- Create: `PUT /<index_name>` with JSON body before deploying code using the index.
- Add field: update JSON mapping + `PUT /<index_name>/_mapping` with new field.
- Change field type: requires reindex — create new index, reindex data, swap alias.
- Never delete fields from live index without reindex plan.

---

## Checklist

- [ ] Index name is `snake_case`, no suffix
- [ ] Mapping file exists as `<index_name>.json`
- [ ] `dynamic: false` in mappings
- [ ] `number_of_shards: 3`, `number_of_replicas: 2`
- [ ] Document ID is deterministic (PK or composite — no random UUIDs)
- [ ] Time fields as `long`; enums as `integer`/`keyword`
- [ ] `nested` only for independently-queried sub-object arrays
- [ ] Tenant/scope ID on every document and as first query clause
- [ ] `updated_at` set before every bulk-update
- [ ] Writes are bulk upserts from async consumers
- [ ] Pagination uses cursor (`next_id`), not `from`/`size` offset
- [ ] Total count via separate `count` call
- [ ] Index alias configured for zero-downtime reindexing
