# E06_T-024 — Observability: metrics, alarms, uptime check, log retention

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** low · **Priority:** P2 · **Type:** infra

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
RED metrics and DB/pool/export-queue gauges, CloudWatch alarms (5xx rate, p95 latency, export failures, RDS CPU/storage), external uptime check on /readyz, log retention and PII-free log review.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._

#### Leader note from decision D-23 (2026-10-08)
Add Redis alarms: memory use above 70%, evictions above zero (should be impossible with noeviction), rejected writes, connection count, replication lag if Multi-AZ, and a count of `503 session_store_unavailable` / `code_store_unavailable` / `limiter_unavailable` responses.

#### Leader note (2026-10-15, from T-073)
The API can serve Prometheus metrics on a second listener (`SMEM_METRICS_ADDR`, default off, see `docs/observability.md`). It has **no authentication by design**: in production bind it to a private interface or the task's loopback and scrape it from inside the VPC only; never expose it through the load balancer or CloudFront. The alarm queries are listed in `docs/observability.md`.

#### Leader note (2026-10-22, from T-091)
Alarm on the ERROR log line `mailer: send failed` with `class=config` (see `docs/email.md`). It covers background and request-path code-email failures caused by provider configuration, for example a revoked key or an unverified sender, which retrying cannot fix. Page on the first one, not on a rate.
