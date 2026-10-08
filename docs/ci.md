# Continuous integration

Workflow: [`.github/workflows/ci.yml`](../.github/workflows/ci.yml). It runs on every pull request (any base branch)
and on pushes to `develop` and `main`; a newer push to the same ref cancels the run in progress. The token is
read-only (`contents: read`), no job uses a repository secret, and every third-party action is pinned to a full
commit SHA with the version in a comment (Dependabot keeps them current, see below).

## Required checks (branch protection on `develop` and `main`)

Require exactly these four status checks; the names are the job names and are stable:

| Check | What it runs | Local equivalent |
|---|---|---|
| `go` | `gofmt -l .` (must print nothing), `go vet ./...` (also with `-tags integration`), golangci-lint v2.8.0 with [`.golangci.yml`](../.golangci.yml) (`errcheck`, `govet`, `staticcheck`, `ineffassign`, `unused`, `gosec`, `bodyclose`), `go test -race -coverprofile=coverage.txt ./...`; total coverage goes to the job summary | `make lint` (Go part) and `make test` |
| `go-integration` | MySQL 8.4 container (`utf8mb4`, same server flags as `docker-compose.yml`) and a MinIO container (S3, the frozen image from `docker-compose.yml`), then `make test-integration` | `make up` then `make test-integration` |
| `web` | In `web/`: `npm ci`, `lint`, `lint:i18n`, `typecheck`, `test`, `build`, `check:api` | `make web-lint web-test web-build` |
| `security` | `govulncheck ./...` (blocking); `npm audit --audit-level=high --omit=dev` (reported, not blocking: see the comment in the workflow) | `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...` and `cd web && npm audit --audit-level=high --omit=dev` |

Recommended settings for both branches: require a pull request, require the four checks above and an up-to-date
branch, no force pushes, no deletions. Do not require the `ci` workflow name; only the job names matter.

## Notes

- Go version: `go.mod` sets the language floor (`go 1.26.0`). CI builds with the newest `1.26.x`
  (`check-latest`), because the standard library of an older patch release carries known vulnerabilities that
  `govulncheck` reports. Upgrade your local Go to the latest 1.26 patch release to see the same result.
- MySQL is started with `docker run` instead of a `services:` block because a service container cannot take
  server arguments (character set, collation, `sql_mode`, time zone). The root password is a fake that exists only on the
  runner. Tests create and drop their own `smem_test_*` databases.
- MinIO is started the same way (fake credentials, `SMEM_TEST_S3_*` in the job `env`); the storage tests create the bucket on first use and delete the objects they write.
- Dependabot ([`.github/dependabot.yml`](../.github/dependabot.yml)) opens one grouped weekly pull request per ecosystem
  (`gomod`, `npm` in `/web`, `github-actions`) against `develop`.
- Making `npm audit` blocking: delete `continue-on-error: true` in the `security` job once the dependency tree is clean.
