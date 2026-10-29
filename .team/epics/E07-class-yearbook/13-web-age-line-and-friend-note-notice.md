# E07_T-139 — Web: say 18+ on the class screens and tell friends a class admin may see their note

**Epic:** E07-class-yearbook · **Milestone:** M3 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** none. **Read first:** decision D-40 in .team/README.md, `docs/class-privacy-review.md` sections 1 and 7, the class create screen (`/classes/new`), the join screen (`/join/:token`), the public friend note form and its existing notice (D-35, T-112), `web/src/i18n`, the i18n parity check.

#### Requirements (SHALL)
1. **18+ line.** On the class create screen and on the join screen (the "Join {class}" step, before the accept button), show one short line in both locales: en "SMemories is for people aged 18 or older." / vi "SMemories dành cho người từ 18 tuổi trở lên." It is plain text near the primary button, not a checkbox and not a blocker (the terms already forbid younger users; the sign-up checkbox of D-35 stays the legal gate).
2. **Friend note notice.** On the public friend note form, add one sentence to the existing notice: en "If the yearbook owner puts your note in a class book, the class admin can see it." / vi "Nếu chủ cuốn lưu bút đưa lời nhắn của bạn vào sách lớp, quản trị viên lớp có thể xem được." Keep the existing notice's wording and position; the sentence follows it.
3. Do not change `docs/legal/*` (the privacy text already covers class admins; the owner finalises the legal texts later). Update `docs/class-privacy-review.md` section 7: mark points 1 and 2 as decided (D-40) and say where the lines are shown.
4. Both locales, i18n parity, 375 px, no new dependency, no API change. vitest: each line renders on its screen, in both locales, and not on other screens.

#### Acceptance criteria
- [ ] AC1 — The 18+ line shows on the class create and join screens in en and vi and is not a blocker.
- [ ] AC2 — The friend form shows the new sentence after its notice in both locales.
- [ ] AC3 — Lint, typecheck, build, vitest, i18n parity, check:api; PR jobs green (D-37/D-38: only go, web, security run on PRs).

#### Test plan
QA checks the three screens at 375 px in both locales and that nothing else changed on them.
