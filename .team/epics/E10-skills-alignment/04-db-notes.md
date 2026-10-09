# E10_T-066 — DB conventions: note collections and notes tables

**Epic:** E10-skills-alignment · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P1 · **Type:** tech-debt
**Read first:** `docs/db-conventions.md`, T-064/T-065 specs, the T-034 spec (the tables it created).
**Depends on:** T-065.

#### Description
Convert `note_collections` and every table T-034 created for submitted notes to `note_collection_tab`, `note_tab` (and the photo/answer tables, if T-034 made separate ones), register notes as a yearbook child purger, and prove that deleting a fully populated yearbook leaves no row anywhere.

#### Requirements (SHALL)
1. All notes tables SHALL follow the conventions: `_tab`, ms timestamps (`deadline_at`, `revoked_at` become BIGINT NULL ms), no FK, no ENUM (note status becomes VARCHAR or TINYINT with Go constants), every `*_id` indexed, `updated_at` on every table.
2. Deleting a yearbook SHALL delete its collections and notes (and note-to-photo links) in the same transaction.
3. Deleting a collection (today: revoke) SHALL keep its notes (as today); only a yearbook delete removes them.
4. A test SHALL delete a yearbook that has profiles, photos, two collections and notes in each status and assert zero rows in every table that can refer to it.

#### Acceptance criteria
- [ ] AC1 — Migration(s) timestamp-named with Up/Down and a round-trip test (rows with and without deadline/revocation, boundary values).
- [ ] AC2 — `notes.Store` returns ms; handler output unchanged in this task.
- [ ] AC3 — `notes.Service` (new, small) implements the purger; registered in `main.go`.
- [ ] AC4 — `dbtest.AssertNoOrphans` pairs for notes added; the full-book zero-rows test (Requirement 4) passes; it lists the tables it checks from `information_schema.tables` (every `*_tab` except `user_tab` family) so a future table that nobody registered fails the test.
- [ ] AC5 — Token hashing, public lookup behaviour, rate limits and T-034 upload caps untouched; existing tests pass with SQL name changes only.

#### Risk
`high`: migration of personal-data-bearing public-form data. Owner approves.

#### Test plan
- QA should probe: revoked collection with notes, collection with deadline in the past, deleting a book while a public submission is in flight (the submission either lands before the delete or fails cleanly, never an orphan note), Down/Up round trip.
