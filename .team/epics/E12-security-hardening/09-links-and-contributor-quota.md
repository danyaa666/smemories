# E12_T-105 — Note links capped and paged; contributor photos cannot exhaust the owner's quota

**Epic:** E12-security-hardening · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P3 · **Type:** security
**Source:** audit run 1, [docs/security-audit-skill-2026-10-20/](../../../docs/security-audit-skill-2026-10-20/) (PR #86; until it merges read it on branch `docs/security-audit-skill-run-1`): lead 6 and the hardening note "contributor photos share the owner's 200-photo quota" (the code comment in `internal/media/service.go` confirms it). **Read first:** `internal/notes/handler.go` (collections), `internal/media/service.go` (`checkQuota`, `UploadContributorFrom`), `docs/moderation.md`, `docs/media.md`.

#### Description
(a) A yearbook can have unlimited note links; revoked links cannot be removed and `get-list` returns all of them unpaged. (b) Photos sent through a link count toward the owner's quotas, so anyone holding a leaked link can fill the owner's 200-photo quota with pending junk and block the owner's own uploads until they delete it.

#### Requirements (SHALL)
1. **Link cap.** At most 50 links per yearbook, revoked included; creating the 51st answers `ERROR_LIMIT_REACHED` (409). `note-collection/get-list` (current route name) is paged with the standard sealed cursor (`limit` 1..100, default 50), newest first; the web list follows the existing "load more" pattern.
2. **Separate contributor quota.** Contributor photos count toward a per-yearbook contributor quota (constant: 600 photos, 1 GiB of stored bytes across all variants) and **not** toward the owner's quota; the owner's quota counts all variant bytes it stores (display, thumb, print), not only display. Over the contributor quota the public submit answers `ERROR_QUOTA_EXCEEDED` with a message the friend understands; the note text can still be sent without photos.
3. **Docs.** `docs/media.md` (both quotas), `docs/moderation.md` (link cap), `api/openapi.yaml`, `schema.d.ts`, Postman.

#### Acceptance criteria
- [ ] AC1 — Link cap and paging tests (51st refused; pages stable across inserts).
- [ ] AC2 — A link holder who sends 600 photos cannot reduce the owner's remaining quota by one; the owner's quota now includes print and thumb bytes (existing quota tests updated with the new numbers).
- [ ] AC3 — `make lint test test-integration`, Postman and e2e green.

#### Risk
`high`: quotas on a public path. No production data, so no backfill.
