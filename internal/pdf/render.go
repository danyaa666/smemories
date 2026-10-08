// Package pdf renders a Book into a PDF using a validated template (internal/templates).
package pdf

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"codeberg.org/go-pdf/fpdf"

	"github.com/danyaa666/smemories/internal/pdf/fonts"
	"github.com/danyaa666/smemories/internal/templates"
)

// Book is the plain data a PDF is made from. Photo fields are media ids resolved through ImageSource.
type Book struct {
	Title, School, Class, Year, Motto string
	CoverPhoto                        string
	Profile                           Profile
	Notes                             []Note
}

// Profile is the owner's own page.
type Profile struct {
	FullName, Nickname, Quote, Hobbies, Plans string
	PhotoID                                   string
}

// Note is one friend's message. ID is only used to say where a warning came from.
type Note struct {
	ID                   string
	Author, Relationship string
	Message              string
	PhotoIDs             []string
}

// Options tune a render. The zero value means A5, English labels and the current time.
type Options struct {
	PageSize string    // A5 (default), A4 or Letter; must be one the template declares
	Lang     string    // en (default) or vi: picks element labels
	Now      time.Time // PDF creation date; fix it for byte-identical output
}

type renderer struct {
	tmpl   *templates.Template
	book   Book
	src    ImageSource
	lang   string
	pdf    *fpdf.Fpdf
	faces  map[string]*glyphs // by registry family name: the families this template uses
	bgs    map[string]string  // background assets registered with fpdf: asset path -> image type
	widths map[widthKey]float64
	imgs   map[string]*imgInfo
	lowRes map[string]bool
	report Report
	// sx, sy scale reference-page millimetres to the page; ss scales font sizes and radii.
	sx, sy, ss float64
}

// slotData is what the slots of one page (or one note) resolve to.
type slotData struct {
	text   map[string]string
	img    map[string]string
	noteID string
}

// Render draws book with tmpl onto w. Problems with the content (missing glyphs, long text, bad or small
// photos) are reported in Report and never fail the render; an error means the PDF is not complete.
func Render(ctx context.Context, tmpl *templates.Template, book Book, images ImageSource, w io.Writer, opts Options) (rep Report, err error) {
	if opts.PageSize == "" {
		opts.PageSize = "A5"
	}
	dims, ok := templates.PageDims(opts.PageSize)
	if !ok || !tmpl.Supports(opts.PageSize) {
		return Report{}, fmt.Errorf("template %q does not support page size %q", tmpl.ID, opts.PageSize)
	}
	if opts.Lang == "" {
		opts.Lang = "en"
	}
	rw, rh := tmpl.RefDims()
	r := &renderer{tmpl: tmpl, book: book, src: images, lang: opts.Lang, faces: map[string]*glyphs{}, bgs: map[string]string{},
		widths: map[widthKey]float64{}, imgs: map[string]*imgInfo{}, lowRes: map[string]bool{},
		sx: dims[0] / rw, sy: dims[1] / rh}
	r.ss = r.sx
	if err := r.loadFaces(); err != nil {
		return Report{}, err
	}

	// fpdf can panic on input it does not expect; the caller must get an error, not a crash.
	defer func() {
		if p := recover(); p != nil {
			rep, err = r.report, fmt.Errorf("render pdf: internal error: %v", p)
		}
	}()
	r.pdf = r.newDoc(dims, opts.Now)
	if err := r.build(ctx); err != nil {
		return r.report, err
	}
	r.report.Pages = r.pdf.PageNo()
	if err := r.pdf.Output(w); err != nil {
		return r.report, fmt.Errorf("render pdf: %w", err)
	}
	return r.report, nil
}

func (r *renderer) newDoc(dims [2]float64, now time.Time) *fpdf.Fpdf {
	// fpdf's built-in "A5" is 148.5 x 210 mm; use exact sizes.
	pdf := fpdf.NewCustom(&fpdf.InitType{OrientationStr: "P", UnitStr: "mm", Size: fpdf.SizeType{Wd: dims[0], Ht: dims[1]}})
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetMargins(0, 0, 0)
	for _, name := range sortedFaces(r.faces) {
		g := r.faces[name]
		if g.fam.Regular != nil {
			pdf.AddUTF8FontFromBytes(g.pdfFam, "", g.fam.Regular)
		}
		if g.fam.Bold != nil {
			pdf.AddUTF8FontFromBytes(g.pdfFam, "B", g.fam.Bold)
		}
	}
	pdf.AddUTF8FontFromBytes(famEmoji, "", fonts.Emoji)
	pdf.SetCatalogSort(true) // stable resource order, so identical input gives identical bytes
	if now.IsZero() {
		now = time.Now()
	}
	pdf.SetCreationDate(now)
	pdf.SetModificationDate(now)
	pdf.SetCreator("SMemories", true)
	title, _ := prepareText(r.book.Title)
	pdf.SetTitle(strings.ReplaceAll(title, "\n", " "), true)
	return pdf
}

