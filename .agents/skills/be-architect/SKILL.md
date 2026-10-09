---
name: be-architect
description: >
  System architecture decision guide. Use when choosing data stores, designing data flows,
  or planning infrastructure for new features. Covers layer responsibilities, database selection
  matrix, canonical data-flow patterns, and anti-patterns.
  Delegates to: @be-rldb (schema), @be-clickhouse (analytics), @be-es (search).
---

# Infrastructure Layers

| Layer | Technology | Role |
|---|---|---|
| Relational DB — master | MySQL / PostgreSQL | Config, CMS, domain master records |
| Relational DB — transactions | MySQL / PostgreSQL (sharded) | Mutable transactional records, source of truth |
| Analytics DB | ClickHouse / BigQuery | Aggregation, time-series, reporting |
| Search DB | Elasticsearch / OpenSearch | Filtered listings, full-text search, display |
| Cache | Redis + in-memory | Read acceleration, ephemeral state, distributed locks |
| Message Bus | Kafka / Pub/Sub | Async fan-out, CDC, cross-service events |

---

# Layer Responsibilities

## 1. Master Data RDBMS
**Use:** Domain config, CMS, operator settings, infrequently mutated records, relational joins between config entities.
**Not for:** Aggregation queries → Analytics DB. High-read listings → Search DB. Ephemeral state → Cache.

## 2. Transactional RDBMS
**Use:** Source-of-truth writes for financial transactions, user activity, promotions, rewards. Must be consistent, auditable, rollback-able.
**Not for:** Analytics on millions of rows → Analytics DB. Filtered search → Search DB.

**Rule:** Every transactional write needing analytics or display MUST publish to Message Bus → consumed into Analytics/Search DB.

## 3. Analytics DB (ClickHouse)
**Use:** `GROUP BY` + `SUM`/`COUNT`/`uniq()` over millions of rows, time-series cohort reports, append-only event facts.
**Not for:** Point lookups → RDBMS/Search. Paginated lists → Search DB. Mutable records → RDBMS. Full-text search → Search DB.

**Key signal:** Query uses `GROUP BY` + aggregates on time range over large dataset → Analytics DB.

**Best practices (industry):**
- Apply **materialized views** for pre-aggregation of frequent report queries — reduces read-time compute.
- Use **TTL** clauses to auto-expire old granular data while keeping aggregated summaries.
- Leverage **projections** as lightweight secondary sort orders without full materialized views.

## 4. Search DB (Elasticsearch)
**Use:** User-facing paginated lists with multi-field filters, admin filtered/sorted lists, full-text search with fuzzy matching.
**Not for:** Aggregation reporting → Analytics DB. Source-of-truth writes → RDBMS. Ephemeral state → Cache.

**Key signal:** "Paginated list of X filtered by Y, sorted by created_at" → Search DB.

**Best practices (industry):**
- Use **index aliases** for zero-downtime reindexing and blue-green deployments.
- Apply **Index Lifecycle Management (ILM)** to auto-roll and manage retention policies.
- Prefer `search_after` over `scroll` API for stateless deep pagination in modern ES versions.

## 5. Cache (Redis)
**Use:** TTL-cached expensive results (leaderboards, homepage data), OTP/rate-limiting, session tokens, distributed locks, pub/sub for ephemeral events.
**Not for:** Durable business data → RDBMS. Analytics → Analytics DB. Search → Search DB.

**In-memory (process-local):** Only for rarely-changing config (game list, provider list). Never for user-specific or cross-instance state.

**Best practices (industry):**
- Use **cache-aside** pattern with explicit TTL — never cache without expiry.
- Apply **circuit breaker** on cache reads — fall back to DB on Redis failure; never let cache outage cascade.
- Use Redis **Cluster** or **Sentinel** for HA; avoid single-node in production.

## 6. Message Bus (Kafka)
**Use:** Fan-out single event to multiple consumers, decouple writes from read-store population, cross-service CDC, background computation.
**Not for:** Request/response → sync RPC. Durable storage → RDBMS. Synchronous caller-needs-result flows.

**Best practices (industry):**
- Design **idempotent consumers** — assume at-least-once delivery.
- Use **dead-letter queues (DLQ)** for poison messages that fail after max retries.
- Prefer **entity-ID partitioning** for ordering guarantees per entity; random for low-volume topics.

