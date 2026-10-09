# E03_T-046 — List a yearbook's photos (API and photo library in the web UI)

**Epic:** E03-yearbooks · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature

#### Description
T-009 lets the owner upload, fetch and delete photos, but no endpoint lists them, so after a page reload the web UI (T-016) can show only the cover and the profile photo; photos uploaded earlier are unreachable. This task adds the owner-only list endpoint and plugs it into the photo section of the yearbook screen.

#### Scope
- In: `GET /v1/yearbooks/{id}/media`; OpenAPI, schema, Postman; the web photo library (list, thumbnails, delete, set as cover or profile photo) reading it; tests.
- Out (do not do): contributor photos in this list by default (they are shown with their notes, T-013/T-017), search, albums, bulk delete, any change to upload, quotas or storage.

#### Acceptance criteria
- [ ] AC1 — `GET /v1/yearbooks/{id}/media?limit=&cursor=&uploader=` (session required) returns `200 {"media":[{"id","width","height","bytes","uploader_kind","created_at"}],"next_cursor":"…"|null}` newest first. `uploader` is `owner` (default), `contributor` or `all`; anything else is `400 invalid_uploader`. `limit` defaults to 50, maximum 100 (`400 invalid_limit`); the cursor is opaque and a malformed one is `400 invalid_cursor`.
- [ ] AC2 — Only the owner of the yearbook may list it; a missing yearbook and one owned by someone else both answer `404 not_found`. Responses never contain object keys, hashes or storage URLs (content is fetched through `GET /v1/media/{id}/content`).
- [ ] AC3 — The query is one indexed read (`media(yearbook_id, id)` index or equivalent; add a migration only if EXPLAIN shows a scan) and stays correct while photos are uploaded or deleted between pages (keyset pagination on the id, no OFFSET).
- [ ] AC4 — `api/openapi.yaml`, `web/src/api/schema.d.ts` (`npm run gen:api`) and the Postman media collection are updated (list, paging, two users, bad parameters); the collection runs twice back to back.
- [ ] AC5 — Web: the photo section of the yearbook screen loads the list with TanStack Query (infinite "load more"), shows thumbnails through the content endpoint, and keeps working after a reload; deleting a photo, uploading new ones and changing the cover or profile photo update the list without a full reload. The empty state and the error state have translated messages (EN and VI parity check passes). A Vitest test covers reload, load more and delete.

#### Design
Files: `internal/media/{handler,service,store}.go` (list), `internal/media/*_test.go`, `api/openapi.yaml`, `postman/media.postman_collection.json`, `web/src/api/{media,schema.d}.ts`, `web/src/pages` photo section (`Photos.tsx` marks the plug-in point), locales.
Keyset pagination: `WHERE yearbook_id = ? AND uploader_kind IN (…) AND id < ? ORDER BY id DESC LIMIT ?`; the cursor encodes the last numeric id (base64url of a small JSON), validated and never trusted for authorisation (ownership is checked on every request).

#### Risk
`low`: an owner-scoped read built on the existing ownership check; no migration expected.

#### Security & performance notes
Scope by yearbook through `owner_id` exactly as `ownedYearbook` does; cap the limit; the cursor only narrows a query that is already scoped.

#### Test plan
- Dev: ownership matrix with two users, paging across 120 photos, deletion between pages, filter values, bad parameters; Vitest for the library.
- QA should probe: a cursor from another yearbook (must not leak), limit 0, negative and 101, a deleted photo's id as cursor, the list while uploading, 375 px layout of the library.

#### Leader note from the T-046 review (2026-10-08)
The merged cursor is the plain numeric media id in base64url, so the global auto-increment id leaves the API (L-05 keeps internal ids internal; it does not reveal another yearbook's data). Follow-up T-055 replaces it with an opaque cursor.
