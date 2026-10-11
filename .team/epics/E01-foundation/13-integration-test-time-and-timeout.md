# E01_T-077 — Integration tests: set a timeout and cut the media package time

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt

#### Description
Dev and QA both hit Go's default 10-minute test timeout in `internal/media` on a loaded machine (763 s and 858 s measured on 2026-10-11, load average about 7); `make test-integration` passes no `-timeout`. Every task that touches media now waits many minutes per run, and QA reruns it several times. CI passes because its runner is idle, but the margin is thin and the package keeps growing (T-057, T-065 add tests).

#### Scope
- In: the `test-integration` target of the Makefile and `.github/workflows/ci.yml` (`go-integration` job): an explicit `-timeout` (30m) so a slow machine fails on a real hang, not on the default; `internal/media` integration tests: find where the time goes (`go test -v -json` per-test durations, the image-processing concurrency caps of T-036 serialising tests, large fixtures), then cut it: share one MinIO bucket/DB setup per package, reduce the pixel sizes of fixtures that do not test size limits, run independent tests with `t.Parallel()` where the global caps allow, keep the dedicated cap/stress tests as they are.
- Out (do not do): weakening any assertion, lowering the production caps, removing the stress tests (T-036, T-034).

#### Acceptance criteria
- [ ] AC1 — `make test-integration` and the CI job use `-timeout 30m`; the Makefile comment says why.
- [ ] AC2 — The PR description lists the 10 slowest tests of `internal/media` before and after, and the package time before and after on the same machine (target: at least 40% less wall time at equal coverage; say honestly if less is possible).
- [ ] AC3 — No test is removed or weakened; the mutation checks that exist (e.g. T-034 drain/refund) still fail when the code is broken.
- [ ] AC4 — `make lint build test test-integration` green; CI green.

#### Pointer from T-065 (2026-10-11)
Dev measured the slow tests: the existing backfill tests with big photos take 110 to 200 s each (for example `TestBackfillStopsAfterConsecutiveStorageFailures`, about 200 s alone because every failing S3 call waits for the AWS client's default timeout); start there. A test that points the storage at a dead endpoint should inject a fake store that fails at once instead of waiting for real network timeouts.
