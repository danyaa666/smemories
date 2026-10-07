# ADR 0002: PDF engine

Status: proposed by spike T-005, 2026-10-07 (the leader accepts it by merging the PR).
Context: board decision D-06 (pure-Go PDF engine instead of headless Chromium).

## Verdict

**GO `codeberg.org/go-pdf/fpdf` v0.12.0.**

Both candidates pass every criterion, so the decision rule applies: prefer fewer dependencies and better text wrapping.
Dependencies are a tie (each adds `gofpdi` + `pkg/errors`; fpdf also `x/image`). Text wrapping favours fpdf
(`MultiCell` takes a real line height and alignment; gopdf's has no line-height control and a broken `Border`).
fpdf also used less memory (442 MB vs 539 MB). The price is two fpdf quirks that T-010 must design around
(emoji font aliases and a hard panic on runes above U+FFFF, see "What T-010 must do").

`gopdf` (v0.38.1) is a close second and the fallback if fpdf's quirks bite: it draws supplementary-plane emoji
natively and has a built-in glyph check.

Not considered: `unipdf` (AGPL / commercial licence, excluded by D-06). Import path note: the GitHub repository
`github.com/go-pdf/fpdf` is **archived** (last release v0.9.0, Sept 2023). Development continues on Codeberg
under `codeberg.org/go-pdf/fpdf` (v0.12.0 released 2026-05-18, last commit 2026-09-14). Always import the Codeberg path.

## Scores

Measured on an Apple M3 Pro (11 CPUs), macOS (Darwin 24.6.0), Go 1.26.1, load average about 3.4 (not a
perfectly idle machine). Reproduce with `make spike`. Fixtures are synthetic (`internal/pdf/spike/common_test.go`).

| # | Criterion | fpdf v0.12.0 | gopdf v0.38.1 |
|---|---|---|---|
| C1 | Vietnamese, embedded Be Vietnam Pro (OFL), NFC | **pass**: all diacritics visible ([screenshot](0002-assets/fpdf-text.png)); text extracted by PDFium (pure Go, via go-pdfium) equals the NFC input; NFD input normalised to NFC comes out identical | **pass**: same ([screenshot](0002-assets/gopdf-text.png)) |
| C2 | Per-rune fallback, emoji `🎓🎉❤` as outlines, unknown rune | **pass with workaround**: fallback is our own `splitRuns` (the same code for both libs). fpdf reads only the BMP `cmap` and **panics** on any rune above U+FFFF, so non-BMP emoji go through PUA aliases baked into the emoji font (`toPUA`). Extraction never returns the real 🎓🎉 (PUA code points with PDFium, nothing with pdftotext; ❤ is exact). | **pass**: draws all three natively. Minor defect: its `ToUnicode` map is wrong above U+FFFF (extraction returns `ἹἸ` for 🎓🎉), harmless for printing. |
| C2 | Rune present in no font | detected before drawing by `splitRuns` (uses `x/image/font/sfnt` `GlyphIndex == 0`), drawn as `?` and reported to the caller. Unguarded, a BMP rune is silently skipped (no panic; the blank `x  x` line in `fpdf-text.png` is this deliberate unguarded CJK demo, not a bug); a non-BMP rune panics | detected the same way (or `IsCurrFontContainGlyph`). Unguarded, silently skipped (no error, no panic) |
| C3 | 4000x3000 JPEG, full-bleed cover on A5 | **pass** (pitfall: a cover crop has a negative x, and fpdf silently replaces a negative x/y with the current margin/cursor unless `ImageOptions.AllowNegativePosition` is true; with it the page renders full-bleed, checked by `TestQA_CoverIsFullBleed`): 363 effective DPI (>= 300); JPEG bytes found verbatim in the PDF (not re-encoded); PDF 2,126,195 B vs source 2,088,659 B; PNG with alpha written as `/SMask`, renders correctly ([cover](0002-assets/fpdf-image-cover.png), [alpha](0002-assets/fpdf-image-alpha.png)) | **pass**: 363 DPI; verbatim JPEG; PDF 2,126,582 B; alpha via `/SMask` ([cover](0002-assets/gopdf-image-cover.png), [alpha](0002-assets/gopdf-image-alpha.png)) |
| C4 | Layout primitives ([fpdf](0002-assets/fpdf-layout.png), [gopdf](0002-assets/gopdf-layout.png)) | **pass**: A5 and A4 (see note), filled rects, `ClipRoundedRect` / `ClipCircle` (true curves), rotated image and text, `MultiCell` with line height + left/centre, text colour, `SetAlpha` (applies to text) | **pass with workarounds**: custom `Rect` page sizes; clip is polygon only (rounded rect and circle approximated with 24-step arcs, 10 lines of helper); `MultiCell` has no line-height control and its `Border` draws a box per line, so wrap with `SplitTextWithWordWrap` and place lines ourselves (about 8 lines); word wrap needs `BreakOption`; opacity only via `CellOption.Transparency` (`SetTransparency` did not affect text); rotation sign is opposite to fpdf |
| C5 | 40 A5 pages, 40 photos 4000x3000 (about 2 MB each), rounded clip + caption per page | **pass**: 0.12 s wall, 442 MB peak RSS (target <= 10 s, <= 512 MB), 83.7 MB PDF | **pass**: 0.09 s, 539 MB (4% over the RSS target, inside the 2x tolerance), 83.7 MB PDF |
| C6 | Licence and health | **pass**: MIT. v0.12.0 on 2026-05-18, commits until 2026-09-14, 36 open issues. Caveat: moved to Codeberg, small community (29 stars there), GitHub copy archived | **pass**: MIT. v0.38.1 (tag), commits until 2026-09-12, 123 open issues, 2.9k stars |
| C7 | Valid PDFs | **pass**: `pdfcpu v0.16.1 validate --mode strict` "validation ok" on all 5 PDFs; PDFium (the engine Chrome uses) opens and renders them; macOS Quick Look (PDFKit, Preview's renderer) thumbnails checked by eye for the text and layout PDFs | **pass**: same, 5 of 5 |

Notes on the numbers:
- Peak RSS is `getrusage` of a separate process per library, measured before PDFium starts, with all 40 source JPEGs
  held in memory (about 84 MB) plus the 84 MB output buffer. Production will downscale photos to print size first
  (A5 at 300 DPI is about 1750x2480), so real figures will be lower. Both libraries copy JPEGs through without decoding,
  which is why wall time is a fraction of a second.
- A5: fpdf's built-in `"A5"` is 148.5 x 210 mm (420.94 pt wide) and gopdf's `PageSizeA5` is 420x595 pt (both slightly off 148 x 210 mm).
  Use exact custom sizes (`fpdf.NewCustom`, `gopdf.Rect{W:148,H:210}` with `UnitMM`). The spike asserts MediaBox 419.53 x 595.28 pt.
- The `x/image` `sfnt` parser, not the PDF library, is what tells us which font has a glyph. We need it for fallback either way.

## Rationale

The product needs: correct Vietnamese, emoji in friends' notes (messages are user content, so emoji will happen),
photo placement without quality loss, rounded and circular frames, rotation, and text boxes with controllable line
height. fpdf gives all of that directly; gopdf needs more glue for text boxes and clips. The one hard fpdf hazard,
the panic on runes above U+FFFF, is contained in a single place (the run splitter) that we need anyway for fallback.

## What T-010 must do (consequences)

1. Normalise every string to NFC before drawing (`golang.org/x/text/unicode/norm`).
2. Split text into runs with `splitRuns` logic (font priority: Be Vietnam Pro, then Noto Emoji). Never pass a rune above
   U+FFFF to fpdf: emoji go through `toPUA` (U+1F000..U+1FAFF to U+E000..U+EAFF); anything else not found in a font is
   replaced by `?` and **reported** (log it with the note id, show it in the editor).
3. Ship `internal/pdf/fonts/NotoEmoji-Regular.ttf` as prepared here (static wght 400 instance, subset, with PUA aliases,
   see `internal/pdf/fonts/README.md`). Do not swap in an unmodified Noto Emoji font without redoing the aliases.
   The emoji are monochrome outlines (colour emoji would need images; out of scope).
4. Keep spaces in the primary font (an emoji-font space is 1.27 em wide and shows as a gap).
5. Place photos with `ClipRoundedRect`/`ClipCircle` + cover rectangle (`coverRect` in the spike is the math). Pass JPEGs as
   is when no resize is needed; that keeps files small and fast. Always pass `AllowNegativePosition: true` in the
   `ImageOptions` of every cover-cropped image: the crop starts at a negative x or y and fpdf otherwise draws the
   image at the margin instead (white strip, wrong crop, no error). Pin it with a rendered-page test like `TestQA_CoverIsFullBleed`.
6. Use `codeberg.org/go-pdf/fpdf`; the Go directive becomes `go 1.26.0` (the library requires it).
7. Add a test pinning the non-BMP panic (`TestFpdf_NonBMPRunePanics`): if a future fpdf fixes it, the aliases can go.
8. Text extraction of PDFs is not a product feature; do not rely on emoji extraction.
9. Delete `internal/pdf/spike/` and its dev-only dependencies (`go-pdfium`, `wazero`, `gopdf`) with `go mod tidy`; keep `x/image` and `x/text`.

## Revisit when

fpdf stops getting releases for a year, or a template needs something it cannot do (for example colour emoji,
complex scripts needing shaping, or CMYK/bleed for print shops). Then try gopdf or reopen headless Chromium (D-06).

## Reproduce

`make spike` (needs Go, network on first run to download modules). Dev-only dependencies of the spike: `go-pdfium`
(PDFium compiled to WebAssembly, run by `wazero`; MIT/Apache) as the Go text extractor and renderer, and `pdfcpu`
(Apache-2.0, run via `go install`) as the validator. `ledongthuc/pdf` was tried first and rejected: its `ToUnicode`
range decoding drops the high byte, so it mis-reads valid fpdf output (pdftotext and PDFium read it correctly).
