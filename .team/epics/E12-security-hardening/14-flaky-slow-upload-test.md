# E12_T-123 — Flaky TestSlowUploadsDoNotStarveOthersForLong (T-102)

**Epic:** E12-security-hardening · **Milestone:** M2 · **Risk:** low · **Priority:** P2 · **Type:** bug
**Depends on:** none. **Read first:** the test (`grep -rn TestSlowUploadsDoNotStarveOthersForLong internal`), the T-102 spec `05-upload-slot-fairness.md`, how T-109 fixed its flaky test (root cause, no sleeps).

#### Why
In CI on PR #117 (T-122, 2026-10-23), go-integration failed once on this test: it got 200 where 503 was expected while the pool should be full. It passed on rerun. A flaky security test teaches everyone to rerun red CI.

#### Requirements (SHALL)
1. Find the root cause first. Is the test's timing assumption wrong (the pool not yet full when the probe request arrives), or is there a real gap in slot accounting, for example a slot released early or the per-user cap counted after the request is accepted? A real gap is a product bug: fix it and say so plainly.
2. Never just add sleeps, longer timeouts or retries. Make the test wait on an observable condition (the slot gauge reaching the cap, or a hook) before probing.
3. Show it is stable: `-race -count=50` under CPU pressure (`GOMAXPROCS=2` plus load) before and after, and a forced reordering that fails the old test and passes the new one.
4. Stop every background load process you start.

#### Acceptance criteria
- AC1: 50/50 passes under pressure after the fix; the failure is reproduced before it (or explain why it can't be, with the forced-ordering evidence instead).
- AC2: CI is green.
