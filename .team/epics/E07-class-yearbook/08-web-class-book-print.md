# E07_T-130 — Web: print the class book (many profiles, 120 pages)

**Epic:** E07-class-yearbook · **Design:** [DESIGN.md](DESIGN.md) · **Milestone:** M3 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-127, T-128. **Read first:** DESIGN.md, `web/src/book/` (BookPages, templates, `env.tsx`, `ready.ts`), `QueuedImg` and the T-121 lessons (photos always eager in the book, queued under 16, ready only when every photo has settled), `tools/print-check`, docs/templates.md, docs/adr/0003-html-print-export.md.

#### Requirements (SHALL)
1. Route `/classes/:id/book` (admin only) fetches `class-book/get` and renders: cover (class name, school, year), front class pages, then for each student in order the **existing** `tpl.Profile` and `tpl.Notes` pages of the student's own template, then back class pages, then the back cover. No change to the personal book route `/yearbooks/:id/book` or its templates; share code by extracting a per-profile page group from `BookPages`.
2. Class pages print as simple text pages (title, body with newlines, A5/A4/Letter like the other pages, fonts and ink-only rules unchanged, signatures unchanged for member notes).
3. The ready signal waits for fonts and **every photo** (T-121 rule), photos stay queued under the 16 cap, and the Print button is disabled until ready. Show the page count and an overflow summary that names students whose notes were cut by the 40-note limit or the template floor.
4. A student with no linked page prints nothing and is listed in the summary as "no page".
5. **print-check:** add a fixture of 30 students with photos and notes (A5 and A4) and assert page count within the 120-page budget, no blank pages, every photo loaded before ready, no text cut off beyond the documented summary. Runs in CI (it already runs in the e2e job); keep the added time under 2 minutes.
6. Both locales, accessibility basics (print screen only), no new dependency.

#### Acceptance criteria
- [ ] AC1 — A 30-student fixture prints in a real Chrome PDF: all pages present, order as set, class pages first and last.
- [ ] AC2 — The personal book route and its print-check cases are unchanged and green (52 plus T-097's cases).
- [ ] AC3 — Photos: 0 missing at ready with nothing scrolled; at most 16 in flight (vitest as in T-116).
- [ ] AC4 — The overflow summary lists the right students for 41-note and unlinked cases.
- [ ] AC5 — Lint, typecheck, build, vitest, print-check; CI green.

#### Test plan
QA prints a 30-student class to PDF in Chrome and Edge in both languages, compares page order with the assemble screen, and checks photo counts.
