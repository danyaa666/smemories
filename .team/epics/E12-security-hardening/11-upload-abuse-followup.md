# E12_T-110 — Owner uploads need a verified email and a per-client share; bound the drains of rejected public bodies

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** security
**Source:** QA of T-102 (2026-10-21, board comments on T-102). **Read first:** the T-102 spec (`05-upload-slot-fairness.md`) and its merged code in `internal/media/handler.go` and `internal/notes/submit.go`, `docs/media.md`, `docs/moderation.md` (verified-email rule on moderation).

#### Description
T-102 caps each user at 2 uploads in progress, but registration is open and uploads do not need a verified email: QA kept the 8-slot owner pool full for 75 s with 4 throwaway accounts (2 dripping uploads each) and a victim got 503 every time. Separately, the early slot release on rejected public submissions lets one rate-limited address keep many paced sockets open while their bodies drain (1500 sockets held about 56 MB RSS in QA; before T-102 it was bounded at 8 per address).

#### Requirements (SHALL)
1. **Verified email for owner uploads.** `POST /api/media/create` requires a verified email like the moderation endpoints (`ERROR_EMAIL_NOT_VERIFIED`, 403), checked before any slot is taken and before the body is read. The web upload screens already require sign-in; show the existing "verify your email" message when this error comes back.
2. **Per-client share of the owner pool.** At most 4 owner uploads in progress per client key (`httpx.ClientIP`, /64 for IPv6 since T-098), taken right after the per-user cap and released on every path; the next answers 429 with `Retry-After: 5` before the body is read.
3. **Bounded drains.** At most 16 rejected public bodies draining at once per client key; beyond that the server answers the error and closes the connection without draining (`Connection: close`), accepting that such a browser may see a reset. Count in `smem_public_drains_in_progress` (gauge, no labels with addresses).
4. **Docs.** `docs/media.md` (the three caps, verified-email rule), `docs/observability.md` (gauge), `api/openapi.yaml` (`ERROR_EMAIL_NOT_VERIFIED` on media/create), Postman.

#### Acceptance criteria
- [ ] AC1 — An unverified account gets 403 on media/create with no slot taken; a verified one uploads as before.
- [ ] AC2 — 4 verified accounts behind one client key with 2 drips each cannot fill the pool: the fifth in-progress upload from that key gets 429; a victim from another key uploads.
- [ ] AC3 — 100 rate-limited public bodies from one address: at most 16 are drained, the rest are answered and closed; memory stays flat (record RSS).
- [ ] AC4 — `make lint test test-integration`, Postman and `make e2e` green (the e2e owner verifies the email first already).

#### Risk
`high`: upload paths and an error contract change on media/create. Pre-production, no users affected.
