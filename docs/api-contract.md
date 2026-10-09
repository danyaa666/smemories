# HTTP API contract (v2)

Standard for every client-facing endpoint. Source skill: `.agents/skills/be-api-design`, adapted by decision **D-25** (2026-10-09).
Applies to every new endpoint now. Existing `/v1/...` endpoints are moved domain by domain (epic E10); until a domain is moved, its
`/v1` contract in `api/openapi.yaml` (flat error envelope `{"error":{...}}`, REST verbs, ISO timestamps) stays valid.

## 1. Paths and methods

- Business endpoints: `/api/<namespace>/<action>` or `/api/<namespace>/<action>-<resource>`. kebab-case, **singular** namespaces, flat (no deep nesting, no path parameters).
- `GET` reads, `POST` changes anything. No `PUT`, `PATCH` or `DELETE`.
- Verbs: `get`, `get-list`, `create`, `update`, `delete`, `verify`, `setup`; batch: `batch-<action>`. Domain verbs are fine when natural (`login`, `register`, `revoke`, `submit-note`).
- Inputs: `GET` takes query parameters; `POST` takes a JSON body (or `multipart/form-data` for uploads). Identifiers that used to be path parameters become fields (`id`, `yearbook_id`).
- Infrastructure probes `/healthz` and `/readyz` stay at the root and outside this contract (no envelope; they are for load balancers).
- The web app and the API share one origin; the edge forwards `/api/*` **unchanged** (it no longer strips `/api`; supersedes the strip rule in L-07).

## 2. Envelope

Success (HTTP 200):

```json
{ "status": "OK", "data": { } }
```

Error:

```json
{ "status": "ERROR_INVALID_TITLE", "error_message": "title must be 1-120 characters", "request_id": "01J..." }
```

- `status` is always a string: `"OK"` or an error code (`ERROR_` + SCREAMING_SNAKE_CASE). The error code is the contract; `error_message` is English text for logs, the client localises.
- `request_id` is our addition (support and log correlation). It is also the `X-Request-Id` response header.
- **HTTP status is kept as well** (deviation from the skill, which is silent): 200 success (also for what used to be 201/204; an empty result is `"data": {}`), 400 invalid input, 401 not signed in, 403 forbidden, 404 not found, 409 conflict, 413 too large, 429 rate limited (with `Retry-After`), 503 dependency down, 500 internal. Browsers, proxies and the edge need real status codes.
- Binary responses (`media/get-content`) have no envelope on success; their errors do.

## 3. Error codes

- Global, never documented per endpoint: `ERROR_INTERNAL` (500), `ERROR_UNAUTHORIZED` (401), `ERROR_PARAM` (400, malformed body or query; unknown field; wrong type).
- Everything else is **business-specific** and listed per endpoint: `ERROR_NOT_FOUND`, `ERROR_FORBIDDEN`, `ERROR_RATE_LIMITED`, `ERROR_LIMIT_REACHED`, and field-level codes such as `ERROR_INVALID_TITLE` (the web shows a message under the field).
- Mapping from the old codes: `snake_case` becomes `ERROR_` + upper case (`invalid_title` -> `ERROR_INVALID_TITLE`, `internal_error` -> `ERROR_INTERNAL`, `invalid_body` / `unknown_field` -> `ERROR_PARAM`).
- Server code builds errors with `internal/apperr` (see `docs/go-conventions.md`); one mapper turns them into status + envelope.

## 4. Field rules

- **Timestamps**: Unix milliseconds as a JSON number (`int64`), named `*_at` (`created_at`, `deadline_at`). Unset optional timestamps are `null`. Always say "Unix ms" in the description.
- **Ids**: the public id of a row is its ULID string (`id`, `yearbook_id`, ...). Internal numeric ids never leave the server (L-05 stays; the ULID is our ordered unique id, no UUID). Any other `int64`/`uint64` that can exceed 2^53 is a string; small counters and sizes (`bytes`, `width`) stay numbers.
- **Calendar dates** that are not an instant (a birthday) are `"YYYY-MM-DD"` strings. This is the one documented exception to Unix ms.
- **Enums** list every valid value and its meaning in the description.
- Every field states required / optional and its default if the default changes behaviour.
- Optional text is an empty string when unset, not `null`, unless `null` carries meaning (documented).

