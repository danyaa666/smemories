# E07_T-027 — Class book data API and the admin's read-only media access

**Epic:** E07-class-yearbook · **Design:** [DESIGN.md](DESIGN.md) · **Milestone:** M3 · **Risk:** high · **Priority:** P2 · **Type:** feature
**Depends on:** T-126. **Read first:** DESIGN.md, `internal/bookdata` (`book-data/get`, caps, truncation), `internal/notes/book.go` (approved-note query), `internal/media` (`get-content`, authorisation, `medialink`), docs/book-export.md, docs/media.md.

#### Requirements (SHALL)
1. `class-book/get` (**admin only**): returns `{class, front_pages[], members[], back_pages[]}` where each member entry is `{member_id, name, profile (the same shape as book-data), notes[] (approved only, in the student's own order, at most 40), note_count, truncated}` and the yearbook's template, page size and language. Members without a linked yearbook, or with an empty profile, are listed with `linked:false` and no content. Class pages come from T-127 (empty arrays until then). Order is `class_member_tab.sort_order`.
2. **Only approved content leaves a student's book:** never pending or hidden notes, note-link tokens, other students' data, or unapproved photos. Build the member content by calling the existing approved-notes query for the member's yearbook (do not copy SQL), so moderation rules stay in one place.
3. **Media read path for the admin.** `media/get-content` (and the thumb and print variants) today authorise the yearbook owner. Add a second rule: the caller is an **admin of a class whose member has linked this media's yearbook**, and the media is (a) that yearbook's profile photo or (b) a photo attached to an **approved** note of that yearbook. Everything else stays 404. Implement it as one store query, not in handlers. Keep the cache headers and the per-client download caps (T-107/T-110) unchanged.
4. Bounds: members at most 60, notes per member 40, total response measured and documented (target under 4 MiB); reuse the page budget (about 120 pages) in the docs.
5. openapi, `schema.d.ts`, Postman, docs/book-export.md section "Class book".

#### Carried over from T-126 QA (2026-10-26, required here)
- The admin-404 endpoint test (internal/class/page_integration_test.go) must cover **every** route in `api/openapi.yaml` that takes a yearbook, note or media id, including `media/create`, which T-126 left out. T-027 adds the one read path an admin has into a member's yearbook (profile photo and approved-note photos), so extend the test with that path as the only allowed exception and add a guard (a test that lists the routes from the OpenAPI file and fails when a new route is missing from the matrix).
- `profile_complete` checks that `photo_media_id` is set, not that the media row exists. Make the class book tolerate a dangling photo id (renders without the photo, never errors) and cover it with a test.

#### Acceptance criteria
- [ ] AC1 — Matrix: a student, a non-member and an admin of another class get 403 or 404 on `class-book/get` and on every photo id in it.
- [ ] AC2 — Negative media tests: pending-note photo, hidden-note photo, an unlinked yearbook's photo, a removed member's photo, a photo of a book linked to another class: all 404 for the admin.
- [ ] AC3 — A note approved after the first call appears in the next call; a note hidden after approval disappears and its photo turns 404 immediately.
- [ ] AC4 — Query count is O(members) not O(notes); EXPLAIN the main queries; a 60-member, 40-note fixture answers within the budget.
- [ ] AC5 — Integration, race, lint, newman; CI green; no change to the behaviour of the owner's own `book-data/get` (snapshot test).

#### Test plan
QA builds a class of 3 students with mixed approved, pending and hidden notes and photos, then tries to fetch every media id from every role. Fuzzes ids. Verifies caps with a large fixture.
