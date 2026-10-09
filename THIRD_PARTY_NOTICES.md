# Third-party notices

Licences of third-party material committed to or used by this repository. Update this file when a dependency or asset is added.

## Fonts (committed, SIL Open Font License 1.1)

| File | Source | Licence text |
|---|---|---|
| `internal/pdf/fonts/BeVietnamPro-Regular.ttf`, `BeVietnamPro-Bold.ttf` | Be Vietnam Pro, Copyright 2021 The Be Vietnam Pro Project Authors (https://github.com/bettergui/BeVietnamPro), via google/fonts | `internal/pdf/fonts/OFL-BeVietnamPro.txt` |
| `internal/pdf/fonts/NotoEmoji-Regular.ttf` | Noto Emoji, Copyright 2013 Google LLC, via google/fonts. Modified: static instance at wght=400, subset to emoji ranges, extra Private Use `cmap` aliases (see `internal/pdf/fonts/README.md`). No Reserved Font Name is declared. | `internal/pdf/fonts/OFL-NotoEmoji.txt` |

## Go modules used by the PDF renderer

| Module | Licence | Role |
|---|---|---|
| `codeberg.org/go-pdf/fpdf` v0.12.0 | MIT | PDF engine (ADR 0002) |
| `golang.org/x/image`, `golang.org/x/text` | BSD-3-Clause | Glyph lookup for font fallback, NFC normalisation |

Excluded by decision D-06: `unipdf` (AGPL or commercial licence).

## Redis client and server (T-051, decision D-23)

| Item | Licence | Role |
|---|---|---|
| `github.com/redis/go-redis/v9` v9.23.0 | BSD-2-Clause | Redis-protocol client (`internal/redis`) |
| `github.com/cespare/xxhash/v2`, `go.uber.org/atomic` (indirect) | MIT | go-redis dependencies |
| Valkey 8 (`valkey/valkey` image, local stack and CI only, not shipped) | BSD-3-Clause | Redis-protocol server |

## Contract lint (T-063)

| Item | Licence | Role |
|---|---|---|
| `gopkg.in/yaml.v3` v3.0.1 | MIT and Apache-2.0 | Test-only: parses `api/openapi.yaml` in `api/contract_lint_test.go`; not linked into the API binary |
