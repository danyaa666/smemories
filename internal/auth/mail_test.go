package auth

import (
	"strings"
	"testing"
)

// AC6: both languages carry the code, its lifetime and the name, and no link; an unknown locale falls back to English.
func TestRenderEmailBothLanguages(t *testing.T) {
	const code = "042917"
	for _, purpose := range []string{purposeVerify, purposeReset} {
		en, enText, err := renderEmail("en", purpose, "Alice", code, 15)
		if err != nil {
			t.Fatal(err)
		}
		vi, viText, err := renderEmail("vi", purpose, "Hồng", code, 15)
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
		if fb, _, err := renderEmail("fr", purpose, "X", code, 15); err != nil || fb != en {
			t.Errorf("%s: unknown locale should fall back to English, got %q, %v", purpose, fb, err)
		}
	}
}
