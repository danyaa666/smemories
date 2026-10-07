# T-005 — Spike: choose the pure-Go PDF engine

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M0 · **Risk:** low · **Priority:** P1 · **Type:** feature

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Description
Spike: decide which pure-Go PDF library SMemories uses (board D-06: the owner chose a pure-Go engine over headless Chromium). Everything in M1 depends on this verdict, so it runs first and ends with a written go/no-go backed by measurements. Candidates (both MIT): `github.com/go-pdf/fpdf` and `github.com/signintech/gopdf`. `unipdf` is excluded (AGPL/commercial licence) unless the owner approves a licence cost.

#### Scope
- In: prototype for each candidate under `internal/pdf/spike/` (throwaway, marked as such); measurements; `docs/adr/0002-pdf-engine.md`; font and licence files.
- Out (do not do): template spec (T-010), HTTP endpoints, database, S3, production-quality code, any real person's photo (the repo is public — generate synthetic gradient/noise images).

#### Acceptance criteria
- [ ] AC1 — `docs/adr/0002-pdf-engine.md` has a table scoring **both** libraries against C1–C7 below (pass/fail plus the measured value), a short rationale, and the verdict `GO <library>` or `NO-GO`.
- [ ] AC2 — C1 Vietnamese: with an embedded OFL TTF that has full Vietnamese coverage (for example Be Vietnam Pro or Noto Sans; commit font + licence, < 2 MB total), the string `Chúc mừng tốt nghiệp! Đặng Thị Hồng, Nguyễn Quỳnh Phương, Trần Văn Ưu` renders with every diacritic visible (PNG screenshot committed under `docs/adr/0002-assets/`, each < 300 KB) and text extracted from the PDF by a Go extractor in the test equals the NFC input.
- [ ] AC3 — C2 Fallback and emoji: demonstrate per-rune font fallback (primary Vietnamese/Latin font → monochrome emoji font such as Noto Emoji, OFL) rendering `🎓🎉❤` as outlines, or document the exact workaround and its cost. A rune present in no font must not panic: show how it is detected so it can be reported.
- [ ] AC4 — C3 Images: a 4000×3000 JPEG placed full-bleed on A5 (148×210 mm) with centred `cover` cropping reaches ≥ 300 effective DPI in the frame; when no crop is needed the JPEG bytes are not re-encoded (PDF size ≈ source size); a PNG with alpha renders correctly.
- [ ] AC5 — C4 Layout primitives needed by templates: page sizes A5 and A4, filled rectangles, rounded-rectangle or circular image clipping (or a documented alternative), rotation of an image and of text by an angle, text wrapping inside a box with left/centre alignment and line-height control, text colour and opacity.
- [ ] AC6 — C5 Performance: a 40-page A5 book with 40 photos (4000×3000) — report wall time and peak RSS for each library and the machine used (targets ≤ 10 s and ≤ 512 MB).
- [ ] AC7 — C6 Licence and health: licence of each library and font is MIT/BSD/Apache/OFL (no AGPL or commercial); latest release date and maintenance status noted; `THIRD_PARTY_NOTICES.md` created.
- [ ] AC8 — C7 Validity: every sample PDF opens in macOS Preview and Chrome and passes `pdfcpu validate` (or an equivalent validator) without warnings.
- [ ] AC9 — The verdict is also posted as a board comment. On `NO-GO`, list the failed criteria with the smallest workaround for each, or recommend reopening headless Chromium (D-06 revisit trigger); the leader then asks the owner.

#### Design
Files: `internal/pdf/spike/{fpdf,gopdf}_test.go` with build tag `spike`, `internal/pdf/spike/README.md` ("throwaway — replaced by T-010"), `make spike` target running them, `docs/adr/0002-pdf-engine.md`, `docs/adr/0002-assets/*.png`, `THIRD_PARTY_NOTICES.md`, font files under `internal/pdf/fonts/`.

Decision rule the ADR applies: a library passes if C1, C2 (or an acceptable workaround), C3, C4, C6, C7 pass and C5 is within 2× of target. If both pass, prefer the one with fewer dependencies and better text-wrapping support. Normalise all text to NFC before drawing.

#### Risk
`low` — throwaway prototype, no production dependency yet (the chosen library lands in T-010, which is `high`).

#### Security & performance notes
Only trusted synthetic fixtures. Photos for the benchmark must be generated, never real people. Measure on idle hardware and say so.

#### Test plan
- Dev: `make spike` runs both prototypes and prints the numbers that go into the ADR.
- QA should probe: re-run `make spike` on their machine and compare against the ADR within a reasonable margin; open each generated PDF and the PNGs and check diacritics and emoji by eye; check the licence claims against each repository.
