# E07_T-134 — Web: edit and delete a class, link an existing yearbook as the page

**Epic:** E07-class-yearbook · **Design:** [DESIGN.md](DESIGN.md) · **Milestone:** M3 · **Risk:** low · **Priority:** P3 · **Type:** feature
**Depends on:** T-128 (merged). **Read first:** DESIGN.md, docs/class-permissions.md (class/update, class/delete, class-member/set-book), the T-128 screens in `web/src/` (class pages, join flow), QA's and dev's T-128 comments on the board.

#### Why
T-128 shipped the join flow and the student's page but left three gaps (none were in its spec):
1. The API has `class/update` and `class/delete` (admin), but the UI cannot rename or delete a class.
2. A student who already has a yearbook can only create a new page; there is no way to pick an existing yearbook of their own as the class page (`class-member/set-book` already supports it, and unlinking with `null`).
3. Google sign-in is hidden on Login/Register when the visitor comes from `/join/`, because `return_to` would put the join path (the token) in a query string. A student who only has Google must sign in first, then open the link again.

#### Requirements (SHALL)
1. **Edit class (admin):** name, school, year, language form on `/classes/:id`, saved with `class/update`; errors shown inline.
2. **Delete class (admin):** a confirm step that names the class and says what happens (members lose access; nobody's yearbook is deleted); calls `class/delete`; returns to `/classes`.
3. **Page picker (any member):** on `/classes/:id` a student can choose one of their own yearbooks (list from the existing yearbook list endpoint) or create a new one, and can unlink (`set-book` with `null`). A yearbook already linked to another class shows as unavailable with a short reason (409 handled, not hidden).
4. **Google on the join path:** keep it hidden unless a design that keeps the token out of every URL is found; instead show one sentence on Login/Register when coming from `/join/` ("Signed in with Google? Sign in first, then open your invite link again.") in both locales. Do not store the token anywhere to bridge the round trip.
5. **Role-change errors** (e.g. the last-admin refusal) show next to the member's row, not above the list (QA, T-128).
6. **Small fixes from T-129 QA:** the shared `TextArea` counter counts UTF-16 units (10 emoji show 20/4000) while the server counts runes: count runes; the two plain links "SMemories" and "Back to the class" are under 44 px on phones.
7. Both locales with i18n parity, phone width, keyboard and screen-reader basics, no new dependency, vitest for each new state. No change to `api/openapi.yaml`.

#### Acceptance criteria
- [ ] AC1 — Admin edits and deletes a class; a student sees neither control (rendered and in network calls).
- [ ] AC2 — A student can pick an existing yearbook, switch it, and unlink; a yearbook linked elsewhere shows the 409 reason.
- [ ] AC3 — The Google hint shows on Login/Register from `/join/` and nowhere else; the token is in no query string, storage or console (test).
- [ ] AC4 — Lint, typecheck, build, vitest, i18n parity; CI green (jobs that run).

#### Test plan
QA walks the three flows at 375 px in both locales as admin and as student.