// loadFaces prepares the font families the template uses: the body family and, when an element asks for
// it, the display family.
func (r *renderer) loadFaces() error {
	names := []string{r.tmpl.Family(templates.RoleBody)}
	if r.usesRole(templates.RoleDisplay) {
		names = append(names, r.tmpl.Family(templates.RoleDisplay))
	}
	for _, n := range names {
		if r.faces[n] != nil {
			continue
		}
		g, err := newGlyphs(n)
		if err != nil {
			return fmt.Errorf("render pdf: %w", err)
		}
		r.faces[n] = g
	}
	return nil
}

func (r *renderer) usesRole(role string) bool {
	uses := func(els []templates.Element) bool {
		return slices.ContainsFunc(els, func(e templates.Element) bool { return e.Type == templates.TypeText && e.Font == role })
	}
	for _, p := range r.tmpl.Pages {
		if uses(p.Elements) || (p.Flow != nil && uses(p.Flow.Elements)) {
			return true
		}
	}
	return false
}

func sortedFaces(m map[string]*glyphs) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func (r *renderer) build(ctx context.Context) error {
	for i := range r.tmpl.Pages {
		p := &r.tmpl.Pages[i]
		switch p.Kind {
		case templates.KindNotes:
			if err := r.notesPages(ctx, p); err != nil {
				return err
			}
		default:
			if err := r.page(ctx, p.Elements, r.pageData(p.Kind)); err != nil {
				return err
			}
		}
	}
	if err := r.pdf.Error(); err != nil {
		return fmt.Errorf("render pdf: %w", err)
	}
	return nil
}

// newPage starts a page: the paper fill, then the background image if els has one, below everything else.
func (r *renderer) newPage(els []templates.Element) error {
	r.pdf.AddPage()
	pw, ph := r.pdf.GetPageSize()
	r.setFill("paper")
	r.pdf.Rect(0, 0, pw, ph, "F")
	for _, e := range els {
		if e.Type == templates.TypeBackground {
			return r.drawBackground(e.Asset, pw, ph)
		}
	}
	return nil
}

func (r *renderer) page(ctx context.Context, els []templates.Element, d *slotData) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.newPage(els); err != nil {
		return err
	}
	return r.elements(ctx, els, 0, 0, d)
}

