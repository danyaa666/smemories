# T-055 — Opaque list cursors (do not expose internal ids)

**Epic:** E03-yearbooks · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt

#### Description
The paging cursor of `GET /v1/yearbooks/{id}/media` (T-046) is the base64url of the internal numeric id, which lets a client see the global auto-increment value (L-05: internal ids stay internal). Replace it by an opaque, tamper-evident cursor and provide one shared helper for every list endpoint that will page (notes moderation T-013, yearbooks list if it uses one).

#### Scope
- In: `internal/httpx` or a small `internal/cursor` helper; the media list; tests; docs.
- Out (do not do): changing page sizes, ordering or the response shape.

#### Acceptance criteria
- [ ] AC1 — A cursor is `base64url(payload | HMAC-SHA256(key, payload))` where the payload holds the last position (the numeric id) and the endpoint name; the key is `SMEM_CURSOR_KEY` (at least 32 bytes; required outside dev and test, a fixed development key otherwise, documented in `.env.example`). A cursor from another endpoint, a changed byte, a truncated or empty value is `400 invalid_cursor`.
- [ ] AC2 — The media list uses it; its Go and Postman tests are updated; a test proves the response never contains the raw numeric id and that a forged cursor is rejected.
- [ ] AC3 — The helper is documented (`docs/api-conventions.md` or the README) so the notes moderation list (T-013) uses it.

#### Design
Small pure helper with `Encode(endpoint string, pos uint64) string` and `Decode(endpoint, s string) (uint64, error)`.

#### Risk
`low`.

#### Security & performance notes
Constant-time MAC comparison. No performance impact.

#### Test plan
- Dev: unit tests (round trip, forged, wrong endpoint, truncation) and the existing media list tests.
- QA should probe: a cursor copied from another user's session (still scoped by ownership), base64 padding variants.
