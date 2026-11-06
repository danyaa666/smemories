package textx

import (
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// FuzzClean (E09 T-179): the rules behind every name and layout text. Seeds run in `go test`; explore with -fuzz=FuzzClean.
// An accepted string is valid UTF-8, NFC, trimmed, within the limits, free of control and format characters (a newline only in multiline),
// and cleaning it again changes nothing; nothing takes a second.
func FuzzClean(f *testing.F) {
	for _, s := range []string{
		"Lan", "  Lan  ", "Việt", "Việt", "Việt", "Tiếng Việt", "a\x00b", "a\u202eb", "a\u2066b\u2069", "\xed\xa0\x80", "a\xffb",
		"a\nb", "a\r\nb", "a\rb", "👩\u200d🎓", "\ufeffa", "क़", "̈́", "\U0001d15e", "\u2028", strings.Repeat("a", 1<<20), strings.Repeat("̣", 100000), "NaN", "",
	} {
		f.Add(s, 0, 60, false)
	}
	f.Add("a\nb", 1, 500, true)
	f.Fuzz(func(t *testing.T, s string, min, max int, multiline bool) {
		start := time.Now()
		var got string
		var ok bool
		if multiline {
			got, ok = CleanMultiline(s, min, max)
		} else {
			got, ok = Clean(s, min, max)
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("took %v", d)
		}
		if !ok {
			if got != "" {
				t.Fatalf("rejected with a result %q", got)
			}
			return
		}
		n := utf8.RuneCountInString(got)
		if !utf8.ValidString(got) || !norm.NFC.IsNormalString(got) || got != strings.TrimSpace(got) || n < min || n > max {
			t.Fatalf("accepted %q (%d characters, limits %d..%d)", got, n, min, max)
		}
		for _, r := range got {
			if r == '\n' && multiline {
				continue
			}
			if unicode.IsControl(r) || r == utf8.RuneError || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) && r != zwj {
				t.Fatalf("accepted %q with U+%04X", got, r)
			}
		}
		again, ok := Clean(got, min, max)
		if multiline {
			again, ok = CleanMultiline(got, min, max)
		}
		if !ok || again != got {
			t.Fatalf("not idempotent: %q -> %q (%v)", got, again, ok)
		}
	})
}

// Text typed on a Vietnamese keyboard arrives composed (Windows, most phones) or decomposed (macOS Telex, some Android keyboards): both
// become the same stored text and the same character count, which is the count every limit uses.
func TestVietnameseComposedAndDecomposedAreOneText(t *testing.T) {
	composed := "Tiếng Việt: ặ ở ữ Đ"
	decomposed := norm.NFD.String(composed)
	if len(decomposed) == len(composed) || utf8.RuneCountInString(decomposed) <= utf8.RuneCountInString(composed) {
		t.Fatal("the decomposed form is not longer: the test is not testing anything")
	}
	a, okA := Clean(composed, 1, 100)
	b, okB := Clean(decomposed, 1, 100)
	if !okA || !okB || a != b || a != composed {
		t.Fatalf("composed %q %v, decomposed %q %v", a, okA, b, okB)
	}
	// the limit counts the stored characters: 60 composed letters fit a 60 limit in either form, 61 do not
	for _, n := range []int{60, 61} {
		in := strings.Repeat("ệ", n)
		for _, form := range []string{in, norm.NFD.String(in), strings.Repeat("ệ", n), strings.Repeat("ệ", n)} {
			if _, ok := Clean(form, 1, 60); ok != (n == 60) {
				t.Errorf("%d letters as %q...: accepted=%v", n, form[:6], ok)
			}
		}
	}
}

// Control, bidi and separator characters are refused in single-line and multi-line text alike.
func TestControlAndBidiCharactersAreRefused(t *testing.T) {
	for _, r := range []rune{0, 1, 0x1f, 0x7f, 0x85, 0x200b, 0x200c, 0x200e, 0x200f, 0x202a, 0x202b, 0x202c, 0x202d, 0x202e, 0x2060, 0x2066, 0x2067, 0x2068, 0x2069, 0x2028, 0x2029, 0xfeff, 0xfffd} {
		s := "ab" + string(r) + "cd"
		if got, ok := Clean(s, 1, 50); ok {
			t.Errorf("Clean accepted U+%04X: %q", r, got)
		}
		if got, ok := CleanMultiline(s, 1, 50); ok {
			t.Errorf("CleanMultiline accepted U+%04X: %q", r, got)
		}
	}
	for _, s := range []string{"a\xffb", "\xed\xa0\x80", "\xf4\x90\x80\x80"} { // invalid UTF-8, an encoded lone surrogate, above U+10FFFF
		if _, ok := Clean(s, 0, 50); ok {
			t.Errorf("accepted invalid UTF-8 %q", s)
		}
	}
}
