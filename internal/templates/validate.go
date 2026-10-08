package templates

import (
	"cmp"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // registers the JPEG decoder for DecodeConfig
	_ "image/png"  // registers the PNG decoder for DecodeConfig
	"io/fs"
	"math"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/danyaa666/smemories/internal/pdf/fonts"
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
	maxRotate     = 45.0      // degrees
	maxStaticLen  = 500       // characters per language of a static text

	// Limits of a background asset: file size, total per template, and the effective resolution on the
	// reference page. The aspect ratio must match the reference page within ratioTol.
	maxAssetBytes  = 3 << 19 // 1.5 MiB
	maxAssetsTotal = 8 << 20
	minAssetDPI    = 150
	maxAssetDPI    = 400
	dpiTol         = 0.5 // DPI: rounding of the pixel size of a design exported at exactly 150 or 400
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
	assetFileRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	idRe        = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	colorRe     = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	langs       = []string{"en", "vi"}
)

// Validate checks t and returns every problem found, each naming the template, page and element.
func Validate(t *Template) error {
	v := &validator{t: t, assetSize: map[string]int64{}}
	v.template()
	return errors.Join(v.errs...)
}

type validator struct {
	t         *Template
	errs      []error
	assetSize map[string]int64 // distinct background files seen, for the per-template total
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
	// With an invalid reference the page sizes are not compared to it: that would only add a second,
	// misleading error next to the right one.
	refOK := t.Reference == "" || t.Reference == "A5" || t.Reference == "Letter"
	if !refOK {
		v.errf("", "invalid reference %q: only \"A5\" or \"Letter\"", t.Reference)
	}
	if len(t.PageSizes) == 0 {
		v.errf("", "page_sizes is empty")
	}
	rw, rh := t.RefDims()
	for _, s := range t.PageSizes {
		d, ok := dims[s]
		switch {
		case !ok:
			v.errf("", "unknown page size %q", s)
		case refOK && math.Abs((d[0]/d[1])/(rw/rh)-1) > ratioTol:
			v.errf("", "page size %q has a different aspect ratio than the %s reference page", s, cmp.Or(t.Reference, DefaultReference))
		}
	}
	v.fonts()
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
	v.assetTotal()
}

// assetTotal checks the size of all distinct background files together.
func (v *validator) assetTotal() {
	var total int64
	for _, n := range v.assetSize {
		total += n
	}
	if total > maxAssetsTotal {
		v.errf("", "background assets total %d bytes, the limit is %d", total, maxAssetsTotal)
	}
}

