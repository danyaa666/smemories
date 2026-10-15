# E02_T-081 — Username: a unique public handle with a default and a weekly change

**Epic:** E02-auth · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high (migration on `user_tab`, user identity, public API) · **Priority:** P2 · **Type:** feature
**Depends on:** T-071 (the `/api/user/*` and `/api/auth/*` v2 domain; registration code is rewritten there). Decision **D-28** (owner, 2026-10-15).

#### Description
Every user gets a **username**: a unique, public handle. It is a name other people will see (class books, notes, later features), **not** a login name: sign-in stays email + password or Google. At registration the system assigns a default; the user can change it, at most once every 7 days. Standards: [docs/api-contract.md](../../../docs/api-contract.md), [docs/db-conventions.md](../../../docs/db-conventions.md), [docs/go-conventions.md](../../../docs/go-conventions.md).

#### Scope
- In: migration on `user_tab`, backfill of existing users, default at registration (email and Google paths), `POST /api/user/update-username`, `username` fields on `GET /api/user/get-me`, openapi, Postman, docs.
- Out (do not do): login by username, an availability-check endpoint (the form just submits and shows the 409), showing the username anywhere else (that belongs to the features that need it), profile pictures, display names, name history or name reservation after a change.

#### Data model
`user_tab` gains (goose Up/Down, timestamp-named migration):

| Column | Type | Meaning |
|---|---|---|
| `username` | `VARCHAR(30) CHARACTER SET ascii NOT NULL` + `UNIQUE KEY uq_user_tab_username` | lowercase handle, see rules |
| `username_changed_at` | `BIGINT NOT NULL DEFAULT 0` | Unix ms of the last **user-made** change; `0` = never changed (the default name is still in place) |

Backfill (same migration, before the unique key is added): every existing row gets `CONCAT('user-', LOWER(RIGHT(public_id, 8)))`. The ULID's last characters are random, so a collision is very unlikely; if the backfill finds duplicates the migration SHALL resolve them deterministically (append the row id) rather than fail. Down drops both columns and the key. Hard delete of a user frees the name (no tombstone).

#### Requirements (SHALL)
1. **Format.** 3 to 30 characters, `a-z 0-9 . _ -`, starts and ends with a letter or digit, no two specials in a row. Input is trimmed and lower-cased before validation (so `Alice` is stored as `alice`); anything else is `ERROR_INVALID_USERNAME` (400). The stored and returned value is always lower case. Uniqueness is therefore case-insensitive by construction.
2. **Reserved.** A fixed list in Go (`admin`, `administrator`, `support`, `help`, `root`, `system`, `smemories`, `api`, `www`, `mail`, `team`, `staff`, `moderator`, `official`, `null`, `undefined`, plus any name starting with `user-` that is not the caller's own default) is `ERROR_USERNAME_RESERVED` (400). The `user-` prefix is kept for generated defaults so nobody can claim someone else's default.
3. **Default.** Registration (password and Google paths) SHALL assign `user-` + the last 8 characters of the new `public_id`, lower-cased. On a unique-key conflict it retries with 10 then 14 characters, then the whole lower-cased `public_id` (26 characters, so it still fits the 30-character rule). Registration never fails because of a username.
4. **Change.** `POST /api/user/update-username` (`x-auth: required`, CSRF origin guard like the other POSTs), body `{ "username": "..." }`. Success returns the same user object as `get-me`. Setting the current name again is a successful no-op (does not start the cooldown).
5. **Cooldown.** After a user-made change the next one is allowed **7 days later**; the first change from the default is allowed at once. A too-early change is `ERROR_USERNAME_CHANGE_TOO_SOON` (429, `Retry-After` in seconds, message with the allowed time). The check and the write SHALL be one conditional `UPDATE ... WHERE id=? AND username_changed_at <= ?` so two concurrent requests cannot both pass.
6. **Taken.** A duplicate is `ERROR_USERNAME_TAKEN` (409), mapped from MySQL error 1062; never a 500. Failed attempts (taken, invalid, reserved) are limited to 20 per user per hour with the existing Redis limiter (`ERROR_RATE_LIMITED`, 429) so the endpoint cannot be used to probe which names exist.
7. **`GET /api/user/get-me`** adds `username` (string) and `username_next_change_at` (Unix ms, `null` when a change is allowed now). No other endpoint exposes the username in this task.
8. **Layering.** handler -> `user.Service` -> store as in go-conventions; errors are `apperr`; no `net/http` in the service; the reserved list and format rule are unit-tested pure functions.
9. **Docs.** `api/openapi.yaml` (summary, description, `x-auth`, `x-error-codes`, examples, per-field descriptions), `web/src/api/schema.d.ts` regenerated, Postman collection, README feature list, `docs/` note on username rules.

#### Acceptance criteria
- [ ] AC1 — Migration Up/Down/Up on a populated table: every existing user has a unique valid username; Down restores the previous schema; the migration test also covers a forced duplicate in the backfill.
- [ ] AC2 — Register by email and by Google (existing test doubles) each yields a `user-xxxxxxxx` username; a forced conflict falls back to the longer slice; `get-me` shows it and `username_next_change_at` is `null`.
- [ ] AC3 — Table test of the format rule (min/max length, upper case, leading/trailing special, double special, unicode, SQL/HTML characters, empty, whitespace only) and of the reserved list.
- [ ] AC4 — Change flow: first change succeeds; a second within 7 days is 429 with `Retry-After`; with the clock advanced past 7 days it succeeds; same-name request is a no-op that does not set the cooldown.
- [ ] AC5 — Concurrency: two users claim the same name at once: exactly one 200, one 409; one user fires two changes at once: exactly one wins (cooldown).
- [ ] AC6 — 21st failed attempt in an hour is 429; success responses and the cooldown error do not leak whether a name belongs to a deleted user.
- [ ] AC7 — openapi contract lint, `npm run check:api`, Newman for the new route, `make lint build test test-integration` green; no `.team/` files in the PR.

#### Test plan
- QA should probe: `Alice` vs `alice` clash, names with trailing spaces or look-alike unicode, a deleted user's name becoming free, Google-linked accounts, two browsers changing at once, clock edges at exactly 7 days, a user trying to take another user's default `user-xxxxxxxx` (reserved).
