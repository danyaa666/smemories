package textx

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	nfd := "Vie\u0302\u0323t" // V + i + e + combining circumflex + combining dot below + t
	tests := []struct {
		name     string
		in       string
		min, max int
		want     string
		ok       bool
	}{
		{"trims", "  Lan  ", 1, 10, "Lan", true},
		{"nfc", nfd, 1, 10, "Vi\u1ec7t", true},
		{"empty ok when min 0", "   ", 0, 10, "", true},
		{"empty rejected when min 1", "   ", 1, 10, "", false},
		{"max counts characters not bytes", strings.Repeat("Đ", 10), 1, 10, strings.Repeat("Đ", 10), true},
		{"one over max", strings.Repeat("Đ", 11), 1, 10, "", false},
		{"emoji counts as runes", "🎓🎓🎓", 1, 3, "🎓🎓🎓", true},
		{"zwj kept", "👩\u200d🎓", 1, 10, "👩\u200d🎓", true},
		{"newline", "a\nb", 1, 10, "", false},
		{"nul", "a\x00b", 1, 10, "", false},
		{"zero width space", "a\u200bb", 1, 10, "", false},
		{"bidi override", "a\u202eb", 1, 10, "", false},
		{"bom", "\ufeffa", 1, 10, "", false},
		{"line separator", "a\u2028b", 1, 10, "", false},
		{"invalid utf8", "a\xffb", 1, 10, "", false},
	}
	for _, tc := range tests {
		got, ok := Clean(tc.in, tc.min, tc.max)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: Clean(%q) = %q, %v; want %q, %v", tc.name, tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
