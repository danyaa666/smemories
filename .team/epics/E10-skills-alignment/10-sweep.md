# E10_T-072 — Quality sweep: mnd, forbidigo, pointer parameters, pool defaults, remove the transition switch

**Epic:** E10-skills-alignment · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P2 · **Type:** tech-debt
**Read first:** `docs/go-conventions.md`, `.agents/skills/be-golang/references/code-quality.md`, `.golangci.yml`.
**Depends on:** T-071.

#### Description
Make the remaining skill rules mechanical so they cannot regress, and delete the transitional code.

#### Requirements (SHALL)
1. `.golangci.yml` SHALL enable `mnd` (magic numbers; no `//nolint:mnd` except crypto constants with a reason) and `forbidigo` forbidding `fmt.Errorf` and `errors.New` outside `cmd/`, `internal/config`, tests and `*test` helper packages; `lint` stays green.
2. Every `fmt.Errorf`/`errors.New` left in application code SHALL be an `apperr` call; every inline duration/limit/size in business logic SHALL be a named constant or config.
3. Domain-struct parameters and returns SHALL be pointers (`*Yearbook`, `*Profile`, ...) where the skill's default applies; value types remain for tiny immutable values and scalars, with a one-line comment on each exception that a reviewer might question.
4. `SetMaxIdleConns` SHALL default to the max-open value (config default 20, `SMEM_DB_MAX_IDLE` default = `SMEM_DB_MAX_OPEN`), and the connection lifetime default stays 5 minutes.
5. The `TRANSITION(E10)` switch in `httpx` SHALL be deleted: `WriteError` emits only the v2 envelope, `WriteJSON` loses its error use, old helper names disappear.
6. A scan for queries inside loops (N+1) over all stores SHALL be recorded in the PR (command and result); any hit is fixed or justified in a comment.
7. Docs: `docs/go-conventions.md` "deviations" table re-checked against the code; the README engineering section lists the three standards.

#### Acceptance criteria
- [ ] AC1 — `make lint` green with the new linters; the PR description lists counts of fixed findings per linter.
- [ ] AC2 — No behaviour change: `make test`, `make test-integration`, web tests green, contract lint green with no skip list.
- [ ] AC3 — `grep -rn "TRANSITION(E10)\|/v1" --include='*.go' --include='*.ts' --include='*.yaml' .` returns nothing outside `.team/`, tests that assert 404 for the old paths, and CHANGELOG-style docs.
- [ ] AC4 — `go vet`, `gofmt -l` clean; no new dependency.

#### Test plan
- QA should probe: pool numbers in the start-up log, a config with `SMEM_DB_MAX_IDLE` set explicitly, error bodies for 404/405/413 and panics (all v2 now), unchanged JSON of one endpoint per domain against the openapi examples.
