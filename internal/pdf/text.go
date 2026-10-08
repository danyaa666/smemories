package pdf

import "strings"

const (
	ptMM         = 25.4 / 72 // one point in millimetres
	shrinkStep   = 0.5       // pt
	ellipsis     = '…'
	defaultLineH = 1.3
)

// unit is one drawn character with its width in em.
type unit struct {
	glyph
	em float64
}

type line struct {
	units []unit
	em    float64
}

// textBox is one text element to lay out, in page millimetres and points.
type textBox struct {
	face          *glyphs
	w, h          float64 // mm
	size, minSize float64 // pt
	lineH         float64 // multiple of the font size
	bold          bool
}

// layout is the fitted text: lines to draw at size pt.
type layout struct {
	lines     []line
	size      float64
	truncated bool
}

func (b textBox) lineMM(size float64) float64 { return size * ptMM * b.lineH }

// toUnits classifies and measures text. missing returns the runes found in no font (once each, in order).
func (r *renderer) toUnits(face *glyphs, text string, bold bool) (us []unit, missing []rune) {
	seen := map[rune]bool{}
	for _, c := range text {
		g := face.classify(c)
		if !g.ok && !seen[c] {
			seen[c] = true
			missing = append(missing, c)
		}
		u := unit{glyph: g}
		if c != '\n' {
			u.em = r.emWidth(face, g, bold)
		}
		us = append(us, u)
	}
	return us, missing
}

// emWidth is the advance of a glyph in em, measured by fpdf itself so fitting matches drawing.
func (r *renderer) emWidth(face *glyphs, g glyph, bold bool) float64 {
	k := widthKey{face.name, g.draw, g.font, bold && g.font == fontPrimary}
	if w, ok := r.widths[k]; ok {
		return w
	}
	fam, style := fontFor(face, g.font, bold)
	r.pdf.SetFont(fam, style, 10)
	w := float64(r.pdf.GetStringSymbolWidth(string(g.draw))) / 1000
	r.widths[k] = w
	return w
}

type widthKey struct {
	family string
	draw   rune
	font   int
	bold   bool
}

// fontFor is the fpdf family and style that draw a glyph of the given font index. A family with a single
// face is registered under that face's style only, so the weight asked for does not matter then.
func fontFor(face *glyphs, font int, bold bool) (family, style string) {
	if font == fontEmoji {
		return famEmoji, ""
	}
	if _, isBold := face.fam.Face(bold); isBold {
		return face.pdfFam, "B"
	}
	return face.pdfFam, ""
}

// wrap breaks units into lines no wider than maxEm: by words, and by character for words wider than a line.
// Newlines start a new line; runs of spaces collapse to one; leading and trailing spaces are dropped.
func wrap(us []unit, maxEm float64) []line {
	var lines []line
	var cur line
	var word []unit
	var wordEm float64
	pendingSpace := false
	space := unit{}
	push := func() { lines = append(lines, cur); cur = line{} }
	add := func(u unit) {
		if cur.em+u.em > maxEm && len(cur.units) > 0 {
			push()
		}
		cur.units = append(cur.units, u)
		cur.em += u.em
	}
	flushWord := func() {
		if len(word) == 0 {
			return
		}
		switch {
		case len(cur.units) == 0:
		case pendingSpace && cur.em+space.em+wordEm <= maxEm:
			add(space)
		case !pendingSpace && cur.em+wordEm <= maxEm:
		default:
			push()
		}
		for _, u := range word { // fits whole, or is broken by character when wider than a line
			add(u)
		}
		word, wordEm, pendingSpace = word[:0], 0, false
	}
	for _, u := range us {
		switch u.draw {
		case '\n':
			flushWord()
			push()
			pendingSpace = false
		case ' ':
			flushWord()
			if len(cur.units) > 0 {
				pendingSpace, space = true, u
			}
		default:
			word = append(word, u)
			wordEm += u.em
		}
	}
	flushWord()
	if len(cur.units) > 0 {
		push()
	}
	return lines
}

// fit lays text out in the box: wrap, shrink by shrinkStep down to minSize, then cut to the lines that fit
// and end the last one with an ellipsis. It never returns a line wider than the box (ponytail: a box
// narrower than one glyph is not handled; templates are trusted and validated for size).
func (r *renderer) fit(us []unit, b textBox, cutEarlier bool) layout {
	size := b.size
	for {
		lines := wrap(us, b.w/(size*ptMM))
		if float64(len(lines))*b.lineMM(size) <= b.h+1e-6 {
			return layout{lines, size, cutEarlier}
		}
		if size <= b.minSize+1e-9 {
			return r.truncate(lines, b, size)
		}
		size = max(b.minSize, size-shrinkStep)
	}
}

func (r *renderer) truncate(lines []line, b textBox, size float64) layout {
	n := max(int(b.h/b.lineMM(size)), 1)
	if len(lines) > n {
		lines = lines[:n]
	}
	maxEm := b.w / (size * ptMM)
	last := lines[len(lines)-1]
	dots := unit{glyph: b.face.classify(ellipsis)}
	dots.em = r.emWidth(b.face, dots.glyph, b.bold)
	us := last.units
	em := last.em
	for len(us) > 0 && (em+dots.em > maxEm || us[len(us)-1].draw == ' ') {
		em -= us[len(us)-1].em
		us = us[:len(us)-1]
	}
	lines = append(lines[:len(lines)-1:len(lines)-1], line{append(us[:len(us):len(us)], dots), em + dots.em})
	return layout{lines, size, true}
}

// plain returns the text of the lines, for tests and logs.
func (l layout) plain() string {
	var sb strings.Builder
	for i, ln := range l.lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		for _, u := range ln.units {
			sb.WriteRune(u.draw)
		}
	}
	return sb.String()
}
