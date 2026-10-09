---
name: be-golang
description: Go (Golang) backend coding skill. Use for ALL code changes — new features, bug fixes, modifications, refactoring, and performance improvements. Covers 3-layer architecture (Controller/Manager/Adapter), DI, error handling, GORM, logging, and code quality. Delegates to @be-rldb (schema), @be-clickhouse (analytics), @be-es (search), @be-api-design (API contracts).
---

# Go Backend Development Skill

## Workflow

Run the **pre-task checklist** before any work:
1. **Confirm scope** — restate the requirement; ask if ambiguous.
2. **Identify affected modules** — list all modules to create or modify.
3. **Read existing code** — find patterns, conventions, existing helpers; do not duplicate.
4. **Map impact** — list files to create/modify; trace callers of any shared method being changed.

Then follow the workflow for the task type:

| Task | Steps |
|---|---|
| **New Feature** | Pre-task → DB migration (if needed) → proto/API contract → Adapter → Manager → Controller → integration tests |
| **Bug Fix** | Pre-task → reproduce → write failing integration test → root-cause → minimal fix → check similar patterns → verify all tests pass |
| **Modify** | Pre-task → read impl + tests → list affected features → modify code + tests → update call sites → verify all tests pass |
| **Refactor** | Pre-task → read impl + tests → identify target pattern → list affected modules → apply pattern (no behavior change, do not touch integration tests) → verify all tests pass |
| **Performance** | Pre-task → read impl + tests → identify bottleneck → apply optimization (do not touch integration tests) → verify all tests pass |

> Full workflow details: [references/workflow.md](references/workflow.md)

---

## Project Structure

3-layer architecture: `Controller → Modules Manager → Adapter`

| Layer | Responsibility | Reference |
|---|---|---|
| **Controller** | Validate request, parse input, coordinate Manager calls, format response | [architecture-controller.md](references/architecture-controller.md) |
| **Modules Manager** | Domain business logic; framework-agnostic; exposes reusable interfaces independent of request/response formats | [architecture-modules-manager.md](references/architecture-modules-manager.md) |
| **Adapter** | Access databases, external clients, or any I/O interface; no business logic | [architecture-adapter.md](references/architecture-adapter.md) |

---

## Error Handling

Library: `github.com/whitelabel6688/go-common/errors`

| Operation | Code |
|---|---|
| Construct | `errors.NewError(errors.ErrorNotFound, "msg")` |
| Wrap | `errors.WrapError(errors.ErrorInternal, err, "context")` |
| Map code | `errors.WithErrorCode(errors.ErrorDb, err)` |

**Rules:**
- **Prohibited:** `fmt.Errorf`, stdlib `errors.New` for application logic.
- Add business context when wrapping errors.
- Reserve `panic` for programmer errors only.
- Handle errors at the right boundary; never silently ignore.

> API envelope format → `@be-api-design`

---

## Data Store Integration

> Schema design delegated to specialized skills. Go-side conventions only.

| Store | Go Convention | Schema Skill |
|---|---|---|
| **GORM (Relational DB)** | Use proper GORM tags; batch writes (2000–5000/batch); no DB joins in loops; keep transactions short | `@be-rldb` |
| **ClickHouse** | Client: `clickhouse-go/v2`; always batch via `PrepareBatch` → `AppendStruct` → `Send`; named const queries | `@be-clickhouse` |
| **Elasticsearch** | Client: `go-elasticsearch/v8/typedapi` (typed API only) | `@be-es` |

> Full details: [references/data-store.md](references/data-store.md)

---

## Observability (Logging & Metrics)

Use `log/slog` structured logging:

```go
logger.Error("failed to update order",
    slog.Int64("order_id", orderID),
    slog.String("reason", err.Error()),
)
```

**Rules:**
- `error` level only for unexpected/critical failures.
- No `log.Fatal` in business logic.
- Never log passwords, API keys, tokens, or PII.
- Keep structured logs with stable keys.
- Add metrics and tracing at critical boundaries.

---

## Code Quality

> Full details: [references/code-quality.md](references/code-quality.md)

Key rules:
- Follow [Effective Go](https://go.dev/doc/effective_go); small, focused, readable functions.
- Run `gofmt` and `golangci-lint` before completion.
- No unused code, no magic numbers, no hardcoded config values.
- Use type inference (`a := 1`); avoid redundant explicit type annotations.
- Use proto-generated enums; do not declare parallel Go `iota` enums.
- Default to pointer receivers and `*Struct` params/returns.
- Pass `ctx context.Context` through controller → manager → adapter; never store in structs.
- Goroutines must have bounded lifecycle and cancellation.

---

## Proto — Go Implementation

> Contract design → `@be-api-design` | Full details: [references/proto.md](references/proto.md)

**Rules:**
- Every RPC declares `(permission.auth)` and `(apidoc.errors)`.
- Call `req.Validate()` at the top of every RPC handler; return `errors.ErrorParam` on failure.
- Do **not** duplicate proto-level validation in business logic.
- Keyset pagination: `limit` + `next_id` only — no offset pagination.
- `int64`/`uint64` IDs **must** use `json:",string"` tag (prevents JavaScript precision loss).
- Use `jstype = JS_STRING` for proto `int64` fields served over HTTP/JSON via grpc-gateway.

---

## Service Reliability

- Implement graceful shutdown for all services and workers.
- Expose health/readiness checks for long-running services.
