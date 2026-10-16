# Usernames (T-081, decision D-28)

Every user has a **username**: a unique, public handle other people will see (class books, notes, later features). It is **not** a login name:
sign-in stays email + password or Google. Nothing but `GET /api/user/get-me` and the auth responses shows it yet.

## Rules

| Topic | Rule |
|---|---|
| Format | 3-30 characters of `a-z 0-9 . _ -`; starts and ends with a letter or digit; no two specials in a row. Input is trimmed and ASCII-lower-cased first (`Alice` is stored as `alice`); anything else, any non-ASCII character included, is `ERROR_INVALID_USERNAME` (400). The Kelvin sign and other look-alikes are not folded to ASCII, they are simply invalid. |
| Reserved | `admin`, `administrator`, `support`, `help`, `root`, `system`, `smemories`, `api`, `www`, `mail`, `team`, `staff`, `moderator`, `official`, `null`, `undefined` (list in `internal/auth/username.go`), and every `user-...` name that is not the caller's own generated default: `ERROR_USERNAME_RESERVED` (400). |
| Default | At registration (password and Google): `user-` + the last 8 characters of the new public id, lower-cased. On a unique-key conflict the insert is retried with the last 10, then 14 characters, then the whole 26-character id (no prefix). Registration never fails because of a username. Existing users were backfilled by the migration with the 8-character form (a clash gets the row id appended). |
| Change | `POST /api/user/update-username`. Allowed once every **7 days** after a user-made change; the first change from the default is allowed at once. Setting the current name again is a successful no-op and does not start the cooldown. Too early: `ERROR_USERNAME_CHANGE_TOO_SOON` (429) with `Retry-After` and the allowed time in the message. |
| Concurrency | The cooldown check and the write are one `UPDATE user_tab SET ... WHERE id = ? AND username_changed_at <= ?`, so two parallel requests cannot both pass. A duplicate (MySQL 1062 on `uq_user_tab_username`) is `ERROR_USERNAME_TAKEN` (409), never a 500. |
| Probing | Taken, invalid and reserved attempts are limited to 20 per user per hour (`username_tries`, fail-closed, `ERROR_RATE_LIMITED`); successes, no-ops and cooldown answers are not counted. A cooldown answer does not depend on whether the requested name exists. |
| Deleting | A hard delete of a user frees the name (no tombstone, no name history). |

## Storage

`user_tab.username VARCHAR(30) CHARACTER SET ascii NOT NULL` with `UNIQUE KEY uq_user_tab_username` (case-insensitive collation on top of the lower-case-only
values), and `username_changed_at BIGINT NOT NULL DEFAULT 0` (Unix ms of the last user-made change, `0` = never). `username_next_change_at` in the API is
`username_changed_at + 7 days`, or `null` when that moment has passed.

## Changing the rules

The format rule and the reserved list are pure functions (`validUsername`, `reservedUsername`) with table tests in `internal/auth/username_test.go`; a new reserved
word is one line in the map. Never loosen the format of names that already exist without a migration plan: other features will link to them.
