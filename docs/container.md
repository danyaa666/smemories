# Container image

The production image (T-022): [`Dockerfile`](../Dockerfile), allow-list [`.dockerignore`](../.dockerignore). The web app is not in it
(static build, S3 + CloudFront). The production configuration contract, Trivy and the SBOM follow in later slices of T-022.

## What is in it

- Build stage: `golang:1.26-bookworm` pinned by digest (the newest 1.26 patch at pin time; Dependabot's `docker` ecosystem keeps tag and digest
  current), `CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w -X ...version.Version -X ...version.Commit"`. Cross-compiles for the target
  (`--platform linux/amd64` and `linux/arm64` both build; about 49 MiB and 45 MiB of filesystem).
- Final stage: `gcr.io/distroless/static-debian12:nonroot` pinned by digest. User 65532, no shell, no package manager.
- Three binaries in `/usr/local/bin`: `smemories-api`, `smemories-migrate`, `smemories-media-backfill`. Migrations, templates and fonts are
  embedded in them; nothing else is copied. `.dockerignore` lets only `go.mod`, `go.sum`, `cmd/`, `internal/` and `migrations/` into the build.
- OCI labels `source`, `revision`, `version`, `licenses` (`NOASSERTION`: the repository has no LICENSE file yet). `make image` sets version and
  revision from git.
- The default command is the API. The one-off tasks override the **command** (ECS `RunTask` can override the command but not the entrypoint):
  `smemories-migrate up` (or `status`), `smemories-media-backfill --dry-run`.

## Running it

```
docker run --read-only --user 65532 --cap-drop ALL --security-opt no-new-privileges \
  --tmpfs /spool:rw,noexec,nosuid,size=64m,uid=65532,mode=0700 \
  -e SMEM_UPLOAD_TMP_DIR=/spool --env-file api.env -p 8080:8080 smemories:local
```

- Nothing but the upload spool writes to disk. `SMEM_UPLOAD_TMP_DIR` must point at a writable mount (a tmpfs, or on Fargate a task volume);
  public submissions are spooled there while they arrive. Without a writable spool the API stops at start-up with
  `upload spool is not writable ... SMEM_UPLOAD_TMP_DIR` (exit 1), it does not fail on the first upload.
- The listener is `SMEM_HTTP_ADDR` (default `:8080`); `/healthz` is liveness, `/readyz` (database and Redis) is readiness.
- Size the container for image processing: 2 GiB minimum, `SMEM_MEMORY_LIMIT_MIB` about 80% of it (docs/media.md).

## Graceful shutdown

On SIGTERM the API (1) answers `/readyz` 503 at once, (2) keeps accepting for `SMEM_SHUTDOWN_DELAY` (default 5s in prod, 0 in dev and test)
so the load balancer notices, (3) stops accepting and lets in-flight requests, public uploads included, finish, (4) logs `shutdown complete`
and exits 0. (2) and (3) together are bounded by `SMEM_SHUTDOWN_TIMEOUT` (default 25s, 1s to 120s). Keep it below the task's stop timeout (30s
on Fargate, the default); the target group's deregistration delay should be at least `SMEM_SHUTDOWN_DELAY` plus the health check interval.

## Tests

- `make image` builds `smemories:local`; `make image-smoke` builds and runs [`scripts/image-smoke.sh`](../scripts/image-smoke.sh), which starts
  its own private MySQL, Valkey and MinIO (never the `make up` stack) and checks: user, labels, no shell, file list, size under 60 MB;
  `smemories-migrate up` twice and `status`, `smemories-media-backfill --dry-run`; the prod refusals (dev OTP shortcut, missing or short
  `SMEM_OTP_KEY`, no secret echoed); a read-only root without the spool; the API under `--read-only --user 65532 --cap-drop ALL` (health,
  register, login, yearbook create, photo upload); SIGTERM during a slow request (readyz 503 while draining, the request completes, exit 0).
- The API part runs with `SMEM_ENV=test`: the only mailer is the log mailer, which refuses `prod`, so the API cannot serve in production
  before the email provider (T-021) lands.
- CI job `image` runs `make image-smoke`. The unit test for the drain is `TestServeDrainsReadyzThenFinishesInFlight` in `internal/httpx`.
