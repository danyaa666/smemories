# Container image

The production image (T-022): [`Dockerfile`](../Dockerfile), allow-list [`.dockerignore`](../.dockerignore). The web app is not in it
(static build, S3 + CloudFront). The production configuration contract is [production-config.md](production-config.md) (T-086); the migration lock, the schema check and the deploy order are in [deploy.md](deploy.md) (T-088); Trivy, the SBOM and the image hygiene check are below.

## What is in it

- Build stage: `golang:1.26-bookworm` pinned by digest (the newest 1.26 patch at pin time; Dependabot's `docker` ecosystem keeps tag and digest
  current), `CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w -X ...version.Version -X ...version.Commit"`. Cross-compiles for the target
  (`--platform linux/amd64` and `linux/arm64` both build; about 49 MiB and 45 MiB of filesystem).
- Final stage: `gcr.io/distroless/static-debian12:nonroot` pinned by digest. User 65532, no shell, no package manager.
- Three binaries in `/usr/local/bin`: `smemories-api`, `smemories-migrate`, `smemories-media-backfill`. Migrations, templates and fonts are
  embedded in them; nothing else is copied. `.dockerignore` lets only `go.mod`, `go.sum`, `cmd/`, `internal/` and `migrations/` into the build.
- OCI labels `source`, `revision`, `version`, `licenses` (`LicenseRef-All-Rights-Reserved`: the repository is deliberately unlicensed, decision D-30; there is no LICENSE file and none is to be added for this label). `make image` sets version and
  revision from git.
- The default command is the API. The one-off tasks override the **command** (ECS `RunTask` can override the command but not the entrypoint):
  `smemories-migrate up` (or `status`, `version`), `smemories-media-backfill --dry-run`. Run the migrate task **before** rolling the API ([deploy.md](deploy.md)).

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

After the drain the API waits for the emails still being sent in the background (a code email has up to 2 retries, so one can take a minute and a half), but only for what is left of
`SMEM_SHUTDOWN_TIMEOUT` counted from the signal, not longer (T-090): a hung mail provider used to keep the process about 36 s after SIGTERM, past Fargate's 30 s stop timeout, so the task
was SIGKILLed. What is still unsent when the budget ends is dropped: one ERROR line `emails dropped at shutdown` (`count`) and `smem_mail_send_failures_total{class="shutdown"}` per
email; the student asks again on the resend screen. So the exit is never later than `SMEM_SHUTDOWN_TIMEOUT` plus the process teardown.

