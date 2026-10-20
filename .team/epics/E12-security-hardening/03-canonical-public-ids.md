# E12_T-100 — Canonical public ids and the stored id for object keys

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** security
**Read first:** [docs/security-audit-2026-10-20.md](../../../docs/security-audit-2026-10-20.md) (finding M3), `docs/api-contract.md`, `docs/db-conventions.md`, `docs/go-conventions.md`, `internal/notes/moderation.go` (`isPublicID`, the comment on collation), `internal/media/service.go`, `internal/yearbook/service.go`.
**Do before** T-025 (user data deletion), which relies on a yearbook delete that really deletes.

#### Description
The `public_id` columns are `CHAR(26)` in `utf8mb4_0900_ai_ci`, so `WHERE public_id = ?` matches any case spelling of an id. Only the notes package rejects non-canonical ids. `media.Service.Upload` builds the S3 prefix `yearbooks/<id>/` from the **caller's spelling**, and `yearbook.Service.Delete` purges `yearbooks/<caller's spelling>/` instead of the stored id. A user can upload with a lower-case `yearbook_id`, delete the yearbook with the canonical one, and leave the objects in S3 while the rows (and so the 500 MiB per-user quota) are gone: unbounded storage cost, and "delete my yearbook" does not delete the photos.

#### Requirements (SHALL)
1. **One validator.** Move `isPublicID` to `internal/ulid` as `ulid.Valid(s string) bool` (exactly 26 upper-case Crockford base32 characters) and use it from notes. Table-tested.
2. **Validate at every edge.** Every handler or service entry that takes a public id from the client rejects a non-canonical one with `ERROR_PARAM` before any query or object-store call: yearbook (`id`, `yearbook_id` in get, update, update-profile, delete), media (`yearbook_id` form field and query, `id` in content and delete), note collections (`yearbook_id`, `id` in revoke), notes moderation (already), book-data (already through notes), page-cursor ids. Enumerate them with a grep for every `public_id = ?` and every `ID`/`YearbookID` field in a request struct, and list what you found in the PR.
3. **Keys from the stored id.** `ownedYearbook` also returns the stored `public_id`, and `Upload` builds the prefix from it; `yearbook.Service.Delete` calls `PurgeYearbook(ctx, y.ID)`. Even a caller that bypasses the handlers can then only write and purge under the canonical prefix.
4. **Not in scope (L-15).** Changing the column collation: it needs a table rebuild for no extra safety once 2 and 3 hold.
5. **Docs.** `docs/api-contract.md` (ids are upper-case ULIDs; others are 400), `api/openapi.yaml` (`pattern: '^[0-9A-HJKMNP-TV-Z]{26}$'` on id parameters and fields, regenerate `web/src/api/schema.d.ts`), Postman cases.

#### Acceptance criteria
- [ ] AC1 — `ulid.Valid` table test: valid; lower-case; 25 and 27 characters; `I`, `L`, `O`, `U`; spaces; accented look-alikes; empty.
- [ ] AC2 — Handler tests: each endpoint of requirement 2 answers 400 `ERROR_PARAM` for a lower-case version of a real id, and the database and the object store are not touched (fake store counts calls).
- [ ] AC3 — Service-level integration (real MySQL and MinIO or the memory store): calling `media.Service.Upload` directly with a lower-case spelling stores under the canonical prefix; `yearbook.Service.Delete` called with a lower-case spelling purges that prefix and no object is left (list the bucket).
- [ ] AC4 — The M3 attack replayed through the API (upload with lower-case id, delete with canonical) fails at the first step with 400; add it to Postman.
- [ ] AC5 — No behaviour change for canonical ids: existing integration and Postman suites pass unchanged; web build and `check:api` green.

#### Risk
`high` (data lifecycle). A lower-case id that used to work now answers 400; the web app only sends ids it received from the API, so nothing legitimate changes. No production data yet, so there is nothing to clean up.

#### Test plan
QA probes: every id parameter with lower-case, mixed case, whitespace, 26 non-Crockford characters, a trailing slash or `../`; upload then delete with both spellings; the bucket listing after each; quota numbers before and after.
