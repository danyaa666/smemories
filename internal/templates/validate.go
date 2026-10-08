package templates

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
)

// Bounds that keep a (future, user-supplied) template from exhausting memory.
const (
	maxPages      = 16
	maxElements   = 64
	minFontSize   = 4.0
	maxFontSize   = 96.0
	maxLineHeight = 3.0
	maxPerPage    = 20
	eps           = 1e-6
	maxLabelLen   = 40
	ptMM          = 25.4 / 72 // one point in millimetres
)

// Slots is the closed set of slot names, by page kind (or "note" for the items of a notes flow), and the
// element type each one feeds. Keep docs/templates.md in step.
var Slots = map[string]map[string]string{
	KindCover:   {"title": TypeText, "school": TypeText, "class": TypeText, "year": TypeText, "cover_photo": TypeImage},
	KindProfile: {"photo": TypeImage, "full_name": TypeText, "nickname": TypeText, "quote": TypeText, "hobbies": TypeText, "plans": TypeText},
	KindNotes:   {},
	KindBack:    {"motto": TypeText, "title": TypeText, "school": TypeText},
	"note": {
		"note_author": TypeText, "note_relationship": TypeText, "note_message": TypeText,
		"note_photo_1": TypeImage, "note_photo_2": TypeImage, "note_photo_3": TypeImage,
	},
}

var (
	idRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	langs   = []string{"en", "vi"}
)

// Validate checks t and returns every problem found, each naming the template, page and element.
func Validate(t *Template) error {
	v := &validator{t: t}
	v.template()
	return errors.Join(v.errs...)
}

type validator struct {
	t    *Template
	errs []error
}

func (v *validator) errf(where string, format string, args ...any) {
	v.errs = append(v.errs, fmt.Errorf("template %q%s: %s", v.t.ID, where, fmt.Sprintf(format, args...)))
}

func (v *validator) template() {
	t := v.t
	if !idRe.MatchString(t.ID) {
		v.errf("", "id must match %s", idRe)
	}
	for _, l := range langs {
		if strings.TrimSpace(t.Name[l]) == "" {
			v.errf("", "missing name for language %q", l)
		}
	}
	if t.Unit != "mm" {
		v.errf("", "invalid unit %q: only \"mm\" is supported", t.Unit)
	}
	if t.Reference != "" && t.Reference != "A5" && t.Reference != "Letter" {
		v.errf("", "invalid reference %q: only \"A5\" or \"Letter\"", t.Reference)
	}
	if len(t.PageSizes) == 0 {
		v.errf("", "page_sizes is empty")
	}
	rw, rh := t.RefDims()
	for _, s := range t.PageSizes {
		d, ok := Dims[s]
		switch {
		case !ok:
			v.errf("", "unknown page size %q", s)
		case math.Abs((d[0]/d[1])/(rw/rh)-1) > ratioTol:
			v.errf("", "page size %q has a different aspect ratio than the %s reference page", s, cmp.Or(t.Reference, DefaultReference))
		}
	}
	if !KnownFonts[t.Theme.Font] {
		v.errf("", "missing font %q (known: %s)", t.Theme.Font, strings.Join(sortedKeys(KnownFonts), ", "))
	}
	for name, c := range t.Theme.Colors {
		if !colorRe.MatchString(c) {
			v.errf("", "colour %q: %q is not #rrggbb", name, c)
		}
	}
	for _, name := range []string{"ink", "accent", "paper"} {
		if _, ok := t.Theme.Colors[name]; !ok {
			v.errf("", "theme colour %q is required", name)
		}
	}
	if len(t.Pages) == 0 || len(t.Pages) > maxPages {
		v.errf("", "needs 1..%d pages, has %d", maxPages, len(t.Pages))
	}
	seen := map[string]bool{}
	for i := range t.Pages {
		v.page(i, &t.Pages[i], seen)
	}
	for _, k := range []string{KindCover, KindProfile, KindNotes, KindBack} {
		if !seen[k] {
			v.errf("", "no %q page (every template needs one page of each kind)", k)
		}
	}
}

func (v *validator) page(i int, p *Page, seen map[string]bool) {
	where := fmt.Sprintf(", page %d (%s)", i+1, p.Kind)
	slots, ok := Slots[p.Kind]
	if !ok || p.Kind == "note" {
		v.errf(where, "unknown page kind")
		return
	}
	if seen[p.Kind] {
		v.errf(where, "more than one %q page", p.Kind)
	}
	seen[p.Kind] = true
	rw, rh := v.t.RefDims()
	v.elements(where, p.Elements, slots, rw, rh)
	if p.Kind != KindNotes {
		if p.Flow != nil {
			v.errf(where, "flow is only allowed on notes pages")
		}
		return
	}
	f := p.Flow
	if f == nil {
		v.errf(where, "notes page needs a flow")
		return
	}
	fw := where + ", flow"
	v.box(fw, f.X, f.Y, f.W, f.H, rw, rh)
	if f.Gap < 0 || f.ItemH <= 0 || f.ItemH > f.H+eps {
		v.errf(fw, "item_h must be in (0, flow h] and gap >= 0")
		return
	}
	if n := f.PerPage(); n > maxPerPage {
		v.errf(fw, "%d items per page exceeds %d", n, maxPerPage)
	}
	v.elements(fw, f.Elements, Slots["note"], f.W, f.ItemH)
}