func (r *renderer) notesPages(ctx context.Context, p *templates.Page) error {
	f, notes := p.Flow, r.book.Notes
	per := f.PerPage()
	photoSlots := 0
	for _, e := range f.Elements {
		if e.Type == templates.TypeImage {
			photoSlots++
		}
	}
	for start := 0; start == 0 || start < len(notes); start += per { // an empty book still gets one page
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.newPage(p.Elements); err != nil {
			return err
		}
		if err := r.elements(ctx, p.Elements, 0, 0, &slotData{}); err != nil {
			return err
		}
		for i := 0; i < per && start+i < len(notes); i++ {
			n := notes[start+i]
			if len(n.PhotoIDs) > photoSlots {
				r.warn(Warning{Code: WarnExtraPhotos, NoteID: n.ID})
			}
			d := &slotData{noteID: n.ID,
				text: map[string]string{"note_author": n.Author, "note_relationship": n.Relationship, "note_message": n.Message},
				img:  map[string]string{}}
			for j, id := range n.PhotoIDs {
				d.img[fmt.Sprintf("note_photo_%d", j+1)] = id
			}
			if err := r.elements(ctx, f.Elements, f.X, f.Y+float64(i)*(f.ItemH+f.Gap), d); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *renderer) pageData(kind string) *slotData {
	b, p := r.book, r.book.Profile
	d := &slotData{}
	switch kind {
	case templates.KindCover:
		d.text = map[string]string{"title": b.Title, "school": b.School, "class": b.Class, "year": b.Year}
		d.img = map[string]string{"cover_photo": b.CoverPhoto}
	case templates.KindProfile:
		d.text = map[string]string{"full_name": p.FullName, "nickname": p.Nickname, "quote": p.Quote, "hobbies": p.Hobbies, "plans": p.Plans}
		d.img = map[string]string{"photo": p.PhotoID}
	case templates.KindBack:
		d.text = map[string]string{"motto": b.Motto, "title": b.Title, "school": b.School}
	}
	return d
}

// elements draws els with their coordinates relative to (ox, oy) in reference millimetres.
func (r *renderer) elements(ctx context.Context, els []templates.Element, ox, oy float64, d *slotData) error {
	for _, e := range els {
		switch e.Type {
		case templates.TypeRect:
			r.drawRect(e, ox, oy)
		case templates.TypeText:
			r.drawText(e, ox, oy, d)
		case templates.TypeImage:
			if err := r.drawImage(ctx, e, ox, oy, d.img[e.Slot], d); err != nil {
				return err
			}
		}
	}
	return nil
}

// rotated turns the drawing around the centre of the box by the element's rotate (degrees, clockwise) and
// returns the function that ends it. Defer that function so a panic cannot leave a transform open.
func (r *renderer) rotated(e templates.Element, x, y, w, h float64) func() {
	if e.Rotate == 0 {
		return func() {}
	}
	r.pdf.TransformBegin()
	r.pdf.TransformRotate(-e.Rotate, x+w/2, y+h/2) // fpdf rotates counter-clockwise
	return r.pdf.TransformEnd
}

func (r *renderer) drawRect(e templates.Element, ox, oy float64) {
	r.setFill(e.Fill)
	x, y, w, h := (ox+e.X)*r.sx, (oy+e.Y)*r.sy, e.W*r.sx, e.H*r.sy
	defer r.rotated(e, x, y, w, h)()
	if e.Radius > 0 {
		r.pdf.RoundedRect(x, y, w, h, e.Radius*r.ss, "1234", "F")
		return
	}
	r.pdf.Rect(x, y, w, h, "F")
}

func (r *renderer) drawText(e templates.Element, ox, oy float64, d *slotData) {
	var val, label string
	if e.Static != nil { // fixed text: never "missing data"; the other warnings still apply
		val = cmp.Or(e.Static[r.lang], e.Static["en"])
	} else {
		val, label = d.text[e.Slot], e.Label[r.lang]
	}
	if val = strings.TrimSpace(val); val == "" {
		return
	}
	face := r.faces[r.tmpl.Family(e.Font)]
	txt, cut := prepareText(label + val)
	us, missing := r.toUnits(face, txt, e.Bold)
	for _, m := range missing {
		r.warn(Warning{Code: WarnMissingGlyph, Slot: e.Slot, NoteID: d.noteID, Rune: string(m)})
	}
	lh := e.LineHeight
	if lh == 0 {
		lh = defaultLineH
	}
	minSize := e.MinSize
	if minSize == 0 {
		minSize = e.Size
	}
	box := textBox{face: face, w: e.W * r.sx, h: e.H * r.sy, size: e.Size * r.ss, minSize: minSize * r.ss, lineH: lh, bold: e.Bold}
	lay := r.fit(us, box, cut)
	if lay.truncated {
		r.warn(Warning{Code: WarnTextTruncated, Slot: e.Slot, NoteID: d.noteID})
	}
	x, y := (ox+e.X)*r.sx, (oy+e.Y)*r.sy
	defer r.rotated(e, x, y, box.w, box.h)() // wrapping and fitting above happened in the unrotated box
	r.drawLines(lay, box, x, y, e)
}

// drawLines writes the fitted lines, one Text call per run of the same font.
func (r *renderer) drawLines(l layout, b textBox, x, y float64, e templates.Element) {
	r.setText(e.Color)
	lm := b.lineMM(l.size)
	em := l.size * ptMM
	for i, ln := range l.lines {
		lx := x
		switch e.Align {
		case "center":
			lx += (b.w - ln.em*em) / 2
		case "right":
			lx += b.w - ln.em*em
		}
		base := y + float64(i)*lm + lm/2 + 0.35*em // baseline: the em box centred in the line
		for j := 0; j < len(ln.units); {
			k, runEm := j, 0.0
			var sb strings.Builder
			for ; k < len(ln.units) && ln.units[k].font == ln.units[j].font; k++ {
				sb.WriteRune(ln.units[k].draw)
				runEm += ln.units[k].em
			}
			fam, style := fontFor(b.face, ln.units[j].font, b.bold)
			r.pdf.SetFont(fam, style, l.size)
			r.pdf.Text(lx, base, sb.String())
			lx += runEm * em
			j = k
		}
	}
}

func (r *renderer) warn(w Warning) {
	w.Page = r.pdf.PageNo()
	r.report.Warnings = append(r.report.Warnings, w)
}

func (r *renderer) rgb(name string) (int, int, int) {
	if name == "" {
		name = "ink"
	}
	var cr, cg, cb int
	_, _ = fmt.Sscanf(r.tmpl.Theme.Colors[name], "#%02x%02x%02x", &cr, &cg, &cb) // validated by templates
	return cr, cg, cb
}

func (r *renderer) setFill(name string) { r.pdf.SetFillColor(r.rgb(name)) }
func (r *renderer) setText(name string) { r.pdf.SetTextColor(r.rgb(name)) }
