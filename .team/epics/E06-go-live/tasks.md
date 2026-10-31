# Epic: e06-go-live

PRD: [PRD.md](PRD.md) · Board index: [../../README.md](../../README.md)

Task blocks for this epic. The leader writes them with `board.py add-task --epic e06-go-live`; status, branch,
PR and comments change only through `board.py`.

<!-- tasks:start -->

### T-149 — [E06] CI: raise the e2e Print check timeout (suite outgrew 6 minutes)
- **Status:** DONE
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
- **Updated:** 2026-10-30 20:44Z by human
- **Comments-seen:** 4

Spec: .team/epics/E06-go-live/18-ci-print-check-timeout.md (read it from the main checkout). Manual full run 41052503585 failed e2e only on the Print check step timeout (6 min); the suite is 163 tests, ~319 s.

#### Comments
- 2026-10-30 12:32Z · dev · PR #146 head eaeda27: Print check timeout 6->15 min, e2e job 15->25 min, comment + docs/e2e.md note. Manual full run 41054429190 all 6 jobs green, 'print-check: 333 s'. PR checks: go/web/security pass, 3 paused skipped. actionlint not installed locally; diff is 2 values.
- 2026-10-30 12:35Z · qa · PR #146 head eaeda27, base develop (contains T-146 5334c1c). Evidence:
  AC1: manual run 41054429190 (workflow_dispatch, headSha eaeda27) = success; all 6 jobs success (go, web, security, image, go-integration, e2e); log line 'print-check: 333 s' (limit now 15 min).
  AC2: gh pr diff = ci.yml 2 lines (job timeout 15->25 + comment 'print-check to 15 min'; Print check step 6->15) and docs/e2e.md +2 lines (163 tests, ~319 s, limits 15/25 min). Nothing under .team, no test code, if: lines untouched. actionlint 1.7.7 (docker rhysd/actionlint, no install) on ci.yml: base and head give identical output, 6 findings all shellcheck style (SC2034/SC2086) in untouched run scripts, none new, no workflow-syntax errors.
  AC3: PR checks: go, web, security SUCCESS; go-integration, e2e, image SKIPPED.
  Mergeable: GitHub MERGEABLE/CLEAN; local git merge-tree of eaeda27 onto origin/develop (124ca61) is conflict-free.
  make test-integration / make e2e not run (CI-only change, no product code).
  Non-blocking: 333 s is 55% of the new 15 min step limit, fine headroom.
- 2026-10-30 12:35Z · leader · merged 73c26a9. Review: diff limited to ci.yml (e2e job timeout 15->25, Print check step 6->15, comment) and docs/e2e.md note; if: lines untouched. QA: manual full run 41054429190 all six jobs green on eaeda27 ('print-check: 333 s'), actionlint via docker same as base (6 pre-existing style warnings), PR checks go/web/security green. For owner to check: nothing beyond the next manual run staying green.
- 2026-10-30 20:44Z · human · accepted by owner

<!-- tasks:end -->
