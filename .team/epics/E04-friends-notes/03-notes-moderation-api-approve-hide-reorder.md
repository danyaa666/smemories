# E04_T-013 — Notes moderation API (approve, hide, reorder, delete)

> **E10 contract note (2026-10-09, D-25..D-27):** this task is built on the v2 API contract and database conventions, not the v1 ones written below. Read `docs/api-contract.md`, `docs/db-conventions.md` and `docs/go-conventions.md` first. Wherever this spec names a `/v1/...` path, an old error code, an RFC 3339 time, a `cursor`, a foreign key or a `DATETIME` column, use the v2 equivalent (route map: `docs/api-contract.md` section 8). New tables are `_tab` tables with BIGINT ms timestamps and no foreign keys. Epic: `.team/epics/E10-skills-alignment/PRD.md`.

**Epic:** E04-friends-notes · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Owner lists pending/approved/hidden notes per yearbook, approves or hides them, reorders approved notes, deletes a note (and its photos). Only approved notes are exported. Cross-user access returns 404.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._


#### Leader notes from decision D-21 (2026-10-08)
A note's text is `answers` (JSON object, field id to text), not fixed columns (T-034). The owner list returns `answers` as stored plus the photo ids; the display name of a note is `answers.name` (fallback "Anonymous"). Search or sorting by a field is out of scope.
