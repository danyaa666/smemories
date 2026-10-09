# E10_T-070 — API v2: yearbook domain with a service layer

**Epic:** E10-skills-alignment · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** tech-debt
**Read first:** `docs/api-contract.md`, `docs/go-conventions.md`, T-064 (the `Service` it started), T-068 (the pattern).
**Depends on:** T-064, T-069.

#### Description
Move yearbook and profile endpoints to `/api/yearbook/*`, finish `yearbook.Service` (defaults, limits, ULID creation, ownership scoping, cover/photo checks, delete orchestration) so the handler only decodes and maps, and switch to keyset pagination `limit` + `next_id`.

#### Requirements (SHALL)
1. Routes: `POST /api/yearbook/create`, `GET /api/yearbook/get-list`, `GET /api/yearbook/get?id=`, `POST /api/yearbook/update` (body `id` + any changed fields; absent field = unchanged, as PATCH today), `POST /api/yearbook/update-profile` (body `yearbook_id` + the full profile as PUT today), `POST /api/yearbook/delete`. `/v1/yearbooks...` removed.
2. List response `data`: `{"items":[yearbook...],"next_id":""}`; `next_id` is the existing opaque cursor (still not exposing internal ids); `limit` default 20, max 50.
3. Yearbook JSON: `created_at`/`updated_at` Unix ms; `graduation_year` number or `null`; `birthday` stays `"YYYY-MM-DD"`; ids are public ULIDs.
4. `yearbook.Service` SHALL hold every rule that is in `handler.go` today (page-size default `A5`, language and title required on create, 20-book limit, cursor encode/decode lives in the service or a `cursor.go`, delete = purge storage then transaction). `handler.go` SHALL not import the store and SHALL stay under 150 lines.
5. Field-level validation codes keep their meaning: `invalid_title` -> `ERROR_INVALID_TITLE`, etc.; all are listed in `x-error-codes` per endpoint with the field they belong to.

#### Acceptance criteria
- [ ] AC1 — Contract lint passes for the six operations; request/response examples include a Vietnamese title to document UTF-8.
- [ ] AC2 — Existing yearbook integration tests pass with only path/envelope/code/time/pagination-name changes (listed in the PR); service unit tests with a fake store cover limit, defaults, ownership, cover/photo checks.
- [ ] AC3 — Web: yearbook list/create/edit/profile screens use the new paths and `next_id`; no visible change; vitest updated.
- [ ] AC4 — Postman, openapi, README examples updated.
- [ ] AC5 — Query-count test from T-064 AC6 still passes through the HTTP layer.

#### Test plan
- QA should probe: update with an empty body (`ERROR_PARAM`), unknown field, other owner's book (404), 21st book (409 `ERROR_LIMIT_REACHED`), delete with storage down (502, book still there), pagination across equal `updated_at`, cursor tampering, 50-book list timing.
