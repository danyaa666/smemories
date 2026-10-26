# E07_T-127 — Class pages and book order (API)

**Epic:** E07-class-yearbook · **Design:** [DESIGN.md](DESIGN.md) · **Milestone:** M3 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-027. **Read first:** DESIGN.md, `internal/class`, how notes are reordered today (`notes` ordering endpoint and its validation).

#### Requirements (SHALL)
1. **Migration:** `class_page_tab` as in DESIGN.md (register the orphan pair).
2. **Endpoints (admin only):** `class-page/create`, `class-page/update`, `class-page/delete`, `class-page/get-list`. Fields: `kind` (`intro|teachers|events|superlatives|custom`), `title` (1..120 runes), `body` (plain text, at most 4000 characters, newlines kept, **no HTML**), `placement` (`front|back`). Cap 30 pages per class (409 beyond).
3. **Order:** `class-book/update-order` (admin): body lists the full ordered member ids and, per placement, the ordered page ids. It must be a permutation of the current sets (reject missing, extra or duplicate ids with 400 and change nothing); apply in one transaction under the class row lock so two admins cannot interleave.
4. `class-book/get` (T-027) now returns the pages in `front_pages` and `back_pages`.
5. openapi, `schema.d.ts`, Postman, docs.

#### Acceptance criteria
- [ ] AC1 — Matrix: only admins write; students get 403, non-members 404.
- [ ] AC2 — Order rejects partial, duplicated or foreign ids and leaves the old order intact.
- [ ] AC3 — Two concurrent reorders end in one of the two orders, never a mix (run 20 times under `-race`).
- [ ] AC4 — Body is stored and returned as plain text; a `<script>` string comes back unchanged and is rendered as text by the web (T-130 checks the rendering).
- [ ] AC5 — Integration, race, lint, newman; CI green.

#### Test plan
QA fuzzes the permutation input and the body length limits and checks the cap.
