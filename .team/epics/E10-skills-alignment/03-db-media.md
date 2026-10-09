# T-065 — DB conventions: media table

**Epic:** E10-skills-alignment · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P1 · **Type:** tech-debt
**Read first:** `docs/db-conventions.md`, T-064 spec (the purger and reference hooks you plug into), T-057 spec (print columns).
**Depends on:** T-064.

#### Description
Convert `media` to `media_tab` (including the columns T-057 added), register media as a yearbook child purger and keep deletes consistent without foreign keys. No HTTP change.

#### Requirements (SHALL)
1. `media` SHALL become `media_tab`: `created_at`/`updated_at` BIGINT ms (updated_at = created_at for existing rows), `uploader_kind` VARCHAR(16) validated in Go, no FK on `yearbook_id`, `idx_yearbook_id` kept.
2. Deleting a yearbook SHALL delete its media rows in the same transaction (child purger) after the storage objects were purged.
3. Deleting one media row SHALL clear the cover/photo references (hook from T-064) in the same transaction.
4. Column names SHALL pass the reserved-word check (`bytes`, `width`, `height` stay unless they appear in `information_schema.KEYWORDS`; rename with a comment if they do).

#### Acceptance criteria
- [ ] AC1 — Migration (timestamp-named, Up/Down, data preserved) as in T-064 AC1; migration test with rows from every `uploader_kind`, a thumbnail/print key set and not set (T-057 backfill gap), microsecond and boundary timestamps; Down restores.
- [ ] AC2 — `media.Store` returns ms; `media` JSON and openapi unchanged (RFC 3339 rendered in the handler).
- [ ] AC3 — `media.Service` implements `yearbook.ChildPurger.DeleteByYearbook` (`DELETE FROM media_tab WHERE yearbook_id = ?`) and `main.go` registers it; a test builds a book with 3 photos and a cover and asserts after `yearbook.Delete` zero rows in `media_tab` for that book and nothing for other books.
- [ ] AC4 — Orphan check helper in `dbtest`: `AssertNoOrphans(t, db)` runs generic `SELECT COUNT(*) FROM child c LEFT JOIN parent p ... WHERE p.id IS NULL` for every known pair (listed in one table in the helper; T-066 and T-067 add their pairs); used by the media, yearbook and later tests.
- [ ] AC5 — Upload concurrency behaviour (T-036 caps) and storage-key layout are unchanged; all existing media integration tests pass with only SQL name changes.

#### Risk
`high`: migration + upload path data. Owner approves.

#### Test plan
- Dev: migration round trip, delete tests, `make test-integration`.
- QA should probe: delete a photo used as cover and as a profile photo; delete a yearbook with 0 photos; storage failure during purge (no DB change); two parallel deletes of the same book; Down after new uploads exist.
