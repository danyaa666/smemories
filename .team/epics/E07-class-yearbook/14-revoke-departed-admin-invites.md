# E07_T-140 — Revoke a departed admin's invite links automatically

**Epic:** E07-class-yearbook · **Milestone:** M3 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** none. **Read first:** decision D-40 in .team/README.md, `docs/class-privacy-review.md` sections 4 and 7, docs/class-permissions.md, `internal/class` (`invite_store.go`, `store.go`: leave, removeMember, setRole, lockMembership and the lock order), the matrix tests.

#### Why
An invite link made by an admin stays live until it expires (14 days by default, 30 at most) or another admin revokes it. If that admin leaves, is removed or is demoted, links they made should stop working (owner decision D-40, point c).

#### Requirements (SHALL)
1. When a member who is an admin of a class stops being an admin of it by `class-member/leave`, `class-member/remove` (of an admin) or `class-member/update-role` (admin to student), all invites of that class with `created_by` = that user that are not yet revoked and not expired are revoked (`revoked_at` = now), in the **same transaction** and under the same locks as the membership change (lock order unchanged: class row, then member rows, then invite rows). Class delete already removes the invites.
2. Members who already joined with those links stay members. Other admins' invites are untouched. A user who is only a student has no invites to revoke (no-op).
3. The last-admin rule is unchanged (the last admin still cannot leave, be removed or be demoted).
4. `class-invite/get-list` shows the revoked invites as revoked, as today. No API shape change; no migration (use the existing `revoked_at`).
5. Tests (integration, `-race`): leave, remove and demote each revoke exactly that admin's active invites and nothing else; a revoked token answers like any dead token on lookup and accept; concurrent accept of a token while its creator leaves ends either accepted-before or refused, never half; a student leaving revokes nothing. Add the three cells to the permission matrix and update docs/class-permissions.md and `docs/class-privacy-review.md` (sections 4 and 7: point 3 decided, D-40) and the Postman "invites" folder.

#### Acceptance criteria
- [ ] AC1 — The three paths revoke the leaver's active invites in one transaction; other invites untouched.
- [ ] AC2 — The concurrent test passes 20 times under `-race`.
- [ ] AC3 — Docs updated; `scripts/check-class-matrix.sh` still green.
- [ ] AC4 — Lint, vet (also `-tags integration`), `go test -race ./...`; because D-37 skips go-integration on PRs, dev starts the full CI by hand (`gh workflow run ci.yml --ref <branch>`) and quotes the run id; QA verifies it on the PR head.

#### Test plan
QA creates two admins with invites, removes/demotes/leaves each way, and checks lookup and accept for each token.
