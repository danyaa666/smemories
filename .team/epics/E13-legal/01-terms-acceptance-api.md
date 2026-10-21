# E13_T-111 — Versioned acceptance of the Terms and Privacy Policy (API)

**Epic:** E13-legal · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** feature
**Read first:** the PRD, decision D-35, `docs/legal/*.md` (front matter `version`), `docs/api-contract.md`, `docs/db-conventions.md`, `docs/go-conventions.md`, `internal/auth` (register, Google sign-in, `RequireUser`), `internal/notes/submit.go`.

#### Requirements (SHALL)
1. **One current version.** A constant `legal.CurrentVersion` (`"2026-10-21"`) in a small package. A unit test reads the front matter of the four files in `docs/legal/` and fails unless all four `version` values equal the constant (so changing a text forces a version bump, and the web shows the same version).
2. **Storage.** One expand-only migration: `user_tab.terms_version VARCHAR(16) NULL`, `user_tab.terms_accepted_at BIGINT NULL` (Unix ms), `note_tab.terms_version VARCHAR(16) NULL`.
3. **Register.** `POST /api/auth/register` requires `accept_terms_version` equal to the current version; missing or different answers `ERROR_TERMS_REQUIRED` (400) before anything is created; on success the version and time are stored in the same transaction as the account.
4. **Gate.** A signed-in user whose `terms_version` differs from the current one (new Google accounts, accounts created before this change, everyone after a version bump) gets `ERROR_TERMS_REQUIRED` (403) from every authenticated endpoint **except** `GET /api/user/get-me`, `POST /api/user/accept-terms`, `POST /api/auth/logout` and the email-verification endpoints. Implement it once (the `RequireUser` path), not per handler.
5. **get-me** adds `terms: {accepted_version, current_version, required}`.
6. **Accept.** `POST /api/user/accept-terms {version}` (session, CSRF guard) stores the version and time when `version` equals the current one, else `ERROR_PARAM` (a stale page); repeating it is harmless.
7. **Friend notes.** `public-collect/submit-note` stores the current version on the note (the form shows the notice, T-112); no new request field.
8. **Docs.** `api/openapi.yaml` (+ `x-error-codes`), `web/src/api/schema.d.ts`, Postman (register with and without the field, gated endpoint, accept), `docs/api-contract.md`, and `docs/legal/README.md`: how to change a text (edit all four, bump the version in the files and the constant; every user re-accepts at the next request).

#### Acceptance criteria
- [ ] AC1 — Register: missing, empty, old and current versions; the stored values; nothing created on refusal.
- [ ] AC2 — Gate: a user with a null or old version gets 403 `ERROR_TERMS_REQUIRED` on a yearbook call, and get-me, accept-terms, logout and verification still work; after accept-terms the yearbook call works. Existing integration tests that create users through the store directly get the current version (one helper), so they keep passing.
- [ ] AC3 — The version test fails when one doc's version differs; a submitted note stores the version.
- [ ] AC4 — Migration on a database with users and notes; `make lint test test-integration` and `make e2e` green (the e2e journey sends the field; T-112 adds the checkbox).

#### Risk
`high`: registration and an authorisation gate on every signed-in endpoint. The owner approves the merge.
