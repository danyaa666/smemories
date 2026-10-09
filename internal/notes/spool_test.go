package notes

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// AC1: only an old regular smem-upload-* file directly in the directory goes.
func TestSweepSpool(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-spoolMaxAge - time.Minute)
	write := func(name string, mt time.Time) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	write("smem-upload-x", old)
	write("smem-upload-y", time.Now())
	write("other-file", old)
	write("target", old) // an old file behind the symlink must survive too
	if err := os.Symlink(filepath.Join(dir, "target"), filepath.Join(dir, "smem-upload-z")); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "smem-upload-dir")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "smem-upload-inner"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(sub, old, old); err != nil {
		t.Fatal(err)
	}

	h := &Handler{tmpDir: dir, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if n := h.SweepSpool(); n != 1 {
		t.Fatalf("removed %d, want 1", n)
	}
	if _, err := os.Lstat(filepath.Join(dir, "smem-upload-x")); !os.IsNotExist(err) {
		t.Fatal("old spool file kept")
	}
	for _, n := range []string{"smem-upload-y", "other-file", "target", "smem-upload-z", "smem-upload-dir/smem-upload-inner"} {
		if _, err := os.Lstat(filepath.Join(dir, n)); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
}

// A missing directory is logged, not fatal.
func TestSweepSpoolMissingDir(t *testing.T) {
	h := &Handler{tmpDir: filepath.Join(t.TempDir(), "nope"), logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if n := h.SweepSpool(); n != 0 {
		t.Fatalf("removed %d", n)
	}
}
