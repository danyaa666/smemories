# E10 — Align the codebase with the engineering skills (`.agents/skills`)

**Milestone:** M1 (runs alongside the notes and print work; blocks only the tasks listed under "Ordering")
**Decisions:** D-25 (API contract v2), D-26 (database conventions), D-27 (layering, errors, quality; the skills are our standard)
**Standards written for dev and QA:** [api-contract.md](../../../docs/api-contract.md), [db-conventions.md](../../../docs/db-conventions.md), [go-conventions.md](../../../docs/go-conventions.md)
**Audit:** [AUDIT.md](AUDIT.md)

## Overview
The owner asked (2026-10-09) to follow the backend skills in `.agents/skills`, check the code against them and refactor. The audit found the Go
code mostly clean (small, well-tested, graceful shutdown) but different from the skills in four ways: the API contract (paths, envelope, codes,
timestamps, pagination, contract docs), the database schema (naming, timestamps, foreign keys, enums), the layering (yearbook and notes have no
service layer) and the error handling (stdlib errors, per-package mappers). The owner chose full alignment on API and database, three layers
inside each domain package. Nothing is in production, so this is the cheapest moment; after launch every path and column would be a migration.

## Requirements (SHALL)
1. Every client-facing endpoint SHALL follow `docs/api-contract.md`: `/api/<namespace>/<action>`, GET/POST, envelope `{status,data}` / `{status,error_message,request_id}`, `ERROR_*` codes, Unix-ms timestamps, `limit`/`next_id`.
2. `api/openapi.yaml` SHALL document every operation with purpose, `x-auth`, field descriptions, `x-error-codes` and examples; a Go test SHALL fail when one is missing.
3. Every table SHALL follow `docs/db-conventions.md` (`_tab`, BIGINT ms timestamps, no FKs, no ENUM, indexed reference columns); deleting a parent SHALL remove every child in one transaction, proven by a zero-rows test.
4. Each domain package SHALL separate handler, service and store; handlers SHALL NOT call stores.
5. Application code SHALL use `internal/apperr` and one HTTP mapper; `fmt.Errorf` and `errors.New` are forbidden outside the allow-list by lint.
6. User-visible behaviour SHALL NOT change except the contract itself; the web app SHALL keep working after every merged task.

## Non-functional
| Concern | Requirement |
|---|---|
| Safety | every migration has a tested Down; conversion tests seed old-shape rows and compare after Up and after Down |
| Availability of develop | each task leaves develop green and the web app working (path-based envelope switch lets domains move one at a time) |
| Performance | no extra queries per request beyond the documented constant (parent + one `IN` per relation); no N+1 |
| Security | CSRF guard, session scoping, owner scoping, rate limits and redaction unchanged; tokens never in logs |

## Design: how domains move one at a time
- `httpx.WriteError` and the new `httpx.OK`/`httpx.Fail` choose the envelope by path: `/api/...` gets the v2 envelope, `/v1/...` the old one. The `RequireUser` middleware, 404/405 fallbacks, body-limit and panic handler therefore answer correctly for both generations.
- The web client reads both shapes and both code spellings during the transition (T-063); T-071 removes the old path and shim, T-072 removes the switch.
- Edge: today the browser calls `/api/v1/...` and the edge strips `/api`. During the transition the edge rewrites only the `/api/v1` prefix to `/v1` and passes every other `/api/...` path through unchanged (one rule in the Vite dev proxy, the same rule in CloudFront later). T-063 changes the dev proxy; T-071 removes the rewrite.
- Order inside a domain: database first (store + migration), API second (handler + service + openapi + web).

## Ordering (one lane; the dev works two at a time, QA behind)
```
T-063 foundation ──┐
T-034, T-057 (in flight) ─> T-064 DB yearbook ─> T-065 DB media ─> T-066 DB notes ─> T-067 DB auth (also after T-048, T-052)
                                              T-066 ─> T-068 API notes ─> T-069 API media ─> T-070 API yearbook ─> T-071 API auth (also after T-053) ─> T-072 sweep
T-073 metrics (waits for Q-018)
```
Feature tasks that add endpoints (T-013 moderation, T-056 book data) are written in v2 from the start and wait for T-068; T-018 (public note form) waits for T-068 too so it is built once.
Already queued work that lands before the lane (T-034, T-048, T-052, T-053, T-057, T-054) keeps its v1 specs; the lane converts what they add.

## Owner actions
1. Google Cloud console: add the new redirect URI `<PUBLIC_BASE_URL>/api/auth/google-callback` before T-071 is deployed anywhere real (local Google sign-in is optional).
2. Approve the high-risk merges (T-064..T-069, T-071). Each is a migration or a public-API change.
3. Answer Q-018 (metrics library) when convenient; only T-073 waits.
4. Decide whether `.agents/` is committed to `develop` (recommended: it is the standard the team now follows; dev and QA read it from the main checkout either way).

## Risks
- Large mechanical diffs (every handler, store, test, the web client). Mitigation: one domain per task, DB and API separated, QA probes listed per task.
- Dropping foreign keys can leave orphans after a partial failure. Mitigation: parent and children are deleted in one transaction; zero-rows test per domain.
- Feature work (notes moderation, book data, print) waits behind T-068. Mitigation: T-063/T-064..T-068 are first in the queue; T-054, T-044, T-050, T-062 do not depend on E10.
- Time-zone slip when converting DATETIME to BIGINT. Mitigation: `TIMESTAMPDIFF(MICROSECOND, ...)` conversion, round-trip test on rows around DST boundaries and the year 2038.

## Out of scope
GORM, gin, protobuf/gRPC, `go-common`, ClickHouse, Elasticsearch, Kafka, soft deletes (no feature needs them), renaming existing migration files, changing the public id format.
