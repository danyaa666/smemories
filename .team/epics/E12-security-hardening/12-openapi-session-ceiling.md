# E12_T-119 — OpenAPI: document the 90-day absolute session lifetime

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** low · **Priority:** P3 · **Type:** docs
**Depends on:** T-101. **Read first:** `api/openapi.yaml` (around line 442, the session/cookie description), `docs/redis.md` (session section after T-101), how `web/src/api/schema.d.ts` is generated and checked.

#### Why
T-101 added an absolute session lifetime: 90 days from `created_at`, with each slide = min(30 d, ceiling − now). The OpenAPI contract still describes only the 30-day sliding expiry. QA found the gap on PR #102.

#### Requirements (SHALL)
1. Every place in `api/openapi.yaml` that describes session or cookie lifetime states both rules: sliding 30 days, never beyond 90 days from sign-in. After that the client gets 401 and must sign in again.
2. Regenerate any file derived from the spec (for example `web/src/api/schema.d.ts`) with the project's own generator, so the CI drift check stays green.
3. No code changes.

#### Acceptance criteria
- AC1: `grep -n "90" api/openapi.yaml` shows the ceiling next to every sliding-expiry description.
- AC2: CI is green, including any spec or schema drift check.