---

# Database Selection Decision Matrix

```
Write that must be immediately consistent and auditable?
  YES → Transactional RDBMS (or Master Data RDBMS if config)

Domain config / operator settings / CMS?
  YES → Master Data RDBMS

Aggregates millions of rows (SUM, COUNT, GROUP BY, cohort)?
  YES → Analytics DB

Paginated list / filtered display for UI?
  YES → Search DB

Full-text search (FAQ, blog, titles)?
  YES → Search DB

Ephemeral / short-lived / read-acceleration of existing DB data?
  YES → Cache (Redis; in-memory for process-local static config)

One-to-many async fan-out from business event?
  YES → Message Bus

None match → Re-examine; most cases fall into one above.
```

---

# Canonical Data Flow Patterns

## Pattern 1 — Transactional Write → Analytics + Display
```
API → Transactional RDBMS (source of truth)
       → Message Bus Producer
            → Analytics DB Consumer
            → Search DB Consumer
            → Business Logic Consumer (quest, rebate)
            → Notification Consumer
```
**Applies to:** financial transactions, user events, rewards, promotions, checkins.

## Pattern 2 — Config Read with Cache
```
API → Cache (hit → return) → miss → Master Data RDBMS → populate cache with TTL → return
```
Always specify explicit TTL. Use helper (e.g., `cache.WithCache()`).

## Pattern 3 — Process-Local Config Cache
```
API → In-memory cache (hit → return) → miss → RDBMS → populate → return
```
Single-node only. **Applies to:** game list, provider list, deploy-static config.

## Pattern 4 — Analytics Query (Direct)
```
API (admin/report) → Analytics DB
```
No cache unless same aggregation shown on high-traffic page (then Redis + TTL).

## Pattern 5 — Search / List Query (Direct)
```
API → Search DB
```
Always scope by `tenant_id`/`agent_id`. Use cursor (`next_id`) pagination. Separate `count()` for totals.

---

# New Feature Design Checklist

1. **Source of truth?** Money/promotions/user state → Transactional RDBMS. Config/CMS → Master Data RDBMS.
2. **Who else needs this data?** Reporting → Message Bus → Analytics DB. Listing/display → Message Bus → Search DB. Multiple consumers → single topic, fan-out.
3. **Read patterns?** Aggregate → Analytics DB. Paginated list → Search DB. Repeated config → Cache. Static master data → in-memory.
4. **Need a consumer?** Yes if data must appear in Analytics/Search DB. Partition by entity-ID for high-volume.
5. **Migrations?** RDBMS: `YYYYMMDDHHII_<desc>.(up|down).sql`. ClickHouse: `YYYYMMDDHHII_<desc>.sql` (local + distributed). ES: `<index_name>.json`.

---

# Anti-Patterns

| Anti-pattern | Why | Correct |
|---|---|---|
| `GROUP BY`/`SUM`/`COUNT` on transactional RDBMS | Table-scan kills DB at scale | Analytics DB |
| RDBMS for user-facing paginated lists | Slow, no full-text, hard multi-filter | Search DB |
| Derived data in RDBMS as primary store | Sync issues, duplicated logic | Search/Analytics DB fed via Message Bus |
| Direct write to Analytics/Search DB from API | Bypasses source of truth | RDBMS first → fan-out via Message Bus |
| Search DB as source of truth for mutable records | No ACID | RDBMS is always source of truth |
| In-memory cache across multiple instances | Per-process inconsistency | Redis for shared state |
| `from`/`size` offset pagination in ES beyond page 10 | `max_result_window` exceeded | Cursor (`next_id`) pagination |
| `dynamic: true` in ES mappings | Uncontrolled schema growth | `dynamic: false` always |
| Analytics query without tenant filter | Full distributed scan | Always `WHERE tenant_id/agent_id` first |
| N+1 DB queries in loops | Latency spikes | Batch `IN (...)` or prefetch |

---

# Skill References

| Component | Skill |
|---|---|
| Relational DB schema | `@be-rldb` |
| ClickHouse tables/queries | `@be-clickhouse` |
| Elasticsearch indexes/queries | `@be-es` |

