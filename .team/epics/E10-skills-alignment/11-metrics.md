# E10_T-073 — Observability: request metrics middleware and /metrics endpoint

**Epic:** E10-skills-alignment · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** infra
**Depends on:** T-063. **Q-018 answered 2026-10-09: the owner approved `github.com/prometheus/client_golang`** (new core dependency, approved).

#### Description
`be-golang` requires request metrics (duration, status code) and metrics at critical boundaries. We have an access log but no metrics. Library: `github.com/prometheus/client_golang`, text exposition on a separate internal listener.

#### Requirements (SHALL)
1. A middleware records, per matched route pattern (never the raw path): request count and duration histogram by method and status class, in-flight gauge.
2. DB pool statistics (`sql.DBStats`), Redis pool statistics, upload concurrency in use (T-036) and rate-limit rejections are exported.
3. The endpoint is served on a separate address (`SMEM_METRICS_ADDR`, default off) and never on the public listener; no label contains a user id, email, token or other unbounded value.
4. A load-free test scrapes it and checks that route labels are patterns.

#### Acceptance criteria
- [ ] AC1..AC4 — the four requirements, each with a test; `docs/observability.md` states the metric names and how T-024 (alarms) will use them.