// fonts checks theme.font (the body family, kept for older files) and theme.fonts (role -> family).
func (v *validator) fonts() {
	th := v.t.Theme
	known := func(where, name string) {
		if _, ok := fonts.Lookup(name); !ok {
			v.errf("", "%s: missing font %q (known: %s)", where, name, strings.Join(fonts.Names(), ", "))
		}
	}
	for _, role := range sortedKeys(th.Fonts) {
		if role != RoleBody && role != RoleDisplay {
			v.errf("", "unknown font role %q (use %s or %s)", role, RoleBody, RoleDisplay)
			continue
		}
		known("fonts."+role, th.Fonts[role])
	}
	if th.Font != "" {
		known("font", th.Font)
	}
	switch body := th.Fonts[RoleBody]; {
	case th.Font == "" && body == "":
		v.errf("", "missing font \"\": set theme.font or theme.fonts.body (known: %s)", strings.Join(fonts.Names(), ", "))
	case th.Font != "" && body != "" && th.Font != body:
		v.errf("", "theme.font %q and theme.fonts.body %q differ", th.Font, body)
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
	v.elements(where, p.Elements, slots, rw, rh, true)
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
	v.elements(fw, f.Elements, Slots["note"], f.W, f.ItemH, false)
}

// box checks an area lies inside a w x h region and has a positive size.
func (v *validator) box(where string, x, y, w, h, maxW, maxH float64) {
	if x < 0 || y < 0 || w <= 0 || h <= 0 || x+w > maxW+eps || y+h > maxH+eps {
		v.errf(where, "box x=%g y=%g w=%g h=%g is outside the %g x %g mm area (units are mm; w and h must be > 0)", x, y, w, h, maxW, maxH)
	}
}

// elements checks one list of elements; onPage is false inside a notes flow, where a background is not allowed.
func (v *validator) elements(where string, els []Element, slots map[string]string, maxW, maxH float64, onPage bool) {
	if len(els) > maxElements {
		v.errf(where, "%d elements exceeds %d", len(els), maxElements)
		return
	}
	backgrounds := 0
	for i := range els {
		e := &els[i]
		ew := fmt.Sprintf("%s, element %d (%s %q)", where, i+1, e.Type, cmp.Or(e.Slot, e.Asset))
		if e.Type == TypeBackground {
			backgrounds++
			switch {
			case !onPage:
				v.errf(ew, "a background is only allowed on a page, not in a notes flow")
			case backgrounds > 1:
				v.errf(ew, "more than one background on this page")
			default:
				v.background(ew, e)
			}
			continue
		}
		v.box(ew, e.X, e.Y, e.W, e.H, maxW, maxH)
		if math.Abs(e.Rotate) > maxRotate {
			v.errf(ew, "rotate %g must be in -%g..%g degrees", e.Rotate, maxRotate, maxRotate)
		}
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

// background checks a page background: only type and asset may be set, and the file must be a sound image.
func (v *validator) background(where string, e *Element) {
	z := *e
	z.Type, z.Asset = "", ""
	if !reflect.ValueOf(z).IsZero() {
		v.errf(where, "a background takes only type and asset (it always fills the page)")
	}
	file, ok := strings.CutPrefix(e.Asset, v.t.ID+"/")
	ext := strings.ToLower(path.Ext(file))
	if !ok || !assetFileRe.MatchString(file) || strings.Contains(file, "..") || !slices.Contains([]string{".png", ".jpg", ".jpeg"}, ext) {
		v.errf(where, "asset %q must be %q followed by a plain file name ending in .png, .jpg or .jpeg (no folders, no \"..\")", e.Asset, v.t.ID+"/")
		return
	}
	if v.t.assets == nil {
		v.errf(where, "asset %q cannot be checked: the template has no asset folder", e.Asset)
		return
	}
	st, err := fs.Stat(v.t.assets, e.Asset)
	if err != nil || !st.Mode().IsRegular() {
		v.errf(where, "asset %q does not exist in the template's embed folder", e.Asset)
		return
	}
	v.assetSize[e.Asset] = st.Size()
	if st.Size() > maxAssetBytes {
		v.errf(where, "asset %q is %d bytes, the limit is %d", e.Asset, st.Size(), maxAssetBytes)
		return
	}
	f, err := v.t.assets.Open(e.Asset)
	if err != nil {
		v.errf(where, "asset %q cannot be opened: %v", e.Asset, err)
		return
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f) // by content, not by extension
	if err != nil || (format != "png" && format != "jpeg") {
		v.errf(where, "asset %q is not a PNG or JPEG image", e.Asset)
		return
	}
	rw, rh := v.t.RefDims()
	if cfg.Width <= 0 || cfg.Height <= 0 || math.Abs((float64(cfg.Width)/float64(cfg.Height))/(rw/rh)-1) > ratioTol {
		v.errf(where, "asset %q is %d x %d px: its aspect ratio must match the %g x %g mm reference page within 1 %%", e.Asset, cfg.Width, cfg.Height, rw, rh)
		return
	}
	if dpi := float64(cfg.Width) / (rw / 25.4); dpi < minAssetDPI-dpiTol || dpi > maxAssetDPI+dpiTol {
		v.errf(where, "asset %q is %d x %d px: %.0f DPI on the reference page, it must be %d..%d", e.Asset, cfg.Width, cfg.Height, dpi, minAssetDPI, maxAssetDPI)
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
	switch {
	case e.Static != nil && e.Slot != "":
		v.errf(where, "a text element has either a slot or a text object, not both")
	case e.Static != nil:
		v.static(where, e)
	case e.Slot == "":
		v.errf(where, "a text element needs a slot or a text object")
	default:
		v.slot(where, e, slots)
	}
	if !slices.Contains([]string{"", RoleBody, RoleDisplay}, e.Font) {
		v.errf(where, "unknown font role %q (use %s or %s)", e.Font, RoleBody, RoleDisplay)
	}
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

// static checks fixed text: both languages, none other, each non-blank, short and free of control characters.
func (v *validator) static(where string, e *Element) {
	for l := range e.Static {
		if !slices.Contains(langs, l) {
			v.errf(where, "text has unknown language %q (use en and vi)", l)
		}
	}
	for _, l := range langs {
		s := e.Static[l]
		switch {
		case strings.TrimSpace(s) == "":
			v.errf(where, "text is missing the %q language", l)
		case len([]rune(s)) > maxStaticLen:
			v.errf(where, "text %q is longer than %d characters", l, maxStaticLen)
		case strings.IndexFunc(s, unicode.IsControl) >= 0:
			v.errf(where, "text %q contains a control character", l)
		}
	}
	if len(e.Label) > 0 {
		v.errf(where, "label only applies to slot text")
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
	case "ellipse":
		if e.Radius != 0 {
			v.errf(where, "radius is not allowed for an ellipse")
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
