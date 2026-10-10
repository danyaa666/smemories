# E05_T-075 — Media backfill hardening

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt
**Depends on:** T-052, T-057 (both merged).

#### Description
QA of T-057 found small gaps in `cmd/smemories-media-backfill` and `internal/media/backfill.go`. None blocks the print work; all matter before the backfill runs against production (T-022/T-023).

#### Requirements (SHALL)
1. The backfill binary SHALL load config with `config.LoadMigrate` (as `smemories-migrate` does after T-052): it needs the database and object-store settings only, not `SMEM_REDIS_URL`, `SMEM_OTP_KEY` or `SMEM_ALLOWED_ORIGINS`. `docs/media.md` says which variables it needs; T-022's task definition gets the same list.
2. A run SHALL stop with a non-zero exit after N consecutive object-store failures (flag `--max-consecutive-failures`, default 20), so an S3 outage does not walk every photo at about 2.5 s each; photos that fail on their own (corrupt, missing) do not count unless consecutive storage errors.
3. `--dry-run` SHALL NOT count photos whose display object is corrupt or missing as "would create" (it reads and decodes the display object but writes nothing, or it labels the number an upper bound; dev chooses and documents).
4. The Postman print request SHALL upload a photo larger than 1800 px and assert the print object's long edge is 1800.
5. A delete racing the backfill leaves one orphan print object until the yearbook is deleted: document it in `docs/media.md` (accepted) or close the window with a re-check after `setPrintKey` returns false (delete the object just written). Dev chooses; the re-check is preferred.
6. A synthetic alpha PNG that is larger as print than as display (QA observed): when the print encoding is not smaller than the display bytes, store the display bytes as the print object (same rule as photos not larger than 1800 px).

#### Acceptance criteria
- [ ] AC1 — Each requirement has a test (unit or integration) or a documented manual check; `make lint build test test-integration` green.
- [ ] AC2 — The backfill still runs twice with identical results (idempotent) on the T-057 fixture of 30 legacy-style photos.
