# Continuous integration

Workflow: [`.github/workflows/ci.yml`](../.github/workflows/ci.yml). It runs on every pull request (any base branch)
and on pushes to `develop` and `main`; a newer push to the same pull request cancels its run in progress, but runs on `develop` and `main` are never cancelled (each merge commit gets a complete run). The token is
read-only (`contents: read`), no job uses a repository secret, and every third-party action is pinned to a full
commit SHA with the version in a comment (Dependabot keeps them current, see below).

**Paused jobs (D-37, T-132; D-38, T-137):** `go-integration`, `e2e` and `image` carry `if: ${{ github.event_name == 'workflow_dispatch' }}`, so on pushes and pull requests they report *skipped* (counted as passing, so the required check `go-integration` stays satisfied) and they run only by hand: `gh workflow run ci.yml --ref <branch>`. QA runs the integration and e2e commands locally; dev and QA run `make image-smoke` and `make image-scan` locally when they change the Dockerfile, `go.mod`/`go.sum` or anything the image copies. The deploy pipeline (T-023) must build, smoke-test and scan the image itself. To switch a job back on, delete its `if:` line. So on a pull request only `go`, `web` and `security` run.

**Doc-only pushes (D-38, T-137):** the `push` trigger has `paths-ignore` for `.team/**`, `docs/**`, `**/*.md` and `CLAUDE.md`: a push to `develop` or `main` that changes nothing but those paths starts no run. A push that also touches any other path runs everything that is not paused. `pull_request` has no path filter on purpose: a filter there would leave the required checks pending and block doc-only pull requests.

## Required checks (branch protection on `develop` and `main`)

Require exactly these four status checks; the names are the job names and are stable:

| Check | What it runs | Local equivalent |
|---|---|---|
| `go` | `gofmt -l .` (must print nothing), `go vet ./...` (also with `-tags integration`), golangci-lint v2.8.0 with [`.golangci.yml`](../.golangci.yml) (`errcheck`, `govet`, `staticcheck`, `ineffassign`, `unused`, `gosec`, `bodyclose`), `go test -race -coverprofile=coverage.txt ./...`; total coverage goes to the job summary | `make lint` (Go part) and `make test` |
| `go-integration` | MySQL 8.4 container (`utf8mb4`, same server flags as `docker-compose.yml`), a Valkey 8 container (same pinned image and flags) and an S3 container (SeaweedFS `weed mini`, same image and digest as `docker-compose.yml`), then `make test-integration` | `make up` then `make test-integration` |
| `web` | In `web/`: `npm ci`, `lint`, `lint:i18n`, `typecheck`, `test`, `build`, `check:api` | `make web-lint web-test web-build` |
| `security` | `govulncheck ./...` (blocking); `npm audit --audit-level=high --omit=dev` (reported, not blocking: see the comment in the workflow) | `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...` and `cd web && npm audit --audit-level=high --omit=dev` |

A fifth job, `image` (T-022, T-089: builds the production image, runs `make image-smoke`, `make image-hygiene`, the Trivy scan `make image-scan` and its self-test, and uploads the SBOM as the artifact `image-sbom`, see [container.md](container.md)), is not a required
check, and since D-38 it runs by hand only (see above).

Recommended settings for both branches: require a pull request, require the four checks above and an up-to-date
branch, no force pushes, no deletions. Do not require the `ci` workflow name; only the job names matter.

## Notes

- Go version: `go.mod` sets the language floor (`go 1.26.0`). CI builds with the newest `1.26.x`
  (`check-latest`), because the standard library of an older patch release carries known vulnerabilities that
  `govulncheck` reports. Upgrade your local Go to the latest 1.26 patch release to see the same result.
- Newest patch from go.dev (T-062): `actions/setup-go` takes its version list from a manifest that lags a Go release by
  hours, so a fresh security patch can be missing and `govulncheck` (the `security` job) goes red. The `go`,
  `go-integration` and `security` jobs therefore run the local composite action
  [`.github/actions/go-newest-patch`](../.github/actions/go-newest-patch/action.yml) right after `setup-go`. It reads
  `https://go.dev/dl/?mode=json`, takes the highest stable `go1.<minor>.N` for the `go.mod` minor, checks that it matches
  that exact pattern, and sets `GOTOOLCHAIN=go1.<minor>.N` for the rest of the job; the `go` command then downloads that
  toolchain itself and verifies it against the Go checksum database. Every job prints `go version` in that step, so the
  log shows the toolchain in use (all three jobs of a run resolve the same patch). No third-party action is involved.
  If go.dev cannot be reached or answers something unexpected, the step prints a `::warning` annotation
  ("go.dev lookup failed") and the job keeps the `setup-go` result (`check-latest`); a red `security` job in that
  situation with stdlib findings means the fallback was too old: re-run once go.dev is reachable. If the step itself fails
  at `go version`, the toolchain download from the Go module proxy failed; re-run the job. To debug, run the step's script
  locally with `scripts/test-go-newest-patch.sh [--live]`, which runs it with the runner's `bash -e -o pipefail` flags
  against good and malformed answers (HTML page, non-JSON, JSON object, empty array, wrong minor, unreachable host).
- MySQL is started with `docker run` instead of a `services:` block because a service container cannot take
  server arguments (character set, collation, `sql_mode`, time zone). The root password is a fake that exists only on the
  runner. Tests create and drop their own `smem_test_*` databases.
- Valkey 8 (Redis protocol, `docs/redis.md`) is also a `docker run`, with the digest-pinned image and the flags of `docker-compose.yml` (AOF, `noeviction`); keep the digest in both files in step. `SMEM_TEST_REDIS_URL` in the job `env` points the tests at it; each test uses its own key prefix and cleans up. The `go` job needs no Redis.
- The S3 store is started the same way (`docker run` with the digest of `docker-compose.yml`; `-p 127.0.0.1:9000:8333`; the `docker-compose` entry of `.github/dependabot.yml` raises the compose digest, and `ci.yml` and `scripts/image-smoke.sh` must follow it by hand) (fake credentials, `SMEM_TEST_S3_*` in the job `env`); the storage tests create the bucket on first use and delete the objects they write.
- `e2e` (T-020, [e2e.md](e2e.md)): the M1 journey in Chromium against the built web app and the real API on the same three services (the S3 store also gets its bucket, through `S3_BUCKET`). It is the only job that sets `SMEM_DEV_FIXED_OTP` (the API runs with `SMEM_ENV=test`). Not a required check until it has run green ten times in a row; on failure it uploads the Playwright trace, a screenshot and the API log (`e2e-failure`). Local: `make up`, `make e2e`.
- Dependabot ([`.github/dependabot.yml`](../.github/dependabot.yml)) opens one grouped weekly pull request per ecosystem
  (`gomod`, `npm` in `/web`, `github-actions`) against `develop`.
- Making `npm audit` blocking: delete `continue-on-error: true` in the `security` job once the dependency tree is clean.
