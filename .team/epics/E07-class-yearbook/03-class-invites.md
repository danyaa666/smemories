# E07_T-125 — Class invites (create, revoke, lookup, accept)

**Epic:** E07-class-yearbook · **Design:** [DESIGN.md](DESIGN.md) · **Milestone:** M3 · **Risk:** high · **Priority:** P2 · **Type:** feature
**Depends on:** T-026. **Read first:** DESIGN.md, `internal/notes` note-link code (token hashing, revoke, public lookup, rate limits), `internal/medialink` (token handling), docs/class-permissions.md (from T-026).

#### Requirements (SHALL)
1. **Migration:** `class_invite_tab` as in DESIGN.md. Register the orphan pair.
2. **Endpoints** (admin only unless noted):
   - `class-invite/create` returns the plain token **once**; store only its SHA-256. 32 random bytes from `crypto/rand`, URL-safe. `expires_at` defaults to 14 days, max 30. At most 5 active (unexpired, unrevoked) invites per class, else 409.
   - `class-invite/get-list` (never returns tokens; shows created, expires, revoked), `class-invite/revoke`.
   - `class-invite/lookup` **(public, token-bearing)**: returns only the class name and school for a valid active token, 404 otherwise (same body for unknown, expired, revoked). Rate-limited per IP; constant-time comparison; no timing difference between unknown and revoked.
   - `class-invite/accept` **(signed-in, verified email, Terms accepted)**: adds the caller as `student`. Idempotent if already a member (returns the membership, never downgrades an admin). Refuses with 409 when the class has 60 members. Rate-limited per user and per IP.
3. The invite link is `${PUBLIC_BASE_URL}/join/<token>`. **No invite email in v1** (docs/email.md: no links in mail). The token must never be logged: add the `/join/` path to the log-redaction used for `/c/` links, and note it in the T-023 edge-log item.
4. Revoking an invite does not remove members who already joined. Removing a member does not revoke the invite.
5. openapi, `schema.d.ts`, Postman (extend `class.postman_collection.json`), docs.

#### Carried over from T-026 QA (2026-10-26, non-blocking there, required here)
- **Admin-classes cap:** the 10-classes-as-admin cap is checked only in `class/create`. `class-member/update-role` (and the invite accept, if it ever grants admin) must enforce it too: a user who is admin of 10 classes cannot be promoted in an 11th (409, same code as create), checked under the member/class lock.
- **Caps need an enforcement point:** the 60-member and 5-active-invite caps are enforced here, under the class row lock, with a matrix cell each.
- **Matrix cell:** add a real cross-class `member_id` cell to the permission matrix (an admin of class A names a member id of class B on update-role and remove: 404, nothing changes).

#### Acceptance criteria
- [ ] AC1 — Create, list, revoke, lookup and accept work for the matrix in docs/class-permissions.md; a student cannot create or list invites (403), a non-member gets 404.
- [ ] AC2 — Expired and revoked tokens behave exactly like unknown ones on `lookup` and `accept` (same status and body).
- [ ] AC3 — 6th active invite refused; 61st member refused; accept is idempotent and never changes an admin's role.
- [ ] AC4 — A concurrency test: 10 simultaneous accepts of one token on a class with 59 members produce exactly one new member.
- [ ] AC5 — The token appears in no log line, error, metric label or stored column (grep a full integration run's logs).
- [ ] AC6 — The three carried-over items above are covered by tests. `go test -race -tags integration`, lint, newman green; CI green.

#### Test plan
QA tries token guessing and timing differences, replays an old token after revoke, accepts as an unverified user, accepts twice, and greps logs for the token.
