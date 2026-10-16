# Go conventions

Standard for every Go change. Source skills: `.agents/skills/be-golang` (and its `references/`), `be-architect`, `be-td`, adapted by decision **D-27**
(2026-10-09). Read the skills in the main checkout (`.agents/skills/`) before a task; this file lists how they apply here and where we deviate.

## Layers inside a domain package

One package per domain (`auth`, `media`, `yearbook`, `notes`, ...), three layers in three files (split a file at about 500 lines):

| Layer (skill name) | File | Does | Must not |
|---|---|---|---|
| Controller | `handler.go` | decode and validate the request, call the service, map the result to the response envelope, register routes | contain business rules, touch SQL, import the store |
| Manager | `service.go` | business rules, defaults, limits, transactions that span stores, calls other domains through small interfaces **declared here** (consumer side) | import `net/http`, know about the envelope or cookies |
| Adapter | `store.go` (+ `s3.go`, `redis.go`) | SQL / object storage / Redis / HTTP clients, nothing else; returns domain types | contain business rules, read the clock, decide limits |

- Dependency direction: handler -> service -> store. A handler never calls a store.
- Wiring (constructors, no globals, no `init`) happens in `cmd/smemories-api/main.go`; constructors return concrete pointer types, consumers accept the small interface they need.
- Cross-domain calls go service -> interface of the other domain's service (e.g. `yearbook` deleting children), never into its store.
- Shared helpers: `internal/httpx` (HTTP plumbing), `internal/apperr`, `internal/ulid`, `internal/textx`. A helper has no business logic.

## Errors (`internal/apperr`)

The skill uses the private `go-common/errors`. We have our own equivalent with the same shape:

| Operation | Call |
|---|---|
| construct | `apperr.New(apperr.NotFound, "yearbook not found")` |
| wrap with context | `apperr.Wrap(apperr.Internal, err, "insert yearbook")` |
| set the code on a plain error | `apperr.WithCode(apperr.DB, err)` |
| read | `apperr.CodeOf(err)`; `errors.Is` / `errors.As` still work (`Unwrap`) |

- In application code (handlers, services, stores) `fmt.Errorf` and `errors.New` are not used; lint (`forbidigo`) enforces it. Allowed: `cmd/`, `internal/config`, test files and `*test` helper packages (the exclusion is in `.golangci.yml`). A sentinel for `errors.Is` is an `apperr.New` value (`redis.ErrUnavailable`, `storage.ErrNotFound`); a cause plus a sentinel uses a small type with `Unwrap() []error`.
- Business context is added when wrapping. Never swallow an error. `panic` only for programmer errors.
- Stores return `apperr` codes for what the caller must tell apart (`NotFound`, `Conflict`); the service adds business codes (`ERROR_LIMIT_REACHED`).
- One mapper in `internal/httpx` turns an `apperr` into HTTP status + envelope (`docs/api-contract.md`). Handlers do `httpx.Fail(w, r, err)`; no per-package `fail` switches. `Fail` also answers an error that wraps `context.Canceled` (the client went away) as `ERROR_CLIENT_CLOSED` 499, so no handler does that itself. `httpx.WriteError` is for middleware and router fallbacks that hold a status and a code name instead of an error; it writes the v2 envelope only.

## Code quality (code-quality.md)

- `gofmt`, `golangci-lint` clean before every PR; no unused code.
- **No magic numbers**: durations, limits, sizes are named constants (package-level, or `internal/consts` if shared) or config. `//nolint:mnd` is not allowed (crypto constants excepted). The `mnd` linter is on (T-072); its allowlist in `.golangci.yml` is `apperr.NewCode` (an HTTP status is the name of the code), the number 2 (halving and doubling), the `*test` helper packages and `internal/media/image.go` (JPEG/EXIF offsets and tags are the format's own numbers).
- Type inference: `x := 1`, not `var x int = 1` (exceptions: interfaces, nil slices/maps).
- Pointers by default: pointer receivers; `*Struct` parameters and returns for domain structs. Value types stay for tiny immutable values, scalars, maps/slices/funcs (so a page of books is `[]Yearbook`, and `Discard(...Media)` takes values); explain the exception in a short comment. Start-up settings structs (`Limits`, `GoogleConfig`, `S3Config`, ...) are passed by value on purpose and say so on the type.
- Interfaces are small and live on the consumer side; no interface for a single implementation unless a second consumer or a test seam needs it.
- `ctx context.Context` is the first parameter all the way handler -> service -> store; never stored in a struct. Every goroutine has a cancellation path and a bounded lifetime; fan-out uses bounded pools.
- Logging: `log/slog`, stable keys (`request_id`, `user_id`, `yearbook_id`, `reason`), `error` level for unexpected failures only, never passwords, tokens, email codes, or personal data.
- Optimise after measuring (`pprof`, benchmarks). Pre-size slices and maps when the size is known.

## Observability and reliability

- HTTP chain (outermost first): request id, access log, metrics (T-073), security headers, panic recovery, request timeout, body cap, client IP, mux. Metrics sit outside the recovery so a panic counts as a 500. Metric names and alarm use: [observability.md](observability.md).
- Graceful shutdown and `/healthz` + `/readyz` stay (they exist). Pool statistics are exported with the metrics.

## Tests

- Table-driven tests for logic; `httptest` + the real MySQL/Redis/MinIO stack for integration (`make test-integration`); `-race` for concurrent code; avoid mocks unless unavoidable.
- A refactor does not change behaviour: integration tests pass unchanged. The exception is E10, which changes the contract on purpose: tests change only for renamed paths, envelope, timestamps, codes and tables, and each such change is listed in the PR description.

## Specs (be-td)

New epic PRDs and task specs use the technical-design sections of the `be-td` skill where they apply: overview, numbered testable "SHALL" requirements, non-functional requirements (latency, throughput, consistency, security), data model, API contract (as in `docs/api-contract.md`), flows, test plan, open questions.

## Where we deliberately differ from the skills

| Skill says | We do | Why |
|---|---|---|
| GORM, tags, hooks | plain `database/sql` with hand-written SQL | L-11 |
| gin and `go-common/http_middleware` | stdlib `ServeMux` + our middleware in `internal/httpx` | L-01; `go-common` is a private library |
| proto/gRPC, protoc-gen-validate, openapiv2, `permission.auth` | hand-written `api/openapi.yaml` with `x-auth`, `x-error-codes`, a lint test; validation in Go | L-02; one browser client, no internal RPC |
| `go-common/errors` | `internal/apperr`, same operations | library not available |
| `idgen` ids | ULID `public_id` | ordered, non-UUID, no extra dependency |
| ClickHouse, Elasticsearch, Kafka | not used | one MySQL, Redis for time-limited data (D-23) |
| cache reads fall back to the DB when Redis is down | sessions and email codes fail closed (503) | D-23: they have no DB copy |
| `WithGzipMiddleware` (decode gzip requests) | not used | browsers do not gzip request bodies; body cap protects us |
| `*Struct` for every struct | slices of structs and start-up settings structs stay values (see Code quality) | a `[]*T` page adds nil checks and allocations for no gain here |
| soft deletes | hard delete on user request | privacy (D-26) |
| unix-ms timestamps | `DATE` for a birthday | a calendar date is not an instant |
