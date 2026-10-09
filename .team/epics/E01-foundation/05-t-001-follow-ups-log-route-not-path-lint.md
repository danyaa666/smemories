# E01_T-028 — T-001 follow-ups: log route not path, lint scope and findings, OpenAPI 404/405

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M0 · **Risk:** low · **Priority:** P2 · **Type:** tech-debt

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Description
Small follow-ups found in the T-001 review and QA. They should land before the CI task (T-004, which turns golangci-lint on) and before the notes-link API (T-012, whose collection token is a secret that appears in a URL path).

#### Scope
- In: access-log route pattern; `make lint` scope; the two golangci-lint findings; OpenAPI 404/405 documentation.
- Out (do not do): new endpoints, CI (T-004), changing the middleware order, new dependencies.

#### Acceptance criteria
- [ ] AC1 — The access log line carries `route` (the matched mux pattern, for example `GET /healthz`) and **no raw `path`**; an unmatched request logs `route` as `-`. A test registers `GET /v1/secret/{token}`, requests `/v1/secret/abc123secret`, and proves `abc123secret` appears nowhere in the log line.
- [ ] AC2 — `make lint` ignores hidden directories such as `.team/worktrees/*` (list files with `git ls-files '*.go'` or an equivalent) yet still fails with a non-zero exit and the file names when a tracked Go file is not gofmt-clean. Show both cases in the PR description.
- [ ] AC3 — `golangci-lint run` (v2, default linters) reports 0 issues, including `errcheck` at `internal/httpx/server_test.go:46` and `ST1023` at `internal/httpx/router.go:23`. The README "Development" section records the version used.
- [ ] AC4 — `api/openapi.yaml` documents the `404 not_found` and `405 method_not_allowed` responses (with the `Allow` header) for `/healthz` using the shared `Error` schema.
- [ ] AC5 — No other behaviour change: existing tests pass; the Postman collection passes twice back to back.

#### Design
Files: `internal/httpx/middleware.go`, `internal/httpx/router.go`, `internal/httpx/router_test.go`, `internal/httpx/server_test.go`, `Makefile`, `api/openapi.yaml`, `README.md`.

Gotcha for AC1: the mux sets `Request.Pattern` on the request value it dispatches, while `AccessLog` runs outside it and holds a different `*http.Request` (`RequestID` clones it with `WithContext`). Capture the pattern through a small holder stored in the request context (set by the router wrapper, read by `AccessLog` after `next` returns) instead of reading `r.Pattern` directly in the logger.

#### Risk
`low`

#### Security & performance notes
AC1 is a security fix in waiting: bearer tokens in URL paths must never reach logs. Keep logging the query string out as well (already the case).

#### Test plan
- Dev: the token-redaction test, a log-format test for matched and unmatched routes, a manual run of `make lint` in a checkout with an unformatted file under `.team/worktrees/`.
- QA should probe: 404/405/panic requests still log exactly one line with the right `route`; a path containing `%2F` or `..`; run Newman twice.
