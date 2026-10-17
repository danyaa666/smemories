# E06_T-022 — Dockerfile, production config and migrations as a one-off task

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** infra

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Multi-stage Dockerfile (distroless or alpine, non-root), production config validation, graceful shutdown, migrate-as-task entrypoint, image scan in CI.

#### Description
Package the API for production without needing an AWS account yet: one reproducible container image that holds the three binaries (`smemories-api`, `smemories-migrate`, `smemories-media-backfill`), a production configuration that refuses to start when anything unsafe or missing, migrations that run as a separate one-off task (never inside the API start), and an image scan in CI. The infrastructure (T-023) only has to point a Fargate task definition at what this task produces. The web app is NOT in this image: it is a static build (`web/dist`) that T-023 puts on S3 + CloudFront.

#### Requirements (SHALL)
1. **Image.** `Dockerfile` at the repo root, multi-stage: build with the Go version of `go.mod` (pin the builder image by version and digest), `CGO_ENABLED=0`, `-trimpath`, version and commit through `-ldflags`; final stage `gcr.io/distroless/static-debian12:nonroot` (pinned by digest) or an equally small non-root base, user id 65532, no shell, no package manager. The image contains the three binaries, the embedded migrations (already embedded or copied), the templates manifests and fonts the API needs at run time, and nothing else (no `.git`, `.env`, tests, `node_modules`, `.team`). `.dockerignore` enforces that. Default `ENTRYPOINT` is the API; the migrate task and the backfill task run by overriding the command (`smemories-migrate`, `smemories-media-backfill`). OCI labels (source, revision, version, licenses). Image size is reported in CI and stays under 60 MB.
2. **Run as non-root with a read-only root filesystem.** The API must work with `--read-only` plus one writable tmpfs for the upload spool: `SMEM_UPLOAD_TMP_DIR` is documented and points at a mounted path in the container docs; nothing else writes to disk (prove it by running the smoke test with `--read-only`).
3. **Graceful shutdown.** On SIGTERM the API stops accepting, finishes in-flight requests (including a public upload in progress) within a configurable `SMEM_SHUTDOWN_TIMEOUT` (default 25 s, must be below the Fargate stop timeout of 30 s), flushes logs and exits 0; a test sends SIGTERM during a slow request and asserts it completes and the exit code is 0. `/readyz` starts answering 503 as soon as shutdown begins so the load balancer drains first.
4. **Production configuration contract.** With `SMEM_ENV=prod` the API refuses to start (clear message naming the variable, never printing a secret) unless: `SMEM_ALLOWED_ORIGINS` and `SMEM_PUBLIC_BASE_URL` are set and https; `SMEM_DB_DSN` is set and requires TLS (`tls=` parameter or an explicit documented opt-out variable for in-VPC RDS with the RDS CA bundle path); `SMEM_REDIS_URL` is `rediss://` (TLS) with credentials; `SMEM_OTP_KEY` and `SMEM_OIDC_COOKIE_KEY` (when Google is on) are at least 32 bytes; `SMEM_S3_BUCKET` is set and no static `SMEM_S3_ACCESS_KEY` is required (task role, see 5); `SMEM_DEV_FIXED_OTP` and every other registered dev shortcut are unset (existing guard, keep and test); `SMEM_TRUST_PROXY` is true only if documented as behind the ALB; `SMEM_MEMORY_LIMIT_MIB` is set (about 80% of the task memory) and `SMEM_METRICS_ADDR`, when set, is not a public address (reject `0.0.0.0` and `:port` unless `SMEM_METRICS_ALLOW_ANY_INTERFACE=true`, documented). One table in `docs/production-config.md` lists every `SMEM_*` variable: required in prod, default, secret or not, where it comes from in AWS (plain env, Secrets Manager, SSM), and which command uses it (api, migrate, backfill).
5. **Storage credentials from the task role.** Extend the S3 client to the AWS default credential chain when `SMEM_S3_ACCESS_KEY` is empty (static keys keep working for MinIO in dev/test); set explicit dial, TLS-handshake, response-header and overall operation timeouts and a bounded retry count on the S3 client in production config (the AWS default is about 25 s on a refused connection, see the T-075 note below); tests with a fake credential provider and a refused endpoint prove the timeouts.
6. **Migrations as a one-off task.** `smemories-migrate` runs goose `up` (and `status`, `version`) from the embedded SQL, takes an advisory lock (`GET_LOCK`) so two tasks cannot migrate at once, logs each migration applied, exits non-zero on failure, never runs `down` in prod unless `--allow-down` is passed together with `SMEM_ENV` not prod. The API never migrates on start; instead it checks at start-up that the schema is at the expected version (refuse to serve if the DB is behind or ahead by more than the compatible range, message names the versions). Document the deploy order: migrate task first, then roll the API (the T-081 `user_tab.username` column has no default, so an old API cannot insert users after the migration: say so in the runbook and keep migrations backward compatible with the previous API version from now on, "expand then contract").
7. **Image scan and build in CI.** A new CI job `image` builds the image (no push), runs Trivy (pinned version, DB cached) failing on HIGH/CRITICAL vulnerabilities that have a fix, generates an SBOM artifact, then runs the smoke test (8). The job is also runnable locally with `make image` and `make image-scan`. Pin every action and image by version or digest like the other jobs.
8. **Smoke test of the image.** `make image-smoke` (and the CI job) starts the built image against the compose MySQL, Valkey and MinIO with a prod-like config (`SMEM_ENV=prod` with TLS opt-outs documented for the local network ONLY inside the test, never in the image), runs `smemories-migrate`, then the API with `--read-only`, `--user 65532`, `--cap-drop ALL`, and checks `/healthz` 200, `/readyz` 200, a register + login + yearbook create through the real API, and the SIGTERM test of 3. It also asserts the API refuses to start with the dev shortcut variable set and with a missing `SMEM_OTP_KEY`.
9. **No secrets anywhere.** The image, its layers (`docker history`), the CI logs and the SBOM contain no secrets; the build uses no build args for secrets; add a check that greps the final image filesystem for `.env`, `*.pem`, `AKIA` and the dev OTP value `123123`.

