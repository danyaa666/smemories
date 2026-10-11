# E03_E03_T-078 — Yearbook reads: a book deleted mid-read is 404, not 500

**Epic:** E03-yearbooks · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** bug
**Depends on:** T-064 (merged).

#### Description
T-064 replaced the single join with separate queries (book, then profile, then media ids). QA of T-065 reproduced on a clean build that a book delete racing another request makes `Store.get`/`list` fail with the 500 "yearbook without its owner profile": the profile row was deleted between the two reads (2 to 4 per 360 deletes in QA's storm). A book that vanished mid-read is just "not found".

#### Requirements (SHALL)
1. `Store.get` and `Store.list` SHALL give a consistent answer: either read book, profile and media ids in one repeatable-read transaction (a read-only transaction with a consistent snapshot), or treat a missing profile as `ErrNotFound` for that book (list: skip it). Dev chooses; the snapshot is preferred when it costs no extra query.
2. A book deleted mid-request SHALL give 404 `not_found` on `get`, `update` and `update profile`, and SHALL be absent from `list`; never a 500 and never an ERROR log line.
3. The query counts stay at 3 for `get` and 3 for `list` (existing test).

#### Acceptance criteria
- [ ] AC1 — A deterministic test (a hook or a transaction held open between the two reads) reproduces the old 500 and now gets 404 / a shorter list.
- [ ] AC2 — QA's storm (parallel get/list/patch vs delete, 360 deletes) shows 0 5xx.
- [ ] AC3 — `make lint build test test-integration` green.
