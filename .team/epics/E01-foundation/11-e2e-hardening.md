# E01_T-087 — E2E hardening: refuse non-local targets, tighter CI limits, clearer errors

**Epic:** E01-foundation · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt
**Depends on:** T-020 (merged). Source: the non-blocking findings of the T-020 QA.

#### Description
The end-to-end test (`tools/e2e`, `scripts/e2e.sh`, CI job `e2e`) works, but QA found five rough edges:
1. **It can be pointed at the wrong environment.** Nothing stops the spec or the script from running against a non-local `E2E_BASE_URL` or the developer's own DSN. Refuse to run unless the base URL host is `localhost`, `127.0.0.1` or `::1` (or `E2E_ALLOW_REMOTE=1` is set deliberately), and refuse a DSN whose database name does not start with `smem_e2e` or `smem_test`.
2. **Vague failure when the register limit is hit.** Running `scripts/e2e.sh` directly (not through `make e2e`, which resets the limits) after 5 registrations in an hour fails at step 1 with "element(s) not found". Detect the 429 / rate-limited answer and fail with a message that says what to do (`make reset-limits`, or use a fresh Redis db).
3. **CI time budget.** The job has `timeout-minutes: 20`; the intended budget is 5 minutes. Set the job timeout to 10 and keep the Playwright limit at 240 s.
4. **CI speed.** Cache the Playwright browser download between runs (key on the Playwright version).
5. **Failure artifact contents.** The failure trace holds the throwaway link token and cookies of a throwaway DB. Harmless today, but the repository is public: strip cookies and the link token from the uploaded trace/screenshot names if Playwright allows it, otherwise document that the artifact is retained for 3 days only and set `retention-days: 3`.

#### Acceptance criteria
- [ ] AC1 - Running the e2e script/spec against a non-local base URL or a non-`smem_e2e*`/`smem_test*` database exits non-zero before touching anything, with a clear message; `E2E_ALLOW_REMOTE=1` overrides the URL check only. A test (shell or Playwright config unit) proves both refusals.
- [ ] AC2 - A rate-limited registration at step 1 fails with an actionable message.
- [ ] AC3 - CI: job `timeout-minutes: 10`, browsers cached, artifact `retention-days: 3`; one green run shown in the PR description.
- [ ] AC4 - `docs/e2e.md` updated (guards, how to override, how to read the failure).
- [ ] AC5 - No change to what the journey tests; the 10-run determinism is not re-proven, but one green CI run and one local run are shown.

#### Test plan
Dev: run with a wrong URL and a wrong DSN (both refused), with the register limit exhausted (clear message), and one normal green run. QA: the same, plus confirm the CI job definition.
