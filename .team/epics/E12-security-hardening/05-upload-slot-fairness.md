# E12_T-102 — Upload slot fairness: per-user cap and paced body on media/create; early release for rejected public submissions

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** security
**Source:** audit run 1, [docs/security-audit-skill-2026-10-20/](../../../docs/security-audit-skill-2026-10-20/) (PR #86; until it merges read it on branch `docs/security-audit-skill-run-1`): confirmed finding `media/create-inflight-gate-before-unpaced-body-read` (medium) and lead 7. **Read first:** `internal/media/handler.go` (`inflight`, `slowRoute`), `internal/notes/submit.go` (`PaceBody`, slot acquire), `internal/httpx/respond.go` (`PaceBody`, `BodyPace`), `docs/media.md`.

#### Description
`POST /api/media/create` takes a shared in-flight slot (pool of 4 x `SMEM_MEDIA_MAX_CONCURRENT` = 8) **before** it reads the body and holds it until the 2-minute read deadline. The body is not paced. One registered account can open 8 slow uploads and every other owner's upload answers 503 `ERROR_BUSY`. The public submission route paces its body but keeps its slot while a rate-limited request drains.

#### Requirements (SHALL)
1. **Per-user cap first.** Before the shared slot, take a per-user in-progress slot keyed by the internal user id: at most 2 uploads in progress per user (constant), the third answers 429 `ERROR_RATE_LIMITED` with `Retry-After: 5` and is refused before any body byte is read. In-process map with counts, released on every path (`defer`); one API task is enough today, note the multi-task behaviour in `docs/media.md`.
2. **Paced body.** Wrap the body with `httpx.PaceBody` using the same pace as the public route (idle 10 s, total 2 min, at least 16 KiB/s after a 15 s grace): a stalled or dripping body ends with 408 within seconds and frees both slots. Do **not** move the shared slot after the read: it bounds request-buffer memory.
3. **Early release on public rejection.** On `POST /api/public-collect/submit-note`, a request refused by a rate limit (or any rule decided before the photos are processed) releases its per-IP and global upload slots **before** draining the rest of the body, and the drain itself stays paced and bounded; the answer stays the same (errors after the body was read, so browsers show it).
4. **Metrics and docs.** `docs/media.md` (limits table) and `docs/redis.md` if a limiter is involved; the existing `smem_media_*` and `smem_public_uploads_in_progress` gauges show the new behaviour; no new label carries an id.

#### Acceptance criteria
- [ ] AC1 — Integration: one user with 2 dripping uploads (1 byte every 5 s) gets 429 on the third immediately; another user's upload succeeds meanwhile; the dripping uploads end with 408 within 30 s and the slot gauge returns to 0.
- [ ] AC2 — Eight users each with one slow upload cannot starve a ninth for longer than the pace allows (408 frees the slots); a normal 10 MiB upload over a 2 Mbit/s link (simulated) still succeeds.
- [ ] AC3 — Public route: 8 rate-limited submissions from one address release their slots before the drain finishes (gauge test), the response body is unchanged.
- [ ] AC4 — Existing upload, quota and public-submit tests and Postman suites pass; `make lint test test-integration` green.

#### Risk
`high`: upload paths of the public and owner APIs. A real phone on a very slow network could hit the pace; the numbers are the ones the public route has used since T-034.
