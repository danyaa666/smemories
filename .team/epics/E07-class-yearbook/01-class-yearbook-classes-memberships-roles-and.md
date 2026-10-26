# E07_T-026 — Classes, memberships and roles (API foundation)

**Epic:** E07-class-yearbook · **PRD:** [PRD.md](PRD.md) · **Design:** [DESIGN.md](DESIGN.md) (read it first) · **Milestone:** M3 · **Risk:** high · **Priority:** P2 · **Type:** feature
**Depends on:** none. **Read first:** DESIGN.md, docs/api-contract.md, docs/db-conventions.md, docs/go-conventions.md, the `be-*` skills in `.agents/skills/`, `internal/yearbook` (store and service patterns), `internal/auth` (`RequireUser`, `User.EmailVerified`).

#### Requirements (SHALL)
1. **Migration** (expand-only, goose, safe under the T-108 lock): `class_tab` and `class_member_tab` exactly as in DESIGN.md. Register both in `dbtest.orphanPairs`.
2. **New package `internal/class`** (handler → service → store; handlers never call stores; `internal/apperr`; no `fmt.Errorf`/`errors.New`). Endpoints, all `{status,data}` and behind `RequireUser`:
   - `class/create` (verified email required; the creator becomes `admin`; cap 10 classes as admin, `ERROR_LIMIT_REACHED` 409), `class/get-list` (my classes with my role, sealed cursor paging), `class/get` (name, my role, my page status; **no roster**), `class/update` (admin), `class/delete` (admin; removes class rows only, never yearbooks).
   - `class-member/get-list` (**admin only**: names, role, joined, page status), `class-member/update-role` (admin; promote or demote), `class-member/remove` (admin), `class-member/leave` (any member). **The last admin can neither leave, be removed nor be demoted** (`ERROR_FORBIDDEN` with a clear code).
3. **Authorisation helper** `class.Authorize(ctx, userID, classPublicID, minRole)` used by every handler; "not a member" and "no such class" both answer 404, wrong role answers 403. All access checks are SQL joins on `class_member_tab`, never in-memory.
4. Validate at the boundary (name 1..120 runes, school and year as in yearbook, language `en|vi`), size-cap bodies, rate-limit writes per user (reuse the existing limiter pattern; fail closed).
5. **docs/class-permissions.md**: the permission matrix from DESIGN.md as the written contract, plus the privacy statement.
6. openapi + regenerated `web/src/api/schema.d.ts`, a Postman collection `postman/class.postman_collection.json` (runs twice on fresh accounts), docs.

#### Acceptance criteria
- [ ] AC1 — A table-driven **matrix test** exercises every endpoint above as: admin, student, non-member, anonymous, and a member of another class; it asserts 200 or 404 or 403 exactly as DESIGN.md says and that responses for non-members are identical to "no such class".
- [ ] AC2 — The last-admin rule holds under concurrency (two admins leaving at once leaves one).
- [ ] AC3 — Caps work (11th class refused); creating a class needs a verified email.
- [ ] AC4 — `class/get` for a student never returns the roster; `class-member/get-list` for a student is 403.
- [ ] AC5 — Migration up/down/up round trip; 4 concurrent `migrate up` still one winner; `go test -race -tags integration`, lint, newman green; CI green.

#### Test plan
QA fuzzes ids (lower-case, wrong length, another class's id), tries IDOR from a second class, hammers leave/remove concurrently, checks that no response leaks roster data or the existence of a class to a non-member.
