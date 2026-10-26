# E07_T-131 — Privacy review, legal texts and the class journey (e2e)

**Epic:** E07-class-yearbook · **Design:** [DESIGN.md](DESIGN.md) · **Milestone:** M3 · **Risk:** high · **Priority:** P2 · **Type:** docs+test
**Depends on:** T-129, T-130. **Read first:** DESIGN.md, docs/class-permissions.md, docs/legal/*.md (D-35, versioned acceptance), tools/e2e (the M1 journey), docs/security-audit-2026-10-20.md.

#### Requirements (SHALL)
1. **Privacy review document** `docs/class-privacy-review.md`: what personal data a class adds (roster, roles, invite metadata, students' profiles and approved notes and photos shown to the admin), who sees it (the matrix), retention (leave, remove, class delete, yearbook delete), the admin-leaves case, and the abuse cases (leaked invite, admin removed). List every endpoint that exposes class data and the test that proves its role check. End with an **owner sign-off block** left unchecked.
2. **Legal texts:** update `docs/legal/privacy.en.md`, `privacy.vi.md`, `terms.en.md`, `terms.vi.md` for class data (a class admin sees the profile and the approved notes and photos a student put in the class book; students never see each other's notes; leaving removes access). Bump `version` (the T-111 constant and its test keep the accepted-version check in step; regenerate `schema.d.ts` and the web copy), and keep the DRAFT banner until the owner finalises.
3. **E2E journey** `tools/e2e/class.spec.mjs` (CI e2e job, under 3 extra minutes): admin creates a class and an invite; two students join through the link, create pages, collect one friend note each, approve it; the admin assembles, reorders, adds an intro page and opens the print route; assert the pages for both students and that student A can not see student B's note anywhere (UI and a direct API call as A for B's yearbook, and for the admin's `class-book/get`, returns 403 or 404). Test credentials come from the existing e2e helpers (dev-shortcut rules in CLAUDE.md apply).
4. Add the "class yearbook" case to the go-live review list in `.team/epics/E06-go-live` (one bullet in the T-023 or PRD checklist).

#### Acceptance criteria
- [ ] AC1 — The privacy review lists each class endpoint with its test file and name; a grep check script `scripts/check-class-matrix.sh` fails CI if an endpoint in `api/openapi.yaml` under `class*` has no entry.
- [ ] AC2 — Legal texts updated in both languages; the version bump flows to the sign-up checkbox and the accept screen (existing tests green, new test for the bumped version).
- [ ] AC3 — The class journey passes locally and in CI; a deliberate break (admin given student's notes) makes it fail.
- [ ] AC4 — All CI jobs green; owner sign-off block present and unchecked.

#### Test plan
QA reads the review against the code, runs the journey, and tries every role on every class endpoint listed.
