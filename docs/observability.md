# Observability: metrics (T-073)

Library: `github.com/prometheus/client_golang` (Q-018). Code: `internal/metrics`, the `httpx.Instrument` middleware, wiring in `cmd/smemories-api/main.go`.

## Exposure

`SMEM_METRICS_ADDR` (host:port, default empty = off) starts a second listener that serves only `GET /metrics` (Prometheus text format). It must differ from `SMEM_HTTP_ADDR`;
the public mux has no `/metrics` route (tested). Bind it to a private interface or a security-group-restricted port; there is no authentication on it.

## Label rules

Every label comes from a small fixed set: `route` is the mux pattern (`GET /api/note-collection/get-list`, `unmatched` when nothing matched), never the raw path or query;
`method` is one of the standard methods or `OTHER`; `code_class` is `1xx`..`5xx`; `limiter` is a constant name in code. No user id, email, token, IP or other unbounded value may become a label.

## Metrics

| Name | Type | Labels | Meaning |
|---|---|---|---|
| `smem_http_request_duration_seconds` | histogram | method, route, code_class | request duration; `_count` is the request count; includes timeouts (503) and recovered panics (500) |
| `smem_http_requests_in_flight` | gauge | | requests being served |
| `smem_ratelimit_rejected_total` | counter | limiter | hits refused by a rate limiter (limiter failures are not counted, they are logged) |
| `smem_mail_send_failures_total` | counter | class | emails given up on after the retries (`unavailable`, `rejected`, `config`, `shutdown`, `other`); background sends only; `config` is an account or setup problem only the operator can fix and must page, `shutdown` counts emails dropped when the process had to exit ([email.md](email.md)) |
| `smem_media_processing_slots_in_use` / `smem_media_processing_slots` | gauge | | images being processed / `SMEM_MEDIA_MAX_CONCURRENT` |
| `smem_media_uploads_in_progress` | gauge | | owner uploads (`media/create`) holding a shared slot; the pool is 4 x `SMEM_MEDIA_MAX_CONCURRENT` (T-102) |
| `smem_media_process_panics_total` | counter | | panics recovered while decoding, scaling or encoding a photo (T-104); each one answered `400 ERROR_INVALID_IMAGE`, freed its processing slot and logged one ERROR line "image processing panicked" (no image bytes). Any increase is a decoder bug or a crafted file: alert on `increase(...[1h]) > 0` |
| `smem_public_uploads_in_progress` / `smem_public_uploads_max` | gauge | | public note submissions in progress / `SMEM_PUBLIC_UPLOAD_MAX_CONNS` |
| `smem_public_media_requests_total` | counter | result | public photo link requests (`ok`, `not_found`, `rate_limited`; T-092); a spike of `not_found` is someone guessing tokens, and the three values are the only labels, never a token or id |
| `go_sql_*` (`db_name="smemories"`) | gauge/counter | db_name | `sql.DBStats`: open, in-use and idle connections, wait count and duration |
| `smem_redis_pool_connections`, `_idle_connections` | gauge | | go-redis pool size |
| `smem_redis_pool_hits_total`, `_misses_total`, `_timeouts_total` | counter | | go-redis pool use |
| `go_*`, `process_*` | | | Go runtime and process collectors |

## How T-024 (alarms) uses them

- Error rate: `sum(rate(smem_http_request_duration_seconds_count{code_class="5xx"}[5m])) / sum(rate(smem_http_request_duration_seconds_count[5m]))` above a threshold.
- Latency: `histogram_quantile(0.95, sum by (le, route) (rate(smem_http_request_duration_seconds_bucket[5m])))` per route.
- Saturation: `go_sql_in_use_connections / go_sql_max_open_connections` near 1, `rate(go_sql_wait_count_total[5m]) > 0` for a while, `rate(smem_redis_pool_timeouts_total[5m]) > 0`,
  `smem_media_processing_slots_in_use == smem_media_processing_slots` and `smem_public_uploads_in_progress` near `smem_public_uploads_max` for several minutes (users are being turned away with 503).
- Abuse or a bad limit: a jump in `smem_ratelimit_rejected_total` by limiter.
