# E01_T-079 — Make the timing-sensitive integration tests tolerant of a starved CI runner

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt
**Depends on:** T-077 (merged).

#### Description
After T-077 many `internal/media` tests run in parallel. On the 4-core GitHub runner the first `go-integration` run on the PR failed in a test that T-077 does not touch, `internal/ratelimit` `TestRedisLimitRetryAfterAndWindow` (retry-after 689 ms, wanted about 850 ms), and passed on re-run. QA could not reproduce it in 120+ local runs under heavy load, so it depends on CPU starvation. A second test, `TestContributorUploadBusyAndDiscard`, asserts a 2 s wall-clock limit and can flake the same way. Each flake costs a CI re-run and trust in the check.

#### Requirements (SHALL)
1. `TestRedisLimitRetryAfterAndWindow` SHALL not depend on real elapsed time to within a few hundred milliseconds: use the injectable fake clock (`NewRedis(..., now)`, the script accepts a clock in ARGV[4]) so the retry-after is computed from controlled time, or widen the tolerance with a stated reason; the test still proves that retry-after is the time until the oldest hit leaves the window.
2. `TestContributorUploadBusyAndDiscard` SHALL replace the 2 s wall-clock assertion by a deterministic signal (a channel or a counter that shows the second upload was turned away while the first held the slot).
3. A scan of the other `*_integration_test.go` files for `time.Since`, `time.Sleep`-based assertions and tight `< N*time.Millisecond/Second` bounds SHALL be listed in the PR description; fix those that can fail on a starved runner (<= 10 fixes; list the rest as findings).
4. The media test binary's memory (1.9 GB peak after T-077) SHALL be bounded: cap parallelism of the media package (for example a semaphore in `newEnv` or `-parallel 4` in the Makefile) so a 4-core / 7 GB runner and a laptop do not get OOM-killed; report the new peak and wall time.

#### Acceptance criteria
- [ ] AC1 — Each requirement done; no assertion weakened without a stated replacement of equal strength.
- [ ] AC2 — `go test -race -count=20 ./internal/ratelimit` (integration tag) and `-count=5 ./internal/media` pass on a loaded machine (run `stress`/parallel jobs alongside); the PR shows the numbers.
- [ ] AC3 — `make lint build test test-integration` green; CI green.
