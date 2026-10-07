package pdf

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/text/unicode/norm"

	"github.com/danyaa666/smemories/internal/pdf/fonts"
)

// Font indexes: the primary font, then the emoji fallback (priority order).
const (
	fontPrimary = 0
	fontEmoji   = 1
)

const (
	famPrimary = "bvp"
	famEmoji   = "emoji"
	// maxTextRunes bounds the text laid out per element (book text is untrusted).
	maxTextRunes = 20000
	puaBase      = 0xE000
	emojiLo      = 0x1F000
	emojiHi      = 0x1FAFF
)

// glyphs knows which bundled font has a glyph for a rune. Not safe for concurrent use.
type glyphs struct {
	fonts [2]*sfnt.Font
	buf   sfnt.Buffer
	memo  map[rune]glyph
}

// glyph is how one input rune is drawn: draw is the rune handed to fpdf (never above U+FFFF), font the
// font index; ok is false when no font has the rune and '?' stands in.
type glyph struct {
	draw rune
	font int
	ok   bool
}

func newGlyphs() (*glyphs, error) {
	g := &glyphs{memo: map[rune]glyph{}}
	var err error
	if g.fonts[fontPrimary], err = sfnt.Parse(fonts.Regular); err != nil {
		return nil, err
	}
	if g.fonts[fontEmoji], err = sfnt.Parse(fonts.Emoji); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *glyphs) has(font int, r rune) bool {
	idx, err := g.fonts[font].GlyphIndex(&g.buf, r)
	return err == nil && idx != 0
}

// toPUA maps the supplementary-plane emoji U+1F000..U+1FAFF to U+E000..U+EAFF, the aliases baked into
// NotoEmoji-Regular.ttf (internal/pdf/fonts/README.md). fpdf indexes glyph widths with a 64K table and
// panics on any rune above U+FFFF, so no such rune may ever reach it (ADR 0002, TestFpdf_NonBMPRunePanics).
func toPUA(r rune) rune {
	if r >= emojiLo && r <= emojiHi {
		return puaBase + r - emojiLo
	}
	return r
}

// classify picks the font for r: primary first, then emoji, else '?' with ok = false. Private-use runes are
// refused because they would hit the emoji aliases.
func (g *glyphs) classify(r rune) glyph {
	if v, ok := g.memo[r]; ok {
		return v
	}
	v := glyph{draw: '?', font: fontPrimary}
	switch {
	case r == '\n':
		v = glyph{r, fontPrimary, true}
	case r == utf8.RuneError || (r >= puaBase && r <= 0xF8FF) || r > 0x10FFFF:
	case r <= 0xFFFF && g.has(fontPrimary, r):
		v = glyph{r, fontPrimary, true}
	case (r <= 0xFFFF || (r >= emojiLo && r <= emojiHi)) && g.has(fontEmoji, r):
		v = glyph{toPUA(r), fontEmoji, true}
	}
	g.memo[r] = v
	return v
}

// prepareText NFC-normalises text and removes what has no visible form here: carriage returns, zero-width
// joiners and spaces, variation selectors and other control characters (tabs become spaces). Emoji ZWJ
// sequences therefore draw as their separate emoji. The result is cut at maxTextRunes (cut = true).
func prepareText(s string) (out string, cut bool) {
	s = norm.NFC.String(s)
	var b strings.Builder
	n := 0
	for _, r := range s {
		switch {
		case r == '\t':
			r = ' '
		case r == '\n' || r == ' ':
		case r < 0x20 || (r >= 0x7F && r < 0xA0), r >= 0x200B && r <= 0x200F, r >= 0x202A && r <= 0x202E,
			r == 0x2060, r == 0xFEFF, r >= 0xFE00 && r <= 0xFE0F:
			continue
		}
		if n == maxTextRunes {
			return b.String(), true
		}
		b.WriteRune(r)
		n++
	}
	return b.String(), false
}
