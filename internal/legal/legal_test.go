package legal

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// frontVersion returns the `version:` value of the YAML front matter of the file.
func frontVersion(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	if !sc.Scan() || sc.Text() != "---" {
		t.Fatalf("%s: no front matter", path)
	}
	for sc.Scan() && sc.Text() != "---" {
		if v, ok := strings.CutPrefix(sc.Text(), "version:"); ok {
			return strings.TrimSpace(v)
		}
	}
	t.Fatalf("%s: no version in the front matter", path)
	return ""
}

func TestAllDocsCarryTheCurrentVersion(t *testing.T) {
	for _, name := range []string{"terms.en.md", "terms.vi.md", "privacy.en.md", "privacy.vi.md"} {
		if got := frontVersion(t, filepath.Join("..", "..", "docs", "legal", name)); got != CurrentVersion {
			t.Errorf("%s: version %q, want %q (edit all four texts and bump CurrentVersion together)", name, got, CurrentVersion)
		}
	}
}

func TestFrontVersionReadsTheValue(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.md")
	if err := os.WriteFile(p, []byte("---\ndocument: terms\nversion: 2030-01-01\n---\n# T\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := frontVersion(t, p); got != "2030-01-01" || got == CurrentVersion {
		t.Fatalf("got %q", got)
	}
}

// T-131 (E07): the class feature changed what is shown to whom, so the version moved on from 2026-10-21 and the four texts say who
// sees what in a class. Removing the class text, or going back to the old version, must fail here.
func TestClassDataIsInTheTextsAndTheVersionMovedOn(t *testing.T) {
	if CurrentVersion <= "2026-10-21" {
		t.Fatalf("CurrentVersion %q is not later than 2026-10-21, the last version without class data", CurrentVersion)
	}
	for name, words := range map[string][]string{
		"privacy.en.md": {"Class admins", "approved notes", "never see the other students"},
		"privacy.vi.md": {"Quản trị viên lớp", "lời nhắn đã duyệt", "không bao giờ thấy trang"},
		"terms.en.md":   {"## 6. Classes", "class admin can see", "Students never see each other"},
		"terms.vi.md":   {"## 6. Lớp học", "quản trị viên lớp xem được", "Học sinh không bao giờ thấy"},
	} {
		b, err := os.ReadFile(filepath.Join("..", "..", "docs", "legal", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range words {
			if !strings.Contains(string(b), w) {
				t.Errorf("%s does not contain %q", name, w)
			}
		}
	}
}
