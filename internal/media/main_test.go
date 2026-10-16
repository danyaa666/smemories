package media

import (
	"flag"
	"os"
	"testing"
)

// T-079: every parallel test here decodes images and holds a DB pool, so the test binary's memory grows with -parallel
// (about 1.2 GB at 4, 1.5-1.8 GB at 11, 0.8 GB at 1). Cap it at 4 unless the caller passed -parallel explicitly.
func TestMain(m *testing.M) {
	set := false
	flag.Visit(func(f *flag.Flag) { set = set || f.Name == "test.parallel" })
	if !set {
		_ = flag.Set("test.parallel", "4")
	}
	os.Exit(m.Run())
}
