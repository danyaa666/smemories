# E07_T-133 — Class book: read limiter, response size cap and admin photo caching

**Epic:** E07-class-yearbook · **Design:** [DESIGN.md](DESIGN.md) · **Milestone:** M3 · **Risk:** low · **Priority:** P3 · **Type:** hardening
**Depends on:** T-027 (merged). **Read first:** DESIGN.md (limits), docs/class-permissions.md, `internal/class` (class-book/get), `internal/media` (`readable`, the `get-content` handler and its Cache-Control), docs/book-export.md (size notes), QA's T-027 comment on the board.

#### Why
QA found three non-blocking gaps on T-027:
1. `class-book/get` has no read limiter. About 60 colluding verified accounts in one class can make a response of roughly 50 MiB (4-byte characters, 60 members x 40 notes of maximal length); the server needs about 100-150 MB per request because the body is marshalled and then copied.
2. `media/get-content` answers with `max-age=3600`, so an admin's browser may keep showing a note's photo for up to an hour after the student hid the note and the server answers 404.
3. An admin can open the photos of approved notes beyond the 40-note cap, which the book never returns. (Decision: allowed, they are approved content of a linked page; document it, no code change.)

#### Requirements (SHALL)
1. **Read limiter:** `class-book/get` gets a per-user limiter (fail closed like the class write limiter; start at 30 requests per hour per user and class, tunable by one constant) and a 429 with `Retry-After`.
2. **Size cap:** the response body never exceeds 8 MiB. Count bytes as the notes are added in book order; when the next member's content would pass the cap, stop adding notes for that member and every later one and list them in a new field `truncated` (member ids), so the web (T-130) can name them in its overflow summary. Document the rule and the number in docs/class-permissions.md and docs/book-export.md.
3. **Encoding:** write the JSON straight to the response writer (no second full copy of the body in memory); a test asserts peak allocation for the maximal class stays under 3x the response size.
4. **Photo caching:** for a response served to a class admin through the read path (not the owner), send `Cache-Control: private, no-store`; the owner's own response keeps its current header. Test both.
5. Document item 3 of "Why" in docs/class-permissions.md. openapi, `schema.d.ts`, Postman, docs.

#### Acceptance criteria
- [ ] AC1 — The 31st read in an hour gets 429 with `Retry-After`; a limiter outage answers 503 (fail closed); another user or class is unaffected.
- [ ] AC2 — A fixture with 60 members x 40 maximal notes returns at most 8 MiB, lists the cut members in `truncated`, and the earlier members are complete; a normal class has `truncated: []`.
- [ ] AC3 — The allocation test passes under `-race`.
- [ ] AC4 — Admin photo responses carry `private, no-store`; the owner's keep their header.
- [ ] AC5 — Lint, vet, `go test -race` with the integration tag; QA runs it locally or via a manual CI run (D-37); newman folder updated.

#### Test plan
QA builds the maximal class through the API (or the fixture), checks the cap and the field, hits the limiter, and inspects headers for owner and admin.
