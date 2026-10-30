# Epic: e09-customisation-editor

PRD: [PRD.md](PRD.md) · Board index: [../../README.md](../../README.md)

Task blocks for this epic. The leader writes them with `board.py add-task --epic e09-customisation-editor`; status, branch,
PR and comments change only through `board.py`.

<!-- tasks:start -->

### T-148 — [E09] Customise screen: guard browser Back, ask before a template change, fast NotFound
- **Status:** READY_FOR_QA
- **Priority:** P3
- **Type:** feature
- **Milestone:** M1b
- **Depends-on:** —
- **Risk:** low
- **Rework:** 1
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-148-e09-customise-screen-guard-browser-back-
- **PR:** https://github.com/danyaa666/smemories/pull/144
- **Updated:** 2026-10-30 12:05Z by dev
- **Comments-seen:** 3

Spec: .team/epics/E09-customisation-editor/08-customise-leave-guard-and-not-found.md (read it from the main checkout). Follow-up to T-144 QA: browser Back drops unsaved edits, template change drops them silently, non-owner waits 8-10 s for NotFound.

#### Comments
- 2026-10-30 08:40Z · dev · PR #144 head 788e0b9. Back: sentinel history entry while dirty (no data router). Template pick asks via TemplatePicker ask prop. NotFound slowness was owner-check yearbook/get retried 3x (not book-design/get): retryUnlessDenied on the customise observer only. vitest 668 pass, lint/typecheck/i18n/check:api/build green. QA: time non-owner NotFound in browser, Back at 375px, both locales. Known: multi-entry history jump skips dialog; extra Back press after confirmed template change. Print untouched. #143 not merged yet.
- 2026-10-30 08:58Z · qa · 1. [blocker] PR #144 (head 788e0b9) conflicts with develop (c0e3b68, T-145 merged after you branched). gh pr view 144: mergeable=CONFLICTING, mergeStateStatus=DIRTY. Repro: git merge-tree --write-tree 788e0b9 origin/develop -> CONFLICT (content) in web/src/pages/customise.test.tsx (locales en.json/vi.json auto-merge). Fix: merge/rebase origin/develop into the task branch, resolve customise.test.tsx keeping both T-145 and T-148 tests, re-run web checks (vitest, lint, typecheck, i18n, check:api, build), push, set READY_FOR_QA. CI on 788e0b9 was green (go, web, security pass; e2e/go-integration/image skipped by D-37/D-38). Not yet tested by QA (a head that cannot merge is not worth testing, the re-merge changes the code under test): browser Back, template-change dialog, NotFound timing (AC3 under 1.5 s), 375 px both locales, print-check. All will be run on the new head.
- 2026-10-30 12:05Z · dev · Merged origin/develop (T-145, c0e3b68) into branch, head ed7891b; only conflict was imports in customise.test.tsx (kept T-145 readFileSync/resolve + T-148 act). Both test sets kept. vitest 680 pass, lint/prettier, typecheck, lint:i18n, check:api, build green. Nothing else changed.

<!-- tasks:end -->
