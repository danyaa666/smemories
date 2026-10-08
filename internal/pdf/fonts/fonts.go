// Package fonts embeds the OFL fonts used by the PDF renderer (see README.md in this directory) and keeps
// the registry of font families a template may name.
package fonts

import (
	_ "embed" // for go:embed
	"fmt"
	"slices"
	"unicode"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/text/unicode/norm"
)

// Primary is Be Vietnam Pro, full Vietnamese coverage.
//
//go:embed BeVietnamPro-Regular.ttf
var Regular []byte

//go:embed BeVietnamPro-Bold.ttf
var Bold []byte

// Emoji is the prepared monochrome Noto Emoji (static instance with BMP aliases for U+1F000..U+1FAFF);
// do not swap in an unmodified upstream file (ADR 0002).
//
//go:embed NotoEmoji-Regular.ttf
var Emoji []byte

// Family is one font family: TTF data for the regular and the bold face. A family may have only one of them
// (the renderer then uses that face for both weights).
type Family struct {
	Regular, Bold []byte
	Key           string // family name inside the PDF; default "f-" + the registry name
}

// Face returns the TTF data for a weight, falling back to the other face when the family has only one.
// bold reports whether the returned data is the bold face.
func (f Family) Face(bold bool) (data []byte, isBold bool) {
	switch {
	case bold && f.Bold != nil:
		return f.Bold, true
	case f.Regular != nil:
		return f.Regular, false
	default:
		return f.Bold, true
	}
}

var registry = map[string]Family{}

// Key "bvp" is the name the PDFs always used, which keeps the sample PDFs byte-stable.
func init() { Register("BeVietnamPro", Family{Regular: Regular, Bold: Bold, Key: "bvp"}) }

// Register adds a family. It panics when a face is not a valid TTF or lacks any Vietnamese letter with its
// tone marks, so an unusable family can never be named by a template. Call it from init only: the registry
// is not locked (ponytail: add a mutex if families are ever registered at run time).
func Register(name string, f Family) {
	if name == "" || (f.Regular == nil && f.Bold == nil) {
		panic("fonts: family needs a name and at least one face")
	}
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("fonts: family %q registered twice", name))
	}
	for _, face := range [][]byte{f.Regular, f.Bold} {
		if face == nil {
			continue
		}
		if err := CheckVietnamese(face); err != nil {
			panic(fmt.Sprintf("fonts: family %q: %v", name, err))
		}
	}
	registry[name] = f
}

// Lookup returns a registered family.
func Lookup(name string) (Family, bool) {
	f, ok := registry[name]
	return f, ok
}

// Names lists the registered families in alphabetical order.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// Vietnamese letters: the base vowels (with their own marks) take the five tone marks, and the consonants
// (d with stroke included) stand alone. Every letter is checked in upper and lower case, composed (NFC) as
// the renderer normalises text.
const (
	vnVowels     = "aăâeêioôơuưy"
	vnConsonants = "bcdđghklmnpqrstvx"
)

var vnTones = []rune{0, 0x0300, 0x0309, 0x0303, 0x0301, 0x0323} // none, grave, hook, tilde, acute, dot below

// CheckVietnamese reports the first Vietnamese letter the font cannot draw.
func CheckVietnamese(ttf []byte) error {
	f, err := sfnt.Parse(ttf)
	if err != nil {
		return fmt.Errorf("not a usable TTF: %w", err)
	}
	var buf sfnt.Buffer
	has := func(s string) error {
		for _, r := range norm.NFC.String(s) { // a letter may stay two runes if it has no composed form
			if idx, err := f.GlyphIndex(&buf, r); err != nil || idx == 0 {
				return fmt.Errorf("no glyph for %q (%U)", s, r)
			}
		}
		return nil
	}
	for _, c := range vnConsonants {
		for _, r := range []rune{c, unicode.ToUpper(c)} {
			if err := has(string(r)); err != nil {
				return err
			}
		}
	}
	for _, v := range vnVowels {
		for _, base := range []rune{v, unicode.ToUpper(v)} {
			for _, tone := range vnTones {
				s := string(base)
				if tone != 0 {
					s += string(tone)
				}
				if err := has(s); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
