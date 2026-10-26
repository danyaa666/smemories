# E07_T-129 — Web: assemble the class book (order, class pages)

**Epic:** E07-class-yearbook · **Design:** [DESIGN.md](DESIGN.md) · **Milestone:** M3 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-127, T-128. **Read first:** DESIGN.md, the notes ordering UI (how the owner reorders notes: reuse the same component and keyboard pattern), `web/src/book/` conventions.

#### Requirements (SHALL)
1. Route `/classes/:id/assemble` (admin only; students get the not-found screen). Two lists: front class pages, the students' pages (name, `linked` flag, approved note count), back class pages. Reorder with drag and with **keyboard buttons** (move up, move down); save with `class-book/update-order`; show conflicts clearly if the server rejects the permutation (reload the current order).
2. **Class pages editor:** create, edit, delete pages of the five kinds with title and plain-text body (counter at 4000), placement front or back. Render the body as **text** (React text nodes, newlines preserved); a `<script>` string must show literally. No `dangerouslySetInnerHTML`, no new dependency.
3. Students without a linked page are listed with a visible "no page yet" state and can be moved or left out of nothing (they simply print nothing); the admin can see the counts only.
4. Both locales, i18n parity, phone width, accessibility basics (labels, focus after move, live region for the new position).
5. vitest for ordering logic (including the permutation payload), the editor and the text rendering.

#### Acceptance criteria
- [ ] AC1 — Reordering by keyboard only works and the saved order survives a reload.
- [ ] AC2 — A pasted `<img onerror>` in a page body renders as text; test asserts no element is created.
- [ ] AC3 — A rejected save (409 or 400) reloads and tells the admin.
- [ ] AC4 — Lint, typecheck, build, vitest, i18n parity; CI green.

#### Test plan
QA reorders 30 students by keyboard, edits and deletes pages in both locales, injects markup strings, and checks 375 px.
