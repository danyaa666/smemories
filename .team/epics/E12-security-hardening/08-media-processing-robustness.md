# E12_T-104 — Media processing robustness: slots released on every path, no panics, decoded limits

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P2 · **Type:** security
**Source:** audit run 1, [docs/security-audit-skill-2026-10-20/](../../../docs/security-audit-skill-2026-10-20/) (PR #86; until it merges read it on branch `docs/security-audit-skill-run-1`), lead 5 and the hardening note on `internal/media/image.go` (the print-variant goroutine has no `recover`). **Read first:** `internal/media/service.go` (`save`, slots), `internal/media/image.go`, `docs/media.md` (decode limits).

#### Description
Image limits rely on `image.DecodeConfig` only; a decoder that disagrees with its own header (the audit names WebP VP8X) could allocate more than the limit. A panic in a decoder or encoder would leak a processing slot, and a panic in the print-variant goroutine (`wg.Go`) **crashes the whole process**: one crafted upload from a public link would restart the API.

#### Requirements (SHALL)
1. **Slots.** Every processing-slot acquire is paired with a `defer` release in the same function; a test with a panicking loader shows the slot gauge back at 0.
2. **No process crash.** Decode, scale and encode (including the print goroutine) run under `recover`; a recovered panic becomes `ERROR_INVALID_IMAGE` (400) for the caller, one ERROR log line without image bytes, and `smem_media_process_panics_total`.
3. **Decoded limits.** After decode, check the real bounds against the same pixel limit as the header check and refuse a mismatch; read the `golang.org/x/image` WebP decoder for the VP8X canvas vs frame case and add a ~100-byte fixture that exercises it (header small, frame large, or the reverse).
4. **Docs.** `docs/media.md` (what is checked before and after decode).

#### Acceptance criteria
- [ ] AC1 — Panicking loader, decoder and encoder tests (inject through the existing seams): no crash, 400, slot released, metric incremented.
- [ ] AC2 — The VP8X fixture is refused (or proven harmless, with the reason in the PR) and stays in the test data.
- [ ] AC3 — Existing image tests, fuzz seeds and print-size tests pass; `make lint test test-integration` green.

#### Risk
`high`: the image pipeline is reachable from the public form.
