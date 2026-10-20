# E12_T-099 — Mandatory SMEM_ENV and local-only dev/test

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** security
**Read first:** [docs/security-audit-2026-10-20.md](../../../docs/security-audit-2026-10-20.md) (findings M2 and L7), `docs/production-config.md`, `docs/dev-shortcuts.md`, `internal/config/config.go` and `prod.go`, `scripts/image-smoke.sh`.

#### Description
`SMEM_ENV` defaults to `dev` (`config.go`: `get("SMEM_ENV", "dev")`) and the image sets nothing. A task that loses the variable boots in dev mode: the log mailer prints every email code to stdout (CloudWatch), the OTP key is the public constant, the session cookie loses `Secure`, TLS to MySQL, Redis and S3 is not required. Separately, `test` and `dev` accept the public OTP key and the fixed code, so a staging deployment run as `test` is open to anyone who reads the repository.

#### Requirements (SHALL)
1. **Required.** `config.Load` and `config.LoadMigrate` return an error when `SMEM_ENV` is unset or empty: `SMEM_ENV is required: dev, test or prod`. All three binaries (`smemories-api`, `smemories-migrate`, `smemories-media-backfill`) therefore refuse to start without it. The value check (`dev|test|prod`) stays.
2. **Callers set it.** Fix every place that relied on the default: `.env.example` (already sets `dev`), `Makefile` (the `reset-limits` line uses `$(or $(SMEM_ENV),dev)`; make the targets that start the API or migrate pass it explicitly), `scripts/*.sh`, `docker-compose.yml` if it starts the API, the config tests, and the README quick start. Do **not** add an `ENV SMEM_ENV` to the Dockerfile: an explicit value in every task definition is the point.
3. **dev and test are local-only.** In `dev` or `test` the loader refuses to start when any `SMEM_ALLOWED_ORIGINS` entry or `SMEM_PUBLIC_BASE_URL` has a host that is not `localhost`, `*.localhost`, or a loopback, private or link-local IP (reuse `privateHost`'s rules in `prod.go`, without the port). Message names the variable and says dev/test are for local use and a deployed environment must use `SMEM_ENV=prod`. LAN addresses stay allowed so phone testing on the home network keeps working. Defaults (`http://localhost:5173`) pass.
4. **Smoke test.** `scripts/image-smoke.sh` gains two checks: the API image with no `SMEM_ENV` exits non-zero and names `SMEM_ENV`; the API image with `SMEM_ENV=test` and `SMEM_PUBLIC_BASE_URL=https://app.example.com` exits non-zero and names `SMEM_PUBLIC_BASE_URL`.
5. **Docs.** `docs/production-config.md` (`SMEM_ENV` row: required in every environment, no default; the local-only rule), `docs/dev-shortcuts.md` (the fixed code needs a local-only environment), `docs/container.md` if it mentions the default.

#### Acceptance criteria
- [ ] AC1 — Config tests: empty and unset `SMEM_ENV` fail for `Load` and `LoadMigrate`; `dev`, `test`, `prod` pass; `staging` fails (existing).
- [ ] AC2 — Config tests for the local-only rule: `https://app.example.com` as origin or base URL fails in dev and in test; `http://localhost:5173`, `http://127.0.0.1:3000`, `http://192.168.1.20:5173`, `http://[::1]:5173`, `http://app.localhost` pass; prod behaviour unchanged.
- [ ] AC3 — The e2e script, `make up migrate test`, `make test-integration` and the image smoke test all still pass with explicit values; no script relies on a default.
- [ ] AC4 — The two new smoke checks run and fail the job when the guard is removed (show it once locally).
- [ ] AC5 — `make build test lint` green; `docs` updated; `TestEveryVariableIsDocumented` green.

#### Risk
`high` (production configuration). A missed caller makes a command refuse to start, which is loud and local. Nothing weakens prod.

#### Test plan
QA probes: each binary without `SMEM_ENV`; with `SMEM_ENV=` (empty); a staging-like set (`test` + https origins) for the API and for migrate; the dev quick start from a clean checkout following only the README.
