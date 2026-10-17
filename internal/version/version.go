// Package version holds the build identity of the binaries. The Dockerfile sets both through
// -ldflags "-X github.com/danyaa666/smemories/internal/version.Version=... -X ...Commit=..."; a plain `go build` reports dev.
package version

var (
	Version = "dev"
	Commit  = "unknown"
)
