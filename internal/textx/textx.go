// Package textx holds the text rules shared by every user-entered field (decision L-09).
package textx

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const zwj = '\u200d'

// Clean normalises s to NFC, trims it and reports whether it has min..max characters (runes) and no
// control characters (Cc), no format characters (Cf: zero-width space, bidi overrides/isolates,
// LRM/RLM, BOM) except U+200D (ZWJ, used in emoji sequences), and no line/paragraph separators
// (Zl, Zp). Invalid UTF-8 is rejected.
func Clean(s string, min, max int) (string, bool) { return clean(s, min, max, false) }

// CleanMultiline is Clean for long text: "\r\n" becomes "\n" and "\n" is allowed inside the text.
// Any other control character, including a lone "\r", is still rejected.
func CleanMultiline(s string, min, max int) (string, bool) {
	return clean(strings.ReplaceAll(s, "\r\n", "\n"), min, max, true)
}

func clean(s string, min, max int, multiline bool) (string, bool) {
	s = strings.TrimSpace(norm.NFC.String(s))
	n := utf8.RuneCountInString(s)
	if n < min || n > max {
		return "", false
	}
	for _, r := range s {
		if unicode.IsControl(r) && (!multiline || r != '\n') || r == utf8.RuneError || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) && r != zwj {
			return "", false
		}
	}
	return s, true
}