// box checks an area lies inside a w x h region and has a positive size.
func (v *validator) box(where string, x, y, w, h, maxW, maxH float64) {
	if x < 0 || y < 0 || w <= 0 || h <= 0 || x+w > maxW+eps || y+h > maxH+eps {
		v.errf(where, "box x=%g y=%g w=%g h=%g is outside the %g x %g mm area (units are mm; w and h must be > 0)", x, y, w, h, maxW, maxH)
	}
}

func (v *validator) elements(where string, els []Element, slots map[string]string, maxW, maxH float64) {
	if len(els) > maxElements {
		v.errf(where, "%d elements exceeds %d", len(els), maxElements)
		return
	}
	for i := range els {
		e := &els[i]
		ew := fmt.Sprintf("%s, element %d (%s %q)", where, i+1, e.Type, e.Slot)
		v.box(ew, e.X, e.Y, e.W, e.H, maxW, maxH)
		switch e.Type {
		case TypeText:
			v.text(ew, e, slots)
		case TypeImage:
			v.image(ew, e, slots)
		case TypeRect:
			if e.Slot != "" {
				v.errf(ew, "rect elements have no slot")
			}
			v.colorName(ew, "fill", e.Fill, true)
			if e.Radius < 0 || e.Radius > min(e.W, e.H)/2 {
				v.errf(ew, "radius %g must be in [0, min(w,h)/2]", e.Radius)
			}
		default:
			v.errf(ew, "unknown element type %q", e.Type)
		}
	}
}

func (v *validator) slot(where string, e *Element, slots map[string]string) {
	want, ok := slots[e.Slot]
	switch {
	case !ok:
		v.errf(where, "unknown slot %q here (allowed: %s)", e.Slot, strings.Join(sortedKeys(slots), ", "))
	case want != e.Type:
		v.errf(where, "slot %q is a %s slot", e.Slot, want)
	}
}

func (v *validator) text(where string, e *Element, slots map[string]string) {
	v.slot(where, e, slots)
	if e.Size < minFontSize || e.Size > maxFontSize {
		v.errf(where, "size %g must be in %g..%g pt", e.Size, minFontSize, maxFontSize)
	}
	if e.MinSize != 0 && (e.MinSize < minFontSize || e.MinSize > e.Size) {
		v.errf(where, "min_size %g must be in %g..size", e.MinSize, minFontSize)
	}
	lh := e.LineHeight
	if lh == 0 {
		lh = 1.3
	}
	if e.H < lh*ptMM*cmp.Or(e.MinSize, e.Size) {
		v.errf(where, "box is %g mm high, too short for one line at the smallest size", e.H)
	}
	if !slices.Contains([]string{"", "left", "center", "right"}, e.Align) {
		v.errf(where, "unknown align %q", e.Align)
	}
	if e.LineHeight != 0 && (e.LineHeight < 1 || e.LineHeight > maxLineHeight) {
		v.errf(where, "line_height %g must be in 1..%g", e.LineHeight, maxLineHeight)
	}
	v.colorName(where, "color", e.Color, false)
	for l, s := range e.Label {
		if !slices.Contains(langs, l) || len([]rune(s)) > maxLabelLen {
			v.errf(where, "label for %q must use a known language and stay under %d characters", l, maxLabelLen)
		}
	}
}

func (v *validator) image(where string, e *Element, slots map[string]string) {
	v.slot(where, e, slots)
	if e.Fit != "" && e.Fit != "cover" {
		v.errf(where, "unknown fit %q (only cover)", e.Fit)
	}
	switch e.Shape {
	case "", "rect":
	case "rounded":
		if e.Radius <= 0 || e.Radius > min(e.W, e.H)/2 {
			v.errf(where, "radius %g must be in (0, min(w,h)/2]", e.Radius)
		}
	case "circle":
		if e.W != e.H {
			v.errf(where, "a circle needs w == h")
		}
	default:
		v.errf(where, "unknown shape %q", e.Shape)
	}
}

func (v *validator) colorName(where, field, name string, required bool) {
	if name == "" && !required {
		return
	}
	if _, ok := v.t.Theme.Colors[name]; !ok {
		v.errf(where, "%s %q is not a theme colour", field, name)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
