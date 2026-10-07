# T-001 — Repo foundation and API skeleton

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M0 · **Risk:** low · **Priority:** P1 · **Type:** infra

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Description
Replace the GoLand "hello world" stub with the skeleton every other task builds on: module path, directory layout, an HTTP server with shared middleware and the shared error envelope, env-based config, the OpenAPI/Postman starting points and the Makefile. Nothing product-specific yet. Decisions: board §4 D-05, L-01, L-04, L-07; conventions §5.

#### Scope
- In: rename the module; remove root `main.go`; `cmd/smemories-api`; `internal/config`; `internal/httpx` (router, middleware, error and JSON helpers); `GET /healthz`; `api/openapi.yaml` (health + error schema); `postman/platform.postman_collection.json`; `Makefile`; `.env.example`; `.editorconfig`; README "Development" section.
- Out (do not do): database (T-002), web app (T-003), CI (T-004), Dockerfile (M2), any auth or business endpoint, any third-party router/framework.

#### Acceptance criteria
- [ ] AC1 — `go.mod` module is `github.com/danyaa666/smemories` (keep the `go` directive); the root `main.go` stub is gone; `go build ./...` succeeds.
- [ ] AC2 — `make build`, `make test`, `make lint`, `make run` exist and pass from a clean checkout. `lint` = `gofmt -l .` must print nothing + `go vet ./...` (golangci-lint arrives in T-004).
- [ ] AC3 — `GET /healthz` → `200 {"status":"ok"}`. Unknown path → `404 not_found`; wrong method → `405 method_not_allowed` with an `Allow` header; both use the error envelope `{"error":{"code","message","request_id"}}`.
- [ ] AC4 — Each request writes one JSON slog line with `request_id, method, path, status, duration_ms` (never bodies, `Cookie`, or `Authorization`). Response carries `X-Request-Id`: a client value is reused only if it matches `^[A-Za-z0-9-]{8,64}$`, otherwise a new id is generated. A handler panic returns `500 internal_error` in the envelope, logs the stack, and the server keeps serving.
- [ ] AC5 — Config reads env vars with prefix `SMEM_`: `SMEM_HTTP_ADDR` (default `:8080`), `SMEM_ENV` (`dev|test|prod`, default `dev`), `SMEM_LOG_LEVEL` (`debug|info|warn|error`, default `info`). An invalid value makes the process exit non-zero with a message that names the variable. `.env.example` lists every variable with dev-only values; no secrets.
- [ ] AC6 — SIGTERM/SIGINT: stop accepting, drain in-flight requests for up to 10 s, exit 0. Server sets `ReadHeaderTimeout` 5 s, `ReadTimeout` 15 s, `WriteTimeout` 30 s, `IdleTimeout` 60 s; request bodies are capped at 1 MiB by default (`413 payload_too_large`), overridable per route.
- [ ] AC7 — Every response carries `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`, `Cache-Control: no-store`.
- [ ] AC8 — `api/openapi.yaml` (OpenAPI 3.1) documents `/healthz` and the `Error` schema; `postman/platform.postman_collection.json` covers `/healthz`, the 404 and the 405 cases with assertions.
- [ ] AC9 — README gains a "Development" section: prerequisites (Go version from `go.mod`), the make targets, how to run the API locally.

#### Design
Files: `go.mod`, `Makefile`, `.env.example`, `.editorconfig`, `cmd/smemories-api/main.go`, `internal/config/config.go`, `internal/httpx/{router,middleware,errors,respond}.go`, `api/openapi.yaml`, `postman/platform.postman_collection.json`, tests next to the code.

Helpers every later task reuses: `httpx.WriteJSON(w, status, v)` and `httpx.WriteError(w, r, status, code, msg)`. Domain packages later expose `Routes(mux *http.ServeMux)`; `main` only wires config → router → server. Use Go 1.22+ method patterns (`mux.Handle("GET /healthz", ...)`). Infra routes (`/healthz`, `/readyz`) live at the root; business routes will live under `/v1/` (L-07).

```mermaid
flowchart LR
    R[request] --> A[request id] --> B[recover] --> C[access log] --> D[security headers] --> E[ServeMux] --> F[handler]
```

#### Risk
`low`

#### Security & performance notes
No secrets in the repo (public). Do not log bodies, cookies or authorization headers. Timeouts and the body cap are slow-client and large-body protection. Do not trust `X-Request-Id` blindly (log injection).

#### Test plan
- Dev: table tests for config parsing (valid, invalid, defaults); `httptest` tests for AC3/AC4/AC6/AC7 (404/405 envelope, request-id reuse and regeneration, panic recovery, headers, body cap); a test that starts the server on `:0` and shuts it down on context cancel.
- QA should probe: `X-Request-Id` containing CR/LF or 10 kB; 2 MiB POST body; `kill -TERM` while a slow request is in flight; run `make` targets from a fresh clone; run the Postman collection twice back to back.