A public upload that is still arriving when the drain window ends (`SMEM_SHUTDOWN_TIMEOUT`, 25 s, below Fargate's 30 s stop timeout) is cut on a
deploy. This is accepted: uploads are bounded to 120 s by the T-080 pace limits, deploys are rare, and the browser form keeps the answers so the
user presses Send again. `/readyz` logs one WARN (`readyz: shutdown ping failed`) when the drain starts, not one per load balancer probe.

## Startup warnings

All three binaries (`smemories-api`, `smemories-migrate`, `smemories-media-backfill`) log `production safety opt-out is on` with `"variable"` set to the
name when `SMEM_ENV=prod` and one of `SMEM_DB_ALLOW_INSECURE`, `SMEM_REDIS_ALLOW_INSECURE`, `SMEM_S3_ALLOW_INSECURE` or
`SMEM_METRICS_ALLOW_ANY_INTERFACE` is on, and a WARN naming `SMEM_S3_ACCESS_KEY` when static S3 keys override the task role. Alarm on those lines in production.
Errors about `SMEM_PUBLIC_BASE_URL`, `SMEM_ALLOWED_ORIGINS` or `SMEM_S3_ENDPOINT` never repeat a `user:pass@` part (shown as `***@`), and such a value is refused
in every environment.

## Image scan, SBOM and secret hygiene (T-089)

- `make image-scan` builds the image and runs Trivy (`ghcr.io/aquasecurity/trivy`, pinned by version and digest in [`scripts/image-scan.sh`](../scripts/image-scan.sh)) on
  `docker save` of it (the Docker socket is not mounted). It exits 1 on any **HIGH or CRITICAL vulnerability that has a fix** (`--ignore-unfixed`), in the Debian base and in the Go
  modules and standard library compiled into the three binaries. It also writes the CycloneDX SBOM to `build/sbom/sbom.cdx.json` (git-ignored); CI uploads it as the artifact
  `image-sbom`. The vulnerability database is cached in `~/.cache/smem-trivy` locally and per day in CI.
- `make image-scan-selftest` proves the gate: it scans `alpine:3.12` (end of life) and expects the scan to fail.
- `make image-hygiene` ([`scripts/image-hygiene.sh`](../scripts/image-hygiene.sh)) fails when the final filesystem holds a `.env`, `*.pem` or `*.key` file, or when any file
  (binaries included) or any `docker history` line or label holds `AKIA` plus 16 key characters or the dev OTP value `123123`. It never prints a match, only the file name. Its
  `--self-test` builds a derived image with a planted `.env` and expects the check to fail, and also expects a missing image to fail: the check fails closed, with
  "cannot inspect image", whenever it cannot inspect, export or unpack the image it was given (T-090).
- Raising a pinned digest (Trivy, the Dockerfile `# syntax=` frontend, the smoke test's MinIO image) is a manual change; Dependabot covers the `FROM` lines only.
- A finding with a fix means: bump the Go patch or the dependency (or the distroless digest) and rebuild; there is no ignore file.

## Tests

- `make image` builds `smemories:local`; `make image-smoke` builds its own tag `smemories:smoke-<pid>-<time>` (so another process re-tagging `smemories:local` cannot change
  what is tested, and two runs side by side do not touch each other; the tag is removed at the end), then runs [`scripts/image-smoke.sh`](../scripts/image-smoke.sh), which starts
  its own private MySQL, Valkey and MinIO (never the `make up` stack) and checks: user, labels, no shell, file list, size under 60 MB;
  the API refusing an un-migrated database, two concurrent `smemories-migrate up`, `status`, `version`, `down` refused in prod, `smemories-media-backfill --dry-run`; the prod refusals (dev OTP shortcut, missing or short
  `SMEM_OTP_KEY` and `SMEM_MEDIA_LINK_SECRET`, no secret echoed); the API refusing to start without `SMEM_ENV` and with `SMEM_ENV=test` plus a public base URL or origin (T-099: the image sets no `SMEM_ENV`, every task definition must); a read-only root without the spool; the API under `--read-only --user 65532 --cap-drop ALL` (health,
  register, login, yearbook create, photo upload); SIGTERM during a slow request (readyz 503 while draining, the request completes, exit 0).
- Section 3 of the script also proves that a prod start refuses plaintext database and Redis (the script sets `SMEM_DB_ALLOW_INSECURE` and `SMEM_REDIS_ALLOW_INSECURE`
  for its private network only), http origins, a missing memory limit, a public metrics address and a missing bucket.
- The API part runs with `SMEM_ENV=test`, local origins (`http://localhost:5173`: dev and test are local-only) and the log mailer, which refuses `prod`; production uses Amazon SES (`SMEM_MAIL_PROVIDER=ses`, [email.md](email.md)),
  which the smoke test cannot reach, so SES is covered by the stub tests in `internal/mailer`.
- Every wait of the smoke test is bounded (T-090): each one-off task by `SMOKE_TASK_TIMEOUT` (120 s), every `curl` by `--max-time`, the image pulls by 600 s and the whole script by
  `SMOKE_TIMEOUT` (900 s; a warm run takes about 30 s), so a stuck container fails the run with a message instead of hanging it.
- CI job `image` runs `make image-smoke`, `make image-hygiene`, `make image-scan` and `make image-scan-selftest`. The smoke test checks `smemories-migrate status` by capturing its output and retrying five times, and prints the output on failure. The unit test for the drain is `TestServeDrainsReadyzThenFinishesInFlight` in `internal/httpx`.
