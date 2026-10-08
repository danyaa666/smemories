# T-013 — Notes moderation API (approve, hide, reorder, delete)

**Epic:** E04-friends-notes · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Owner lists pending/approved/hidden notes per yearbook, approves or hides them, reorders approved notes, deletes a note (and its photos). Only approved notes are exported. Cross-user access returns 404.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._


#### Leader notes from decision D-21 (2026-10-08)
A note's text is `answers` (JSON object, field id to text), not fixed columns (T-034). The owner list returns `answers` as stored plus the photo ids; the display name of a note is `answers.name` (fallback "Anonymous"). Search or sorting by a field is out of scope.
