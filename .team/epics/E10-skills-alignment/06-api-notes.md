# E10_T-068 — API v2: notes domain (collections, public lookup and submission) with a service layer

**Epic:** E10-skills-alignment · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high (public API) · **Priority:** P1 · **Type:** tech-debt
**Read first:** `docs/api-contract.md` (all), `docs/go-conventions.md`, `.agents/skills/be-api-design/SKILL.md`, `.agents/skills/be-golang/references/architecture-controller.md` and `architecture-modules-manager.md`, T-063 (helpers).
**Depends on:** T-063, T-066.

#### Description
First domain on the v2 contract. Move every notes endpoint (collection links, public lookup, public submission from T-034) to `/api/...`, add the missing service layer, use `apperr` and the v2 envelope, update openapi, Postman, the web calls and tests. Later feature tasks (T-013 moderation, T-018 public form, T-056) are built on this.

#### Scope
- In: `internal/notes` (handler/service/store split), routes, `api/openapi.yaml`, `postman/notes.postman_collection.json`, web code that calls these endpoints (the collection management calls that exist; T-018 does not exist yet), tests.
- Out: new endpoints (T-013), changes to token hashing, rate limits, upload caps or the field catalogue.

#### Requirements (SHALL)
1. Routes SHALL be exactly the v2 names in `docs/api-contract.md` section 8 (`POST /api/note-collection/create`, `GET /api/note-collection/get-list`, `POST /api/note-collection/revoke`, `GET /api/public-collect/get`, `POST /api/public-collect/submit-note`); the `/v1` ones SHALL be removed.
2. Handlers SHALL only decode, validate shape, call `notes.Service`, map to the response; they SHALL NOT import the store. `notes.Service` SHALL NOT import `net/http`.
3. Errors SHALL be `apperr` values; codes follow the mapping rule (`invalid_token` -> `ERROR_INVALID_TOKEN`, ...). The error table of each endpoint is written into `x-error-codes`.
4. Times in JSON SHALL be Unix ms numbers (`created_at`, `deadline_at`, `revoked_at`, note timestamps); lists SHALL be `{items, next_id}` where they paginate.
5. The public token SHALL never be logged: access log records the route only; add a test that a request with `?token=SECRET` leaves no `SECRET` in captured logs.

#### Acceptance criteria
- [ ] AC1 — Every operation passes the T-063 contract lint (summary, description incl. who calls it, `x-auth`, property descriptions, `x-error-codes`, examples).
- [ ] AC2 — Integration tests cover each endpoint with the new paths/envelope; assertions that changed are only path, envelope, code spelling and time format; the PR description lists them.
- [ ] AC3 — CSRF origin guard still protects every `POST` that uses the session cookie; the public endpoints still need no cookie and no CORS (test with and without `Origin`).
- [ ] AC4 — Web: API functions use the new paths via `request()`; screens and i18n keys unchanged (T-063 shim normalises codes); vitest updated.
- [ ] AC5 — Postman collection and `README`/docs paths updated; `api/openapi.yaml` regenerates `web/src/api/schema.d.ts` cleanly (`npm run gen` or the existing script) and `tsc` passes.
- [ ] AC6 — `notes.Service` has unit tests for its rules (revoked, expired deadline, wrong yearbook) using a fake store; handler tests do not duplicate them.

#### Risk
`high`: public, unauthenticated endpoints are changed. Owner approves.

#### Test plan
- QA should probe: old `/v1/public/collect/...` is gone (404 old envelope), token in query vs body, revoked/expired/unknown token give the same status and no timing difference beyond noise, 429 body and `Retry-After`, Unix-ms values round-trip as numbers (no float rounding), a very long label, OPTIONS/preflight behaviour unchanged.
