package auth

import (
	"strings"
	"testing"
)

// AC7: both languages carry the link and the name; an unknown locale falls back to English.
func TestRenderEmailBothLanguages(t *testing.T) {
	const link = "https://app.example.com/reset-password?token=abc_DEF-123"
	for _, kind := range []string{"verify", "reset"} {
		en, enText, err := renderEmail("en", kind, "Alice", link)
		if err != nil {
			t.Fatal(err)
		}
		vi, viText, err := renderEmail("vi", kind, "Hồng", link)
		if err != nil {
			t.Fatal(err)
		}
		for _, txt := range []string{enText, viText} {
			if !strings.Contains(txt, link) {
				t.Errorf("%s: body lacks the link:\n%s", kind, txt)
			}
		}
		if !strings.Contains(enText, "Alice") || !strings.Contains(viText, "Hồng") {
			t.Errorf("%s: name missing", kind)
		}
		if en == vi || en == "" || vi == "" || strings.Contains(vi, "SMemories password") {
			t.Errorf("%s: subjects not localised: %q / %q", kind, en, vi)
		}
		if fb, _, err := renderEmail("fr", kind, "X", link); err != nil || fb != en {
			t.Errorf("%s: unknown locale should fall back to English, got %q, %v", kind, fb, err)
		}
	}
}
