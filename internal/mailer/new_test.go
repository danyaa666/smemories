package mailer_test

import (
	"bytes"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/mailer"
)

func TestNewPicksTheImplementation(t *testing.T) {
	awsEnv(t)
	logger := slog.New(slog.DiscardHandler)
	var out bytes.Buffer

	for _, provider := range []string{"", "log"} {
		m, err := mailer.New(mailer.Config{Provider: provider, Env: "test", LogOut: &out}, logger)
		if err != nil {
			t.Fatalf("provider %q: %v", provider, err)
		}
		if _, ok := m.(*mailer.LogMailer); !ok {
			t.Errorf("provider %q: got %T", provider, m)
		}
	}
	if _, err := mailer.New(mailer.Config{Provider: "log", Env: "prod", LogOut: &out}, logger); err == nil {
		t.Error("the log mailer started in prod")
	}
	m, err := mailer.New(mailer.Config{Provider: "ses", From: "SMemories <no-reply@mail.smemories.io>", Env: "prod"}, logger)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*mailer.SES); !ok {
		t.Errorf("ses: got %T", m)
	}
	if _, err := mailer.New(mailer.Config{Provider: "ses", Env: "prod"}, logger); err == nil {
		t.Error("ses without a sender started")
	}
	for _, bad := range []string{"smtp", "SES", "log,ses"} {
		if m, err := mailer.New(mailer.Config{Provider: bad, From: "a@mail.smemories.io", Env: "test"}, logger); err == nil || m != nil || !strings.Contains(err.Error(), "SMEM_MAIL_PROVIDER") {
			t.Errorf("provider %q: %v %v", bad, m, err)
		}
	}
}

// AC8: the seam is real. Outside the SES file no code imports the SESv2 package or names an SES error, and only the loader
// (internal/config) and mailer.New read SMEM_MAIL_PROVIDER.
func TestProviderStaysBehindTheSeam(t *testing.T) {
	root := filepath.Join("..", "..")
	allowSES := map[string]bool{"internal/mailer/ses.go": true, "internal/mailer/ses_test.go": true}
	sesNames := []string{"service/sesv2", "MessageRejected", "AccountSuspendedException", "SendingPausedException", "TooManyRequestsException"}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" || strings.HasPrefix(rel, ".team") || d.Name() == "worktrees" {
				return filepath.SkipDir
			}
			return nil
		}
		inScope := strings.HasPrefix(rel, "internal/") || strings.HasPrefix(rel, "cmd/")
		if !strings.HasSuffix(rel, ".go") || !inScope || strings.HasSuffix(rel, "new_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(b)
		if !allowSES[rel] {
			for _, n := range sesNames {
				if strings.Contains(src, n) {
					t.Errorf("%s names %q: provider details belong in internal/mailer/ses.go", rel, n)
				}
			}
		}
		if strings.Contains(src, `"SMEM_MAIL_PROVIDER"`) && rel != "internal/config/config.go" && !strings.HasSuffix(rel, "_test.go") {
			t.Errorf("%s reads SMEM_MAIL_PROVIDER: only the loader does, and mailer.New switches on the value", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
