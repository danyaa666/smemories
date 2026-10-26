# E07_T-128 — Web: classes, invites, joining and the student's page

**Epic:** E07-class-yearbook · **Design:** [DESIGN.md](DESIGN.md) · **Milestone:** M3 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-126. **Read first:** DESIGN.md, docs/class-permissions.md, the existing Register/Login pages, the Terms accept screen (T-112), the yearbook list and create pages, `web/src/auth`, the i18n parity check.

#### Requirements (SHALL)
1. **Routes:** `/classes` (my classes; create button; empty state), `/classes/:id` (admin: roster with page-status flags and counts, invite section, member actions; student: class name, my page status and a button to create or open my page).
2. **Invites (admin):** create (expiry choice 7, 14 or 30 days), show the link **once** with a copy button and a warning that it cannot be shown again, list active invites, revoke. No email sending.
3. **Join flow `/join/:token`:** call `class-invite/lookup`; show "Join {class name}"; if signed out, go through the existing sign-up or sign-in and return to the same URL (keep the token in memory or the path only, never in a query string that is logged elsewhere); then `class-invite/accept`; then offer "Create my page": `yearbook/create` prefilled with the class's name, school, year and language, followed by `class-member/set-book`, then open the yearbook editor. If the lookup is 404, show one neutral "This invite is not valid" message (same for expired, revoked, unknown).
4. **Student view shows no roster and no other student** (assert in a test). Roles are shown as text, not colour only. The last-admin rule errors have clear messages.
5. Both locales (en, vi), i18n parity green, phone width (375 px), keyboard and screen reader basics, `Terms` gate honoured.
6. vitest for each screen and state; one e2e step later in T-131.

#### Acceptance criteria
- [ ] AC1 — A student sees none of the roster, invites or member actions (rendered and in network calls).
- [ ] AC2 — The invite link is shown once; after a reload it is gone; the token never appears in the URL query, localStorage or console.
- [ ] AC3 — Join works signed out (via sign-up) and signed in, and returns to the class; invalid tokens show one message.
- [ ] AC4 — Lint, typecheck, build, vitest, i18n parity; CI green.

#### Test plan
QA walks the flows at 375 px in both languages, signs up through an invite, tries a revoked link, and inspects storage and the network log for the token.
