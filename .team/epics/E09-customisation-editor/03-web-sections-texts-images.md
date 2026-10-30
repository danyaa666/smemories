# E09_T-143 — Web runtime: sections, texts and images (Memory Book pilot)

**Epic:** E09-customisation-editor · **PRD:** [PRD](PRD.md) · **Milestone:** M1b · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-142. **Read first:** PRD "Stage 1 plan", `web/src/book/BookPages.tsx` (`pageCount`, `profilePageCount`, `ProfilePages`), `templates/index.ts` (the `Template` type), memory.tsx, `paginate.ts`, `fit.tsx` (text that does not fit shrinks and is reported), `classBook.ts` (T-130 reuses `ProfilePages`), the manifest design block from T-141.

#### Requirements (SHALL)
1. **Sections.** The book assembles its sections (`cover`, `profile`, `notes`, `back` for Memory Book) from `design.sections`: hidden ones are skipped, the order follows `design.sections.order` (fixed first/last are honoured). `pageCount` and `profilePageCount` count only what prints; page numbers/footers adapt. The class book (T-130, `ClassBookPages`) keeps working with the default design (T-146 applies each student's own).
2. **Texts.** Every fixed text the manifest lists (`cover.title`, `cover.subtitle`, `profile.heading`, `notes.heading`, `back.message`) is read through `useDesign().text(id, fallback)`; empty or missing means the template's own (translated) default. Rendered as React text only; long values shrink through the existing fit rules and are reported like other overflow.
3. **Images.** `cover.pattern` choices map to files shipped with the template (`dots` today, `plain` = none, a third if available); the book loads only the chosen file; photos of the book are unaffected.
4. **Default parity** as in T-142: no design or default design prints exactly as today (69 print-check cases and snapshots unchanged).
5. **print-check**: add cases for hidden profile, hidden back, notes before profile, custom texts at their maximum length (Vietnamese, emoji), and the plain pattern, on A5 and A4; assert page counts, order, no blank pages, no text cut off beyond the summary, and every photo loaded before ready.
6. No new dependency; no API change.

#### Acceptance criteria
- [ ] AC1 — Hide/reorder changes the pages that print and their numbers; a book with `profile` and `back` hidden prints cover + notes only.
- [ ] AC2 — Custom texts at their maximum length fit or shrink and are reported; a `<script>` string prints literally.
- [ ] AC3 — Default design parity; the personal book route and the class book are unchanged and green.
- [ ] AC4 — Lint, typecheck, build, vitest, i18n parity, check:api, print-check.

#### Test plan
QA prints every combination to PDF in Chrome and compares page order and counts with what the options say.
