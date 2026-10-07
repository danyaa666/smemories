# Third-party notices

Licences of third-party material committed to or used by this repository. Update this file when a dependency or asset is added.

## Fonts (committed, SIL Open Font License 1.1)

| File | Source | Licence text |
|---|---|---|
| `internal/pdf/fonts/BeVietnamPro-Regular.ttf`, `BeVietnamPro-Bold.ttf` | Be Vietnam Pro, Copyright 2021 The Be Vietnam Pro Project Authors (https://github.com/bettergui/BeVietnamPro), via google/fonts | `internal/pdf/fonts/OFL-BeVietnamPro.txt` |
| `internal/pdf/fonts/NotoEmoji-Regular.ttf` | Noto Emoji, Copyright 2013 Google LLC, via google/fonts. Modified: static instance at wght=400, subset to emoji ranges, extra Private Use `cmap` aliases (see `internal/pdf/fonts/README.md`). No Reserved Font Name is declared. | `internal/pdf/fonts/OFL-NotoEmoji.txt` |

## Go modules used by the PDF spike (T-005, throwaway)

| Module | Licence | Role |
|---|---|---|
| `codeberg.org/go-pdf/fpdf` v0.12.0 | MIT | Candidate PDF engine (chosen in ADR 0002) |
| `github.com/signintech/gopdf` v0.38.1 | MIT | Candidate PDF engine (not chosen) |
| `github.com/phpdave11/gofpdi`, `github.com/pkg/errors` | MIT, BSD-2-Clause | Dependencies of the two engines |
| `golang.org/x/image`, `golang.org/x/text`, `golang.org/x/sys` | BSD-3-Clause | Glyph lookup, NFC normalisation |
| `github.com/klippa-app/go-pdfium` v1.21.1 | MIT | Spike-only text extraction and rendering (PDFium, BSD-3-Clause / Apache-2.0, as WebAssembly) |
| `github.com/tetratelabs/wazero`, `github.com/jolestar/go-commons-pool/v2` | Apache-2.0 | Dependencies of go-pdfium |
| `github.com/google/uuid` | BSD-3-Clause | Dependency of go-pdfium |
| `github.com/pdfcpu/pdfcpu` v0.16.1 | Apache-2.0 | Spike-only validator, run with `go install`, not a module dependency |

Excluded by decision D-06: `unipdf` (AGPL or commercial licence).
