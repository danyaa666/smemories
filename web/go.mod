// Keeps Go tooling (go ./..., golangci-lint, govulncheck) out of web/node_modules, which
// ships a few stray .go files. This is not a Go module we build.
module smemories.invalid/web

go 1.26.0
