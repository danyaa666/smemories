# E04_T-017 — Web: notes link management and moderation inbox

> **E10 contract note (2026-10-09, D-25..D-27):** build on the v2 API contract and database conventions (`docs/api-contract.md`, `docs/db-conventions.md`, `docs/go-conventions.md`); where this spec names `/v1/...` paths, old error codes, RFC 3339 times or `cursor`, use the v2 equivalents (route map: `docs/api-contract.md` section 8). Epic: `.team/epics/E10-skills-alignment/PRD.md`.

**Epic:** E04-friends-notes · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Create/copy/revoke the collection link, set a deadline, see submissions grouped by status, approve/hide/reorder, preview the note as it will print.

_BACKLOG: needs a full spec (description, acceptance criteria, design, test plan) before it moves to TODO._


#### Leader notes from decision D-21 (2026-10-08)
The moderation inbox shows every answer of a note with the field label (use `GET /v1/public/collect/{token}`'s `fields` shape or a catalogue endpoint of T-013 for labels in the UI language); unknown field ids are shown with the raw id, never hidden. Approving or hiding works on the whole note.

#### Design source and known differences (leader, 2026-10-15)
Visual source: `docs/design/notes/Notes Inbox.dc.html` (screens I1 to I6), tokens and base components from T-083; API from T-013 (`/api/note/*`) and the link routes (`/api/note-collection/*`). Do not start before T-083 and T-013 are merged. Where the design and this spec differ, **the spec wins**. Known differences to resolve exactly like this:
1. **Shell:** the sidebar (desktop) and bottom tab bar (mobile) come from T-083's `YearbookLayout`; this task only adds the "Notes from friends" and "Note links" routes and their nav entries, with the pending-count badge from `counts.pending`.
2. **Data:** tabs and header text use `counts` from `GET /api/note/get-list`; the approved tab is in book order; paging with `next_id` ("load more"). The card shows `answers.name` (fallback "Anonymous"), `answers.relationship` and a relative time from `created_at`, the message, the photo thumbnails (`media/get-content?variant=thumb`, click opens the display variant), the status badge.
3. **Bulk and single actions** use `batch-update-status` and `batch-delete` (one id = one note). Optimistic update with rollback and an error toast on failure.
4. **Delete with Undo is client-side:** the server has no undo. Show the toast with Undo for 6 seconds and send `batch-delete` only when it expires; Undo cancels it. If the user leaves the page or closes the tab before it expires the delete is **dropped** (the note stays), never sent late. The confirm dialog (with "Hide instead") is shown for deleting an approved note or several notes.
5. **Reorder** applies to approved notes only: drag by the grip on desktop (pointer events, keyboard alternative: the ••• menu "Move up" / "Move down"), and the mobile "Book order" mode (I3) with arrows; every move announces "<name>'s note, moved to position X of N" in a live region; save with `POST /api/note/reorder` after the drop (debounced), `ERROR_ORDER_STALE` reloads the list and shows a toast.
6. **States:** first run (I4, button "Create a note link"), loading skeleton (I5), offline banner and load error with Try again (I6); the app-wide offline banner is a T-083 `Banner`.
7. **Header handle:** the design shows `@thaolinh.26`; show the username when the user object has it (T-081/T-082), otherwise the email local part.
8. **Out of scope here:** the "Preview & print" and "Photos" and "My profile page" entries (existing or later tasks), dark theme.
