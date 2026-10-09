# T-025 — Backups, restore drill, and user data export/deletion

> **E10 note (2026-10-09, D-26):** the database has no foreign keys, so deleting a user is an explicit transaction: delete each of the user's yearbooks through `yearbook.Service.Delete` (which purges storage and calls every child purger), then `user_identity_tab` rows, then the `user_tab` row; sessions and codes live in Redis and are removed by key. Extend the zero-rows test of T-066 (`dbtest.AssertNoOrphans` and the full-book test) to cover a user with several books before this task can pass QA.

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** security

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Automated RDS and S3 backups with a documented and tested restore drill; user-initiated export and deletion of account and yearbooks including media and contributor submissions; retention policy written down (owner decision on retention periods).

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._

#### Leader notes from the T-009 review (2026-10-08)
Include an orphan sweep for photo storage: an upload racing a yearbook delete can leave one object without a database row (`media.Service.PurgeYearbook`). A periodic job lists
`yearbooks/<id>/` keys older than a day that have no `media` row and deletes them (log the count). Also cover the bucket in the restore drill (versioning or replication decision goes to the owner with the cost).
