# E12_T-107 — Bound the memory of photo downloads (get-content and public-media)

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** security
**Depends on:** T-092 (its public endpoint shares the content path). **Source:** audit run 1, [docs/security-audit-skill-2026-10-20/](../../../docs/security-audit-skill-2026-10-20/) (PR #86; until it merges read it on branch `docs/security-audit-skill-run-1`), lead 4, and the T-092 dev note (each in-flight public request holds up to 10 MiB for up to 2 minutes). **Read first:** `internal/media/handler.go` (`content`), `internal/storage/`, `docs/media.md`.

#### Description
A photo download reads the whole object into memory and serves it with `http.ServeContent`; nothing bounds how many downloads run at once, per user or in total. A slow reader keeps the buffer for the 2-minute route timeout. With T-092 the same path becomes anonymous.

#### Requirements (SHALL)
1. **Stream or bound.** Prefer streaming: serve from the object store's reader with `Content-Length`, `ETag`, `Range` (a ranged GET to the store) and `If-None-Match` behaviour unchanged. If streaming with ranges is not practical with the storage interface, keep buffering but bound it: a global semaphore on in-memory download bytes (default 64 MiB, `SMEM_MEDIA_DOWNLOAD_MAX_BYTES`, documented in `docs/production-config.md`), acquired before the store read, 503 `ERROR_BUSY` with `Retry-After` when full. Say in the PR which one you chose and why.
2. **Per-client fairness.** At most 8 concurrent downloads per user (owner route) and per client address (public route, /64 keying once T-098 lands); the next answers 429 before touching the store.
3. **Slow readers.** A write deadline per response (the route timeout already exists); a reader slower than 16 KiB/s after 15 s is dropped (same idea as `PaceBody`, applied to writes).
4. **Metrics.** `smem_media_download_inflight` (gauge) and the bytes held if buffering stays.

#### Acceptance criteria
- [ ] AC1 — Test server with a memory cap: 64 slow readers of an 8 MiB object keep heap under the bound (streaming) or get 503 past it (buffering); record RSS in the PR.
- [ ] AC2 — HEAD, Range, ETag and 304 behave exactly as before for both routes (existing tests pass unchanged, add ranged cases for public-media).
- [ ] AC3 — Per-client cap test for both routes; a normal browser page with 30 thumbnails still loads (8 at a time is enough: the web loads thumbnails lazily; check).
- [ ] AC4 — `make lint test test-integration`, Postman and `make e2e` green.

#### Risk
`high`: public endpoint after T-092 and the core photo path.
