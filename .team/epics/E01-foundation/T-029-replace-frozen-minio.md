# E01_T-029 — Replace the frozen MinIO dev image with a maintained S3-compatible store

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low (dev and CI only; production uses AWS S3) · **Priority:** P3 · **Type:** tech-debt
**Depends on:** none. **Read first:** `docker-compose.yml` (services `minio` and `minio-init`), `Makefile` (`up`), `.github/workflows/ci.yml` (the go-integration job starts MinIO with `docker run`), `internal/storage` (the S3 client: `UsePathStyle`, `RequestChecksumCalculation`), `docs/` pages that mention MinIO.

#### Why
Dev and CI run `bitnamilegacy/minio:2025.4.22-debian-12-r2`, a frozen image that gets no security or bug fixes. Production uses AWS S3, so only dev and CI are affected, but a dead image eventually stops pulling or starts drifting from S3 behaviour.

#### What the app uses (the compatibility checklist)
PutObject, GetObject (including ranged reads if any test does them), HeadObject, DeleteObjects (multi-object delete), CreateBucket, ListObjectsV2 with the paginator, path-style addressing, the SDK's default request checksums (`RequestChecksumCalculation` as configured), and a readiness probe for compose `--wait`.

#### Requirements (SHALL)
1. **Choose by test, not by opinion.** Try candidates in this order and take the first that passes everything below: (a) SeaweedFS (`weed server -s3` or `weed mini`, Apache-2.0), (b) versitygw with the posix backend (Apache-2.0). Each candidate must:
   - have an official image published in the last 3 months, for both amd64 and arm64, pinned by digest like the other services;
   - pass `make test-integration`, the Postman media and public-media collections, `make e2e` and `make image-smoke` with no code change.
   Put a short comparison table in the PR (image, license, last release, startup time, idle memory, results). If neither passes, stop and report: a code change for S3 compatibility needs a leader decision first.
2. **Compose:** replace `minio` and `minio-init` with the chosen service plus a bucket-creation step, keeping service names generic (`s3`, `s3-init`) and the same env names the API reads (`SMEM_S3_*`). Credentials stay the dev/CI placeholders. A healthcheck makes `make up` wait for it. Give it a **new named volume**. The old `minio-data` volume must stay untouched: never remove it and never run `down -v`. `docs/` explains how to copy old dev objects across if someone wants them (one command, optional).
3. **CI:** the go-integration job uses the same image and digest. Dependabot's docker ecosystem keeps the digest current.
4. **Docs:** every MinIO mention in `docs/`, `README.md` and the `Makefile` help is updated. Record the choice in the PR for the leader to log as an L-decision.
5. No product code changes. If one turns out to be required, stop and report (see requirement 1).

#### Acceptance criteria
- AC1: On a fresh clone, `make up && make migrate && make test-integration` passes against the new store.
- AC2: CI is green on all jobs with the new image.
- AC3: `docker volume ls` still shows `smemories_minio-data` and `awesomeproject1_minio-data` (if present) after the switch.
- AC4: `grep -rni minio` finds only the optional migration note and the history in docs.

#### Test plan
Run the full local suite listed in requirement 1 twice (fresh volume, then reuse), plus the CI run. QA repeats it on a clean compose project with its own name and ports.
