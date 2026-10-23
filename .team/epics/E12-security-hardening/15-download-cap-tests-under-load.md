# E12_T-124 — Download-cap integration tests fail under heavy CPU load (T-107 tests)

**Epic:** E12-security-hardening · **Milestone:** M2 · **Risk:** low · **Priority:** P3 · **Type:** bug
**Depends on:** T-123 (reuse its `waitInProgress` / gauge-wait helpers). **Read first:** the T-123 PR (#118) and its board comment, `internal/media` download-cap tests, the T-107 spec `07-bounded-photo-download-memory.md`.

#### Why
While fixing T-123, dev found that `TestPublicDownloadsAreCappedPerClient`, `TestOwnerDownloadsAreCappedPerUser` and `TestDownloadAtTheCapWaitsForAFreedSlot` fail under `-race`, `GOMAXPROCS=2` and 24 busy loops, on the old code as well. They have not failed in CI yet. They are likely the same hazard: slots released (or a 500 ms grace expiring) before the test has filled the cap.

#### Requirements (SHALL)
1. Root cause per test: a timing assumption, or a real gap in the download slot accounting or grace logic? A real gap is a product bug: fix it and say so.
2. Make each test wait on an observable condition (the download slot gauge reaching the cap) before probing, with no sleeps, longer timeouts or retries. Reuse T-123's helpers.
3. Show it: under the same load, 50/50 per test after the fix, and the failure reproduced before.

#### Acceptance criteria
- AC1: The three tests pass 50/50 under the T-123 load profile. Full-package runs under load are green 3 of 3.
- AC2: CI is green; no product behaviour change unless requirement 1 finds a real bug.