## 5. Pagination (keyset)

Request: `limit` (int, default and maximum stated per endpoint) and `next_id` (string, omitted on the first page).
Response `data`: `{ "items": [ ... ], "next_id": "" }`. `next_id` is an opaque string; the empty string means no more pages. No offset pagination.

## 6. Authentication and CSRF

Each endpoint declares `x-auth`: `required` (session cookie), `public`, or `optional`. State-changing `POST`s keep the existing origin guard.
Public endpoints that take a credential in the URL (`token`) keep it out of logs: the access log records the route, never the query string.

## 7. Documentation: `api/openapi.yaml` is the contract

Every operation must contain (a Go test, introduced in T-063, fails the build otherwise):

1. `summary` (the purpose, one line) and `description` (who calls it, behaviour, edge cases, retry/polling notes).
2. `x-auth: required | public | optional`.
3. Request and response schemas where **every property has a `description`** that states type meaning, constraints, required/optional and units (timestamps: "Unix ms").
4. `x-error-codes`: the list of business-specific codes (`code` + `description`), empty list if none. Never list the global ones.
5. At least one happy-path `examples` entry for the request and the response.
6. Tags and `operationId` (the web types are generated from this file).

The Postman collection and `web/src/api/schema.d.ts` follow the file; the definition of done in README section 5 applies.

## 8. Route map: old to new

| Old (v1) | New (v2) | Notes |
|---|---|---|
| `POST /v1/auth/register` | `POST /api/auth/register` | |
| `POST /v1/auth/login` | `POST /api/auth/login` | |
| `POST /v1/auth/logout` | `POST /api/auth/logout` | |
| `POST /v1/auth/verify-email` | `POST /api/auth/verify-email` | T-048 changes this to a code check; same name |
| `POST /v1/auth/verify-email/resend` | `POST /api/auth/resend-verification` | |
| `POST /v1/auth/forgot-password` | `POST /api/auth/forgot-password` | |
| `POST /v1/auth/reset-password` | `POST /api/auth/reset-password` | |
| `GET /v1/auth/google/start` | `GET /api/auth/google-start` | |
| `GET /v1/auth/google/callback` | `GET /api/auth/google-callback` | the redirect URI registered at Google changes: owner action |
| `GET /v1/me` | `GET /api/user/get-me` | |
| `POST /v1/yearbooks` | `POST /api/yearbook/create` | |
| `GET /v1/yearbooks` | `GET /api/yearbook/get-list` | `limit`, `next_id` |
| `GET /v1/yearbooks/{id}` | `GET /api/yearbook/get?id=` | |
| `PATCH /v1/yearbooks/{id}` | `POST /api/yearbook/update` | body `id` + changed fields (partial) |
| `PUT /v1/yearbooks/{id}/profile` | `POST /api/yearbook/update-profile` | body `yearbook_id` + profile |
| `DELETE /v1/yearbooks/{id}` | `POST /api/yearbook/delete` | body `id` |
| `POST /v1/yearbooks/{id}/media` | `POST /api/media/create` | multipart, field `yearbook_id` |
| `GET /v1/yearbooks/{id}/media` | `GET /api/media/get-list?yearbook_id=` | |
| `GET /v1/media/{id}/content` | `GET /api/media/get-content?id=&variant=` | binary, no success envelope |
| `DELETE /v1/media/{id}` | `POST /api/media/delete` | body `id` |
| `POST /v1/yearbooks/{id}/collections` | `POST /api/note-collection/create` | body `yearbook_id` |
| `GET /v1/yearbooks/{id}/collections` | `GET /api/note-collection/get-list?yearbook_id=` | |
| `DELETE /v1/collections/{id}` | `POST /api/note-collection/revoke` | body `id` |
| `GET /v1/public/collect/{token}` | `GET /api/public-collect/get?token=` | no session |
| `POST /v1/public/collect/{token}/notes` (T-034) | `POST /api/public-collect/submit-note` | `token` in the body or multipart field |

New endpoints (moderation T-013, book data T-056, ...) are written in v2 from the start; pick the namespace by the same rule (`note`, `book-data`).
