# E07_T-126 — A student's page: link a personal yearbook to the class

**Epic:** E07-class-yearbook · **Design:** [DESIGN.md](DESIGN.md) · **Milestone:** M3 · **Risk:** high · **Priority:** P2 · **Type:** feature
**Depends on:** T-125. **Read first:** DESIGN.md, docs/class-permissions.md, `internal/yearbook` (create, delete, `ChildPurger`), `internal/notes` moderation (what "approved" means).

#### Requirements (SHALL)
1. `class-member/set-book` (student or admin, **own membership only**): body `{class_id, yearbook_id}`. The yearbook must be owned by the caller (the same `owner_id = ?` SQL check as everywhere; foreign or missing id is 404). A yearbook can belong to at most one class (unique `yearbook_id`; 409 `ERROR_CONFLICT` otherwise). Setting it again replaces the link; `yearbook_id: null` unlinks.
2. `class/get` returns my page status: `{linked, yearbook_id, profile_complete, approved_notes}` for **my own** row only.
3. `class-member/get-list` (admin) adds per member only **counts and flags**: `linked`, `profile_complete`, `approved_notes`, `pending_notes_count` is **not** exposed. No profile text, no note text, no photo ids.
4. When a yearbook is deleted, its link is cleared in the same transaction (register a `ChildPurger`, lock order yearbook first). When a member is removed or leaves, the yearbook is untouched.
5. Linking does not change any yearbook permission: the admin gets **no** new rights on the yearbook; verify with a test that every `yearbook/*`, `note-*`, `media/*` endpoint still answers 404 to the admin for a student's book.
6. openapi, `schema.d.ts`, Postman, docs/class-permissions.md updated.

#### Acceptance criteria
- [ ] AC1 — Matrix test extended: only the member can set or clear their own link; admin cannot set a student's link; non-members get 404.
- [ ] AC2 — Same yearbook in two classes refused; deleting the yearbook clears the link; a concurrent delete and link leaves no dangling link.
- [ ] AC3 — The roster for the admin contains counts and flags only (assert the exact JSON keys).
- [ ] AC4 — Admin still gets 404 on every endpoint of a student's yearbook (the IDOR test above).
- [ ] AC5 — Integration, race, lint, newman, CI green.

#### Test plan
QA attacks with another user's yearbook id, a deleted yearbook, and tries to read any student content through roster fields.
