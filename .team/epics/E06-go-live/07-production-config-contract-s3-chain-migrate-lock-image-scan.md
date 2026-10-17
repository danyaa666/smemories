# E06_T-086 — Production config contract, S3 credential chain, migrate lock and image scan (T-022 slice 2)

**Epic:** E06-go-live · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** infra
**Depends on:** T-022 (slice 1: image, shutdown, smoke test, CI `image` job; merged first). **Read first:** `02-dockerfile-production-config-and-migrations.md` (the full T-022 spec and its leader notes) and `docs/container.md`.

#### Description
This is the second half of T-022, split off because slice 1 was a coherent, mergeable increment. It delivers requirements 4, 5, 6, 7 (the scan part) and 9 of the T-022 spec, and acceptance criteria AC3, AC4, AC5, AC6 (Trivy, SBOM, `make image-scan`) and AC7 of that spec. Copy nothing: **the T-022 spec is the source of truth for the text of those requirements and ACs**; this task only says which ones are in scope and adds the points below.

#### In scope
- Req 4 / AC3: the production configuration contract and `docs/production-config.md` (every `SMEM_*` variable, with a test that fails when `internal/config` reads a variable the document lacks).
- Req 5 / AC4: S3 default credential chain when `SMEM_S3_ACCESS_KEY` is empty, plus explicit dial/TLS/response-header/operation timeouts and a bounded retry count.
- Req 6 / AC5: `smemories-migrate` advisory lock, idempotence, `status`/`version`, the API start-up schema-version check, the deploy-order and expand/contract rule in the docs.
- Req 7 / AC6 (remaining part): Trivy scan (pinned, fail on fixable HIGH/CRITICAL), SBOM artifact, `make image-scan`.
- Req 9 / AC7: the image secret-hygiene check.

#### Points added by the leader
1. **Prod start needs a mailer (found by T-022).** The API cannot start with `SMEM_ENV=prod` until T-021 adds a real mailer, because the only mailer is the log mailer, which refuses prod. Do NOT weaken that guard. Test the production contract with `SMEM_ENV=prod` using a test-only mailer injected by the loader tests (not reachable from the binary), and keep the image smoke test on `SMEM_ENV=test` for the full API start; the refusal checks and the migrate task already run with `SMEM_ENV=prod`. Write in `docs/production-config.md` that a first real prod start waits for T-021.
2. **Licence label.** The image label says `NOASSERTION` because the repo has no LICENSE file; the owner decides the licence (Q-034). Leave the label as it is until the owner answers.
3. **CMD, not ENTRYPOINT** (decided in slice 1): ECS RunTask can override the command, not the entrypoint, so `smemories-migrate` and `smemories-media-backfill` run from the same task definition; document this in `docs/container.md` next to the migrate runbook.
4. **Backward compatible migrations from now on** ("expand then contract"): the previous API version must keep working against the new schema for one deploy; add a CI-time or review-time note (a short checklist in `docs/production-config.md`) and mention that `user_tab.username` (T-081) has no default.

#### Acceptance criteria
The ACs named above, exactly as written in the T-022 spec, plus:
- [ ] AC-A — With `SMEM_ENV=prod` the loader refuses (message names the variable, never prints a secret) for each missing or unsafe value listed in the T-022 requirement 4, in a table-driven test; the log mailer still refuses prod.
- [ ] AC-B — `make image-scan` and the CI `image` job fail on a deliberately vulnerable base image (prove it once in the PR description with a link to the failing run or a local transcript) and pass on the real image.

#### Out of scope
The same as T-022 (AWS resources, the web image, DNS, autoscaling) and the real mailer (T-021).

#### Risk
`high`: production configuration and the security posture of the shipped artifact; the owner approves the merge. No cloud account needed.

#### Test plan
As in T-022. QA should also start the built image with each required variable removed one at a time, with a weak key, with `http://` origins and with the dev OTP set, and run two migrate tasks at once.
