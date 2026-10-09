# Audit: code against `.agents/skills` (2026-10-09, leader, read-only)

Method: read all seven skills (be-golang with its seven references, be-api-design, be-rldb, be-architect, be-td; be-clickhouse and be-es do not apply),
then read the router, middleware, error helpers, yearbook/notes handlers and stores, `cmd/smemories-api/main.go`, all nine migrations and the route table, and counted patterns with grep.
~7,500 non-test Go lines in 14 packages, 9 tables, 25 routes.

## What already follows the skills

- Graceful shutdown with drain, `/healthz` and `/readyz` (be-golang "Service Reliability").
- `log/slog` structured logs with a request id; no `log.Fatal`; no secrets in logs by design.
- `ctx` is passed down and never stored (0 structs hold a context); both goroutines have a lifecycle (`Serve`, `RunCleanup(ctx)`).
- Pointer receivers: 185 vs 8 value receivers (all small error or value types: allowed exception).
- No redundant `var x T =` declarations; interfaces are small and declared by the consumer (`Purger`, `Mailer`, `Storage`, `Pinger`).
- Primary keys are `BIGINT UNSIGNED AUTO_INCREMENT`; public ids are ordered unique ids (ULID) returned as strings; Down blocks exist in every migration; goose is the tool.
- Integration tests use `httptest` and the real stack; `-race` runs in `make test`.
- Ephemeral state is in Redis (D-23), as be-architect prescribes.

## Gaps and where each is fixed

| # | Skill rule | Finding | Fix |
|---|---|---|---|
| A1 | be-golang: 3 layers, handlers coordinate only | `yearbook` and `notes` handlers call the store directly and hold rules (defaults, 20-book check path, cursor handling, purge ordering). `auth` and `media` have services. | T-068 (notes), T-070 (yearbook) |
| A2 | be-golang: errors via one library, no `fmt.Errorf` / stdlib `errors.New` | 54 `fmt.Errorf` and 35 `errors.New` in non-test code; every package has its own `ValidationError` and `fail` switch. | T-063 (apperr + mapper), adopted in T-068..T-071, `forbidigo` in T-072 |
| A3 | be-golang: no magic numbers, `mnd` | inline durations and limits (`5*time.Second`, server timeouts, `loginWindow`), no `mnd` linter; golangci runs the standard set + bodyclose + gosec only. | T-072 |
| A4 | be-golang: `*Struct` parameters and returns | stores take and return `Yearbook`, `Profile` by value. | T-064..T-067 as each store is rewritten, T-072 sweep |
| A5 | be-golang: required middleware | missing: request timeout, metrics; client IP lives inside `auth` instead of a shared middleware; (gzip: not needed, recorded). | T-063 (timeout, IP), T-073 (metrics, needs Q-018) |
| A6 | be-golang data-store: avoid joins, no N+1 | yearbook reads join `profiles` and two `media` rows per query. | T-064 |
| B1 | be-api-design: paths `/api/<ns>/<action>`, GET/POST only | REST `/v1/...` with PATCH/PUT/DELETE and path parameters; the edge strips `/api`. | D-25; T-068..T-071 |
| B2 | envelope `{status,data}` / `{status,error_message}` | `{"error":{"code","message","request_id"}}`, bare payloads. | T-063 (helpers), per domain tasks |
| B3 | error codes SCREAMING_SNAKE, globals not documented | lower snake codes. | per domain tasks, mapping in `docs/api-contract.md` |
| B4 | timestamps Unix ms int64 | RFC 3339 strings. | per domain tasks |
| B5 | keyset pagination `limit` + `next_id` | `limit` + `cursor` / `next_cursor`, list keys named by resource. | per domain tasks |
| B6 | contract documented per endpoint (purpose, auth, field tables, business errors, example) | `openapi.yaml` has 6 `example` entries for about 25 operations, no auth marker, no per-operation error list. | T-063 (lint test), enforced per domain |
| C1 | be-rldb: `_tab` suffix | none of the 9 tables. | T-064..T-067 |
| C2 | BIGINT Unix-ms `created_at`/`updated_at` on every table | 16 `DATETIME(6)` columns; `profiles`, `media`, `user_identities`, `email_tokens`, `note_collections` lack `updated_at`. | T-064..T-067 |
| C3 | no DB foreign keys | 11 FKs with `ON DELETE CASCADE` / `SET NULL`. | T-064..T-067 (deletes move into Go, zero-rows tests) |
| C4 | ENUM not in the template; app-level integrity | 7 ENUM columns (`locale`, `language`, `page_size`, `purpose`, `provider`, `uploader_kind`). | T-064..T-067 |
| C5 | index naming `idx_` | `ix_` / `uq_` names. | T-064..T-067 (`uq_` kept for unique) |
| C6 | pool: max idle = max open | defaults 20 / 5. | T-072 |
| C7 | migration file naming `{YYYYMMDDHHMMSS}_{desc}.sql` | `0001`..`0009`. | new files use timestamps; old stay (renaming breaks goose history) |

## Conflicts with earlier decisions (resolved by the owner on 2026-10-09 unless noted)

| Earlier decision | Skill rule | Outcome |
|---|---|---|
| L-07 REST `/v1`, edge strips `/api` | `/api/<ns>/<action>` | skill wins: D-25; L-07 superseded |
| L-05 `DATETIME(6)` timestamps, ULID public ids | BIGINT ms; idgen ids | timestamps: skill wins (D-26); ULID stays as our ordered id |
| L-11 plain `database/sql` | GORM | **L-11 stands** (not asked: the skill rules that matter are independent of the ORM); say so if you want GORM |
| L-01 stdlib `ServeMux` | gin | stands (see go-conventions) |
| §5 error envelope `{"error":{...}}` | `{status,error_message}` | skill wins: D-25 |
| D-23 fail-closed sessions | be-architect: cache falls back to DB | D-23 stands (sessions have no DB copy) |
| README privacy/hard delete | be-rldb soft deletes | hard delete stays (D-26); `deleted_at` only if a feature needs it |

## Not applicable

be-clickhouse, be-es, Kafka patterns, proto/gRPC rules, GORM tags and hooks, `go-common` library names (private; replaced by `internal/apperr` and `internal/httpx`).