#### Acceptance criteria
- [ ] AC1 - `docker build` produces the image described in 1 and 2; `docker run --read-only --user 65532 --cap-drop ALL` starts it with a valid prod config; the file listing shows only what 1 allows; size under 60 MB.
- [ ] AC2 - Graceful shutdown (3) is proven by a test (SIGTERM during a slow request; exit 0; `/readyz` 503 during drain).
- [ ] AC3 - Config validation (4): a table-driven test starts the loader with each missing or unsafe value and asserts the refusal message (no secret echoed); `docs/production-config.md` lists every `SMEM_*` variable and a test fails when a variable read by `internal/config` is missing from the document.
- [ ] AC4 - S3 default credential chain and timeouts (5) are covered by tests; static keys still work for MinIO.
- [ ] AC5 - `smemories-migrate` (6) is idempotent, locked, and covered by an integration test (two concurrent runs: one waits or exits cleanly, never corrupts); the API schema-version check refuses to serve on a mismatching DB; the deploy order and the expand/contract rule are written in `docs/production-config.md` (or `docs/deploy.md`).
- [ ] AC6 - CI job `image` (7) is green on the PR: build, Trivy, SBOM artifact, smoke test; `make image`, `make image-scan`, `make image-smoke` work locally; the job is added to the list of required checks only by the owner (leader will ask).
- [ ] AC7 - Secret hygiene (9) check passes and fails on a planted `.env`.
- [ ] AC8 - `docs/dev-shortcuts.md` still lists the shortcut guard; the smoke test proves a prod start refuses `SMEM_DEV_FIXED_OTP`.

#### Out of scope
AWS resources, the task definition, the ALB, CloudFront, Secrets Manager contents, DNS, the web image (static build), autoscaling, multi-arch builds beyond what the Fargate target needs (build linux/amd64 and linux/arm64 if it is cheap, otherwise amd64 only and say so).

#### Risk
`high` (production configuration and the security posture of the shipped artifact): the owner approves the merge. No cloud account is needed for this task.

#### Test plan
- Dev: unit tests for config validation and S3 timeouts, integration tests for migrate locking and the schema-version check, the image smoke test locally and in CI; say what you could not check.
- QA should probe: start the image with every required variable removed one at a time, with a weak key, with `http://` origins, with the dev OTP set; SIGTERM during an upload; two migrate tasks at once; `docker run --read-only` without the tmpfs (clear error, not a crash loop); the image for leftover files and secrets; the Trivy job failing on a deliberately old base image; the size; `/readyz` during shutdown; that `smemories-media-backfill --dry-run` works from the same image.


#### Leader notes from the T-009 review (2026-10-08)
Image processing is memory hungry (see T-036): size the container for it (2 GiB minimum unless T-036 documents otherwise), set `SMEM_MEMORY_LIMIT_MIB` to about 80% of the container memory,
and add the `SMEM_S3_*` and `SMEM_MEDIA_*` variables to the production configuration; storage credentials come from the task role, not static keys (the S3 client currently supports static keys only: extend it to the default credential chain when SMEM_S3_ACCESS_KEY is empty).

#### Leader note from decision D-23 (2026-10-08)
Production configuration also needs `SMEM_REDIS_URL` (a `rediss://` URL with the auth token from Secrets Manager), `SMEM_OTP_KEY` and `SMEM_OIDC_COOKIE_KEY`; the API must not start in prod without them. `/readyz` includes Redis, so the container health check and the load balancer target group must tolerate a Redis outage only as "not ready", not as a crash loop.

#### Leader note from T-075 (2026-10-10)
The one-off task for `smemories-media-backfill` needs only the database and object-store settings (it uses `config.LoadMigrate`); the exact variable list is in `docs/media.md` (section on the backfill). Run it with `--dry-run` first after the first deploy, then without; it is idempotent. The migrate task definition takes the same variables.
Also from T-075 QA: a failing S3 read takes about 25 s when the endpoint refuses connections (the AWS client default), so set explicit dial/response timeouts on the S3 client in production config (storage.S3Config) before relying on retry or stop-after-N logic.

#### Leader note (2026-10-15, from T-073)
The API can serve Prometheus metrics on a second listener (`SMEM_METRICS_ADDR`, default off, see `docs/observability.md`). It has **no authentication by design**: in production bind it to a private interface or the task's loopback and scrape it from inside the VPC only; never expose it through the load balancer or CloudFront. The alarm queries are listed in `docs/observability.md`.
