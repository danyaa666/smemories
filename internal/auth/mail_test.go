package auth

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden email files in testdata/emails")

// AC6 (T-007): both languages carry the code, its lifetime and the name, and no link; an unknown locale falls back to English.
func TestRenderEmailBothLanguages(t *testing.T) {
	const code = "042917"
	for _, purpose := range []string{purposeVerify, purposeReset} {
		en, enText, _, err := renderEmail("en", purpose, "Alice", code, 15)
		if err != nil {
			t.Fatal(err)
		}
		vi, viText, _, err := renderEmail("vi", purpose, "Hồng", code, 15)
		if err != nil {
			t.Fatal(err)
		}
		for _, txt := range []string{enText, viText} {
			if !strings.Contains(txt, code) || !strings.Contains(txt, "15") {
				t.Errorf("%s: body lacks the code or its lifetime:\n%s", purpose, txt)
			}
			if strings.Contains(txt, "http") || strings.Contains(txt, "token") {
				t.Errorf("%s: body still has link text:\n%s", purpose, txt)
			}
		}
		if !strings.Contains(enText, "Alice") || !strings.Contains(viText, "Hồng") || !strings.Contains(enText, "Do not share") || !strings.Contains(viText, "Không chia sẻ") {
			t.Errorf("%s: name or warning missing", purpose)
		}
		if en == vi || en == "" || vi == "" || strings.Contains(vi, "SMemories password") {
			t.Errorf("%s: subjects not localised: %q / %q", purpose, en, vi)
		}
		if fb, _, _, err := renderEmail("fr", purpose, "X", code, 15); err != nil || fb != en {
			t.Errorf("%s: unknown locale should fall back to English, got %q, %v", purpose, fb, err)
		}
	}
}

// T-021 AC4: the three parts of every message kind and locale are pinned by golden files (`go test ./internal/auth -run Golden -update`
// rewrites them after an intended change; review the diff, the Vietnamese is the owner's to approve).
func TestEmailGolden(t *testing.T) {
	names := map[string]string{"en": "Alice <Smith>", "vi": "Hồng"}
	for _, locale := range []string{"en", "vi"} {
		for _, purpose := range []string{purposeVerify, purposeReset} {
			subject, text, html, err := renderEmail(locale, purpose, names[locale], "042917", 15)
			if err != nil {
				t.Fatal(err)
			}
			for ext, got := range map[string]string{"subject.txt": subject + "\n", "text.txt": text, "html": html} {
				path := filepath.Join("testdata", "emails", locale+"."+purpose+"."+ext)
				if *update {
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
					continue
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%v (run with -update to create it)", err)
				}
				if string(want) != got {
					t.Errorf("%s differs from the golden file; run with -update after reviewing the change:\n--- want\n%s\n--- got\n%s", path, want, got)
				}
			}
		}
	}
}

var externalRef = regexp.MustCompile(`(?i)(src|href|action)\s*=|url\(|@import|<link|<script|<img|<iframe|https?://`)

// T-021 AC4: the HTML is small, has one h1 and a lang attribute, loads nothing, and escapes what a user typed.
func TestEmailHTMLRules(t *testing.T) {
	const code = "042917"
	for _, locale := range []string{"en", "vi"} {
		for _, purpose := range []string{purposeVerify, purposeReset} {
			_, text, html, err := renderEmail(locale, purpose, `<script>alert(1)</script>`, code, 15)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case len(html) > 20<<10:
				t.Errorf("%s.%s: %d bytes, want under 20 KB", locale, purpose, len(html))
			case strings.Count(html, "<h1") != 1:
				t.Errorf("%s.%s: want exactly one h1", locale, purpose)
			case !strings.Contains(html, `<html lang="`+locale+`">`):
				t.Errorf("%s.%s: missing the lang attribute", locale, purpose)
			case externalRef.MatchString(html):
				t.Errorf("%s.%s: the HTML references an external resource or a link: %s", locale, purpose, externalRef.FindString(html))
			case strings.Contains(html, "<script>"):
				t.Errorf("%s.%s: the display name was not escaped", locale, purpose)
			case !strings.Contains(html, code) || !strings.Contains(text, code):
				t.Errorf("%s.%s: the code is missing from a part", locale, purpose)
			case strings.Contains(html, "token") || strings.Contains(html, "unsubscribe"):
				t.Errorf("%s.%s: unexpected link or tracking text", locale, purpose)
			}
		}
	}
}
