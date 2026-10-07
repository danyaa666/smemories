# PDF engine spike (T-005) - THROWAWAY

Prototypes of `go-pdf/fpdf` and `signintech/gopdf` used only to write `docs/adr/0002-pdf-engine.md`.
Replaced by the real renderer in T-010; delete this directory (and `go mod tidy`) when that lands.

Everything is behind the `spike` build tag, so `go build ./...` and `go test ./...` ignore it.

    make spike     # fixtures, both libraries, numbers, validation, PNG screenshots

Outputs go to `internal/pdf/spike/out/` (git-ignored). Photos are synthetic (generated at run time);
never put a real person's photo in this repository.
