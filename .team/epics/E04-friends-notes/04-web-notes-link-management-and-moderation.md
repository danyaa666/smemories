# E04_T-017 — Web: notes link management and moderation inbox

> **E10 contract note (2026-10-09, D-25..D-27):** build on the v2 API contract and database conventions (`docs/api-contract.md`, `docs/db-conventions.md`, `docs/go-conventions.md`); where this spec names `/v1/...` paths, old error codes, RFC 3339 times or `cursor`, use the v2 equivalents (route map: `docs/api-contract.md` section 8). Epic: `.team/epics/E10-skills-alignment/PRD.md`.

**Epic:** E04-friends-notes · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Create/copy/revoke the collection link, set a deadline, see submissions grouped by status, approve/hide/reorder, preview the note as it will print.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._


#### Leader notes from decision D-21 (2026-10-08)
The moderation inbox shows every answer of a note with the field label (use `GET /v1/public/collect/{token}`'s `fields` shape or a catalogue endpoint of T-013 for labels in the UI language); unknown field ids are shown with the raw id, never hidden. Approving or hiding works on the whole note.
