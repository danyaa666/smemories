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
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-148-e09-customise-screen-guard-browser-back-
- **PR:** https://github.com/danyaa666/smemories/pull/144
- **Updated:** 2026-10-30 08:40Z by dev
- **Comments-seen:** 1

Spec: .team/epics/E09-customisation-editor/08-customise-leave-guard-and-not-found.md (read it from the main checkout). Follow-up to T-144 QA: browser Back drops unsaved edits, template change drops them silently, non-owner waits 8-10 s for NotFound.

#### Comments
- 2026-10-30 08:40Z · dev · PR #144 head 788e0b9. Back: sentinel history entry while dirty (no data router). Template pick asks via TemplatePicker ask prop. NotFound slowness was owner-check yearbook/get retried 3x (not book-design/get): retryUnlessDenied on the customise observer only. vitest 668 pass, lint/typecheck/i18n/check:api/build green. QA: time non-owner NotFound in browser, Back at 375px, both locales. Known: multi-entry history jump skips dialog; extra Back press after confirmed template change. Print untouched. #143 not merged yet.

<!-- tasks:end -->
