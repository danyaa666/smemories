# T-033 — CI: do not cancel in-progress runs on develop and main

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** high · **Priority:** P2 · **Type:** infra

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Evidence from the T-006/T-030 merges: the workflow's concurrency group has cancel-in-progress: true for every event, so a push to develop cancels the still-running CI of the previous push (the leader's board-sync commits do this right after each merge). The CI runs of ff48f5b and 7c8b302 were cancelled; only the later commit ran to the end. Cancelling superseded runs is right for pull requests but wrong for develop and main, where every merge commit should get a complete run.
Change: cancel-in-progress: ${{ github.event_name == 'pull_request' }} in .github/workflows/ci.yml, and one sentence in docs/ci.md. Also digest-pin the mysql:8.4 image in the go-integration job (QA note from T-004).
Risk high (CI): needs owner approval to merge. Tiny change.

_BACKLOG: needs a full spec before it moves to TODO (acceptance: a push to develop followed within a minute by a second push leaves both runs to finish; a superseded PR run is still cancelled)._
