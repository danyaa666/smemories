# HTTP API contract (v2)

Standard for every client-facing endpoint. Source skill: `.agents/skills/be-api-design`, adapted by decision **D-25** (2026-10-09).
Applies to every endpoint. The `/v1/...` endpoints were moved domain by domain (epic E10) and no `/v1` route exists any more; section 8 maps old to new.

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
- Binary responses (`media/get-content`, `public-media/get-content`) have no envelope on success; their errors do.

## 3. Error codes

- Global, never documented per endpoint: `ERROR_INTERNAL` (500), `ERROR_UNAUTHORIZED` (401), `ERROR_PARAM` (400, malformed body or query; unknown field; wrong type).
- `ERROR_CLIENT_CLOSED` (499, nginx's code): the client went away (`context.Canceled`) before the answer; nobody reads it, so it is only seen in access logs and metrics. `httpx.Fail` answers it once for every handler, it is never documented per endpoint and the web never shows it.
- Everything else is **business-specific** and listed per endpoint: `ERROR_NOT_FOUND`, `ERROR_FORBIDDEN`, `ERROR_RATE_LIMITED`, `ERROR_LIMIT_REACHED`, and field-level codes such as `ERROR_INVALID_TITLE` (the web shows a message under the field).
- Mapping from the old codes: `snake_case` becomes `ERROR_` + upper case (`invalid_title` -> `ERROR_INVALID_TITLE`, `internal_error` -> `ERROR_INTERNAL`, `invalid_body` / `unknown_field` -> `ERROR_PARAM`).
- `ERROR_TERMS_REQUIRED` (T-111, D-35): HTTP 400 from `register` when `accept_terms_version` is missing or not the current version (nothing is created); HTTP 403 from every other endpoint that needs the session cookie while the user has not accepted the current Terms and Privacy Policy. The gate is one check in `auth.Handler.RequireUser`, so a new endpoint gets it by being wrapped like the others; only `get-me`, `accept-terms`, `logout` and the two email-verification endpoints skip it. The web answers it by sending the user to the accept screen. It is documented in `ApiForbidden` and in `register`, not in every `x-error-codes`.
- Server code builds errors with `internal/apperr` (see `docs/go-conventions.md`); one mapper turns them into status + envelope.

## 4. Field rules

- **Timestamps**: Unix milliseconds as a JSON number (`int64`), named `*_at` (`created_at`, `deadline_at`). Unset optional timestamps are `null`. Always say "Unix ms" in the description.
- **Ids**: the public id of a row is its ULID string (`id`, `yearbook_id`, ...). Ids are **canonical**: exactly 26 upper-case Crockford base32 characters (`^[0-9A-HJKMNP-TV-Z]{26}$`, `ulid.Valid`). The database compares ids case-insensitively, so every endpoint that takes an id from the client answers any other spelling (lower case, wrong length, spaces, look-alike characters) with 400 `ERROR_PARAM` before it queries anything (`ERROR_INVALID_MEDIA` for a `cover_media_id` / `photo_media_id` field, the code of an unknown photo); object-store keys are built from the stored id, never from the caller's spelling (T-100, audit M3). Internal numeric ids never leave the server (L-05 stays; the ULID is our ordered unique id, no UUID). Any other `int64`/`uint64` that can exceed 2^53 is a string; small counters and sizes (`bytes`, `width`) stay numbers.
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
A new public endpoint that takes a bearer secret puts it in the **JSON body** instead (deliberate exception to "`GET` reads": `POST /api/class-invite/lookup`, T-125), so no proxy, load balancer or browser history ever sees it in a URL; it answers the same `ERROR_NOT_FOUND` for every token that is not live.

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
| `POST /v1/auth/register` | `POST /api/auth/register` | T-111: the body must carry `accept_terms_version` (the current Terms version) |
| `POST /v1/auth/login` | `POST /api/auth/login` | |
| `POST /v1/auth/logout` | `POST /api/auth/logout` | |
| `POST /v1/auth/verify-email` | `POST /api/auth/verify-email` | T-048 made this a 6-digit code check; same name. Final auth names (T-071): `register`, `login`, `logout`, `verify-email`, `resend-verification`, `forgot-password`, `reset-password`, `google-start`, `google-callback`; `data` of `register`, `login` and `get-me` is the user object itself `{id, email, email_verified, display_name, locale, created_at}`; `logout`, `verify-email`, `forgot-password`, `reset-password` answer an empty `data`; `resend-verification` answers `{already_verified}` |
| `POST /v1/auth/verify-email/resend` | `POST /api/auth/resend-verification` | |
| `POST /v1/auth/forgot-password` | `POST /api/auth/forgot-password` | |
| `POST /v1/auth/reset-password` | `POST /api/auth/reset-password` | |
| `GET /v1/auth/google/start` | `GET /api/auth/google-start` | |
| `GET /v1/auth/google/callback` | `GET /api/auth/google-callback` | the redirect URI registered at Google changes: owner action |
| `GET /v1/me` | `GET /api/user/get-me` | T-111 adds `terms: {accepted_version, current_version, required}` to its `data` (the user object plus `terms`) |
| (new, T-111) | `POST /api/user/accept-terms` | body `{version}`; stores the version and the time when it equals the current one, else `ERROR_PARAM`; `data` is the get-me object |
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
| (new, T-093) | `GET /api/book-export/get-text?yearbook_id=&lang=` | the book as plain text with public photo links (D-32); see [book-export.md](book-export.md) |
| `POST /v1/public/collect/{token}/notes` (T-034) | `POST /api/public-collect/submit-note?token=` | **deliberate exception:** `token` is a query parameter, not a body field, so the link is checked before the (up to 32 MiB) body is read; the body is the multipart form. Never logged (access log records the route only) |

`purpose` (T-152): `POST /api/media/create` takes the optional multipart field `purpose` and `GET /api/media/get-list` the optional query parameter `purpose`; the only accepted value is `design` (an image for the layout editor), absent means a yearbook photo, and anything else, `photo` included, is `ERROR_PARAM`. Every media object in an answer carries `purpose` (`photo` or `design`). The details are in [media.md](media.md), section "Design images".

`ERROR_INVALID_DESIGN` (400, T-141) is the business code of `book-design/update`: the options document is not valid for the book's template, the message names the field; the rules are in [templates.md](templates.md), section "Design block and book designs".

`book-layout/*` (T-153): `get`, `save` and `reset` of the free layout of a book (owner only, 404 for anyone else). T-163 adds the history: `POST version-list` `{yearbook_id}` answers `{items: [{id, version, kind, created_at, item_count, bytes}], next_id: ""}` (newest first, at most 30, no documents), `POST version-get` `{yearbook_id, id}` answers `{id, version, kind, created_at, layout}`, and `POST restore` `{yearbook_id, id, base_version}` answers `{version, updated_at, layout, dropped}` like `save` (a new, higher version; `dropped` counts the items and backgrounds left out because their asset or design image is gone). The three are owner only, take from the `save` rate limits, and `restore` adds `ERROR_VERSION_TEMPLATE_CHANGED` (409, the snapshot was made for another template). Retention: a save stores the document it replaces as a snapshot when the newest snapshot is older than 10 minutes (or there is none), reset and restore always; the newest 30 are kept; details in [templates.md](templates.md). Two business codes carry a `data` member next to the usual envelope fields: `ERROR_INVALID_LAYOUT` (400) with `data: {problems: [{path, code}]}` (at most 20), and `ERROR_VERSION_CONFLICT` (409, the optimistic-concurrency refusal: `base_version` is not the current version; T-156: the version of a book never decreases, reset and a template change move it on, so an old `base_version` never matches again) with `data: {version, layout}` of the stored document, so the editor can start from it. `save` answers `ERROR_TOO_LARGE` (413) for a layout over 131072 bytes, before the document is read. The rules are in [templates.md](templates.md), section "Layout block and layout document".

`ERROR_INVALID_SIGNATURE` (400, T-095) is the business code of the optional `signature` part of `submit-note`; the format is in [note-fields.md](note-fields.md), section "Signature".

New endpoints (moderation T-013, book data T-056, template list and print feedback T-061, ...) are written in v2 from the start; pick the namespace by the same rule (`note`, `book-data`, `book-export`, `book-design`, `book-layout`, `template`, `print-feedback`).
