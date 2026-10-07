# T-036 — Bound the memory of image processing

**Epic:** E03-yearbooks · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1

#### Description
QA measured the T-009 upload pipeline: one 4000×3000 JPEG leaves about 430 MB resident, and four parallel 49-megapixel PNGs (47 KB each, inside every
current limit) peaked at 3.2 GB at the default `SMEM_MEDIA_MAX_CONCURRENT=4`. A 1–2 GB Fargate task (M2) would be killed by four hostile uploads. T-034 opens the
same pipeline to anonymous contributors (rate-limited, but unauthenticated), so the ceiling must be defined and enforced first. This task makes the pipeline's worst-case
memory a number we chose, not an accident. Decisions: D-08, L-06. Source: T-009 QA and leader review.

#### Scope
- In: measure the worst-case peak per request for 8-bit RGBA PNG, 16-bit PNG, 50 MP JPEG and large WebP; set defaults so that concurrency × worst case stays under a budget
  (target: peak RSS ≤ 1.2 GiB with 4 parallel worst-case uploads, so a 2 GiB task has headroom); a Go memory limit knob; docs; `go.mod` tidy.
- Out (do not do): tiled or streaming decoding, switching to libvips/cgo (would be an owner decision), changing the stored output format, new features.

#### Acceptance criteria
- [ ] AC1 — Measured and written down in `docs/media.md` (new, short): per-format peak memory of `process` for the worst-case inputs, the chosen limits and why.
- [ ] AC2 — Limits enforced before the full decode, from the header (`image.DecodeConfig`): a per-format pixel cap (for example PNG/WebP lower than JPEG, because a JPEG decodes to
  about 1.5 bytes per pixel and a 16-bit PNG to 8) and a cap on decoded bytes computed from the header's colour model; over the cap → `400 invalid_image`. Tests cover each cap just
  below and just above.
- [ ] AC3 — `SMEM_MEDIA_MAX_CONCURRENT` default lowered if the measurement requires it (2 is the expected value); a new `SMEM_MEMORY_LIMIT_MIB` (0 = unset) calls `debug.SetMemoryLimit`;
  both are documented in `.env.example` and the README config paragraph. Invalid values fail at startup with a clear message.
- [ ] AC4 — Large intermediates are released as early as possible (`img` not referenced after the resize; no second full-size copy), shown by the measurement in AC1.
- [ ] AC5 — `go mod tidy` leaves no diff, and `github.com/aws/aws-sdk-go-v2`, `…/credentials` and `…/service/s3` are direct requirements (they are marked `// indirect` today).
- [ ] AC6 — QA reproduces the 4 × worst-case run and reports peak RSS ≤ 1.2 GiB at the defaults.

#### Design
Files: `internal/media/image.go`, `internal/media/service.go`, `internal/config/config.go`, `cmd/smemories-api/main.go`, `docs/media.md`, `.env.example`, `README.md`, `go.mod`.

Header-based budget: `DecodeConfig` gives the colour model and size; estimate decoded bytes = width × height × bytes-per-pixel(model) and refuse above the cap before allocating.
Keep the existing 12000 px side and the global pixel cap as outer bounds.

#### Risk
`low`: no new endpoint, no data migration; the change only tightens limits and adds a config knob. Not high, but QA must re-run the media Postman collection twice.

#### Security & performance notes
Memory exhaustion by a small file is the classic image-upload denial of service. The measurement, not the pixel cap alone, is what counts: record it so later changes can be compared.

#### Test plan
- Dev: unit tests for the caps (table of formats and sizes, just under and over); a measurement script or Go test that prints peak heap for the worst cases (documented in `docs/media.md`).
- QA should probe: 4 parallel worst-case uploads and watch RSS; a 12000 × 4000 JPEG (allowed) next to the same pixel count as a PNG; `SMEM_MEMORY_LIMIT_MIB` set to a small value (requests fail cleanly or are refused, the process does not crash); media Postman collection twice back to back.
