# E06_T-120 — CI: bound and cache the e2e Chromium install

**Epic:** E06-go-live · **Milestone:** M2 · **Risk:** low · **Priority:** P3 · **Type:** ci
**Depends on:** none. **Read first:** `.github/workflows/ci.yml` (the `e2e` job, step "Install Chromium": `npx playwright install --with-deps chromium`).

#### Why
On 2026-10-22 the e2e job hung twice in "Install Chromium" until the job timeout (PRs #109 and #111). A rerun passed both times. Every hang costs about 10 minutes and a manual rerun.

#### Requirements (SHALL)
1. Cache the Playwright browser download (`~/.cache/ms-playwright`) with `actions/cache`, pinned by commit SHA like the other actions. Key it on the OS and the resolved Playwright version from `tools/e2e/package-lock.json`. On a cache hit, install only the OS dependencies (`npx playwright install-deps chromium`).
2. Give the install step its own `timeout-minutes` (for example 4), plus one retry of the step's command, so a hang fails fast and recovers once without a manual rerun.
3. No change to the tests or the Playwright version.

#### Acceptance criteria
- AC1: Two consecutive CI runs: the second shows a cache hit and a shorter install step. Put both durations in the PR.
- AC2: The e2e job stays green, and the install step has a timeout.
