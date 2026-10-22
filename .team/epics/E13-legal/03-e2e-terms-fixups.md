# E13_T-117 — Fix develop e2e after the Terms merges (journey import, h2-gallery register)

**Epic:** E13-legal · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** low · **Priority:** P1 · **Type:** bug
**Depends on:** none (T-111, T-112, T-107 are merged). **Read first:** `tools/e2e/journey.spec.mjs`, `tools/e2e/h2-gallery.spec.mjs`, how the journey sends `accept_terms_version` at sign-up.

#### Why
The develop CI `e2e` job fails on 78ccfa0 (the #97 merge): `journey.spec.mjs:255` calls `readFileSync`, which is not defined. T-112 removed it from the `node:fs` import, but the T-094 export-as-text step (step 9) still uses it. Each PR was green on its own; the combination is not. Second, `h2-gallery.spec.mjs` (T-107) registers through the API without `accept_terms_version`. Since T-111 the API rejects that, so the spec fails at its register step.

#### Requirements (SHALL)
1. `journey.spec.mjs` imports `readFileSync` again. Do not change step 9.
2. `h2-gallery.spec.mjs` sends `accept_terms_version` in its register call. Take the value from the same source the rest of the e2e suite uses; if the suite has no shared source, read the `CurrentVersion` constant from `internal/legal/legal.go`. Never hard-code the date.
3. If more than one spec needs the version, put a small helper in the e2e folder and use it from both. Otherwise skip the helper.
4. No product code changes. The diff covers only `tools/e2e/`.

#### Acceptance criteria
- AC1: `make e2e` passes locally against a fresh stack, including the journey (all steps) and the h2-gallery spec.
- AC2: The PR's CI `e2e` job is green.
- AC3: `grep -rn "auth/register" tools/e2e` shows every register call sending `accept_terms_version`.

#### Test plan
Run `make e2e` (with `make reset-limits SMEM_ENV=test` first if a 429 appears). The PR CI e2e job is the regression check.
