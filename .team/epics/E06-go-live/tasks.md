# Epic: e06-go-live

PRD: [PRD.md](PRD.md) · Board index: [../../README.md](../../README.md)

Task blocks for this epic. The leader writes them with `board.py add-task --epic e06-go-live`; status, branch,
PR and comments change only through `board.py`.

<!-- tasks:start -->

### T-149 — [E06] CI: raise the e2e Print check timeout (suite outgrew 6 minutes)
- **Status:** READY_FOR_QA
- **Priority:** P2
- **Type:** ci
- **Milestone:** M2
- **Depends-on:** —
- **Risk:** low
- **Rework:** 0
- **Owner-approved:** —
- **Assignee:** —
- **Branch:** task/t-149-e06-ci-raise-the-e2e-print-check-timeout
- **PR:** 146
- **Updated:** 2026-10-30 12:32Z by dev
- **Comments-seen:** 1

Spec: .team/epics/E06-go-live/18-ci-print-check-timeout.md (read it from the main checkout). Manual full run 41052503585 failed e2e only on the Print check step timeout (6 min); the suite is 163 tests, ~319 s.

#### Comments
- 2026-10-30 12:32Z · dev · PR #146 head eaeda27: Print check timeout 6->15 min, e2e job 15->25 min, comment + docs/e2e.md note. Manual full run 41054429190 all 6 jobs green, 'print-check: 333 s'. PR checks: go/web/security pass, 3 paused skipped. actionlint not installed locally; diff is 2 values.

<!-- tasks:end -->
