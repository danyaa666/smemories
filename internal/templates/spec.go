// Package templates defines the declarative page-template spec (JSON, units in millimetres on a
// reference page, A5 by default), validates it and embeds the built-in templates. It knows nothing about PDF.
package templates

import "slices"

// Dims are the known page sizes as width and height in millimetres. The renderer scales a template's
// coordinates from its reference page to the requested size.
var Dims = map[string][2]float64{"A5": {148, 210}, "A4": {210, 297}, "Letter": {215.9, 279.4}}

// DefaultReference is the reference page of a template that does not say.
const DefaultReference = "A5"

// ratioTol is how far two page aspect ratios may differ (relative) and still count as the same shape:
// A5 and A4 differ by 0.3 %, A5 and Letter by 9.6 %.
const ratioTol = 0.01

// Page kinds, element types, image shapes and the known font families.
const (
	KindCover   = "cover"
	KindProfile = "profile"
	KindNotes   = "notes"
	KindBack    = "back"

	TypeText  = "text"
	TypeImage = "image"
	TypeRect  = "rect"
)

// KnownFonts are the font families the renderer embeds.
var KnownFonts = map[string]bool{"BeVietnamPro": true}

// Template is one parsed, validated template.
type Template struct {
	ID        string            `json:"id"`
	Name      map[string]string `json:"name"`                // by language: en, vi
	Unit      string            `json:"unit"`                // must be "mm"
	Reference string            `json:"reference,omitempty"` // A5 (default) or Letter: the page the mm coordinates refer to
	PageSizes []string          `json:"page_sizes"`
	Theme     Theme             `json:"theme"`
	Pages     []Page            `json:"pages"`
}

// Theme holds the font family and named colours (#rrggbb). ink, accent and paper are required.
type Theme struct {
	Font   string            `json:"font"`
	Colors map[string]string `json:"colors"`
}

// Page is one page kind. Notes pages repeat as often as needed and carry a Flow.
type Page struct {
	Kind     string    `json:"kind"`
	Elements []Element `json:"elements"`
	Flow     *Flow     `json:"flow,omitempty"`
}

// Flow is the repeating block of a notes page: items of height ItemH stacked in the area
// (X, Y, W, H) with Gap between them. Item elements are positioned relative to the item's top-left corner.
type Flow struct {
	X        float64   `json:"x"`
	Y        float64   `json:"y"`
	W        float64   `json:"w"`
	H        float64   `json:"h"`
	Gap      float64   `json:"gap"`
	ItemH    float64   `json:"item_h"`
	Elements []Element `json:"elements"`
}

// RefDims is the width and height in mm of the template's reference page (A5 when unset or unknown;
// the validator rejects unknown values).
func (t *Template) RefDims() (w, h float64) {
	d, ok := Dims[t.Reference]
	if !ok {
		d = Dims[DefaultReference]
	}
	return d[0], d[1]
}

// Supports reports whether the template declares the page size.
func (t *Template) Supports(size string) bool { return slices.Contains(t.PageSizes, size) }

// PerPage is how many items fit in the flow area (the validator guarantees at least one).
func (f *Flow) PerPage() int {
	n := int((f.H + f.Gap) / (f.ItemH + f.Gap))
	return max(n, 1)
}

// Element is a text, image or rect box. Fields not used by a type must be left out.
type Element struct {
	Type string  `json:"type"`
	Slot string  `json:"slot,omitempty"` // text and image: where the data comes from
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`

	// text
	Size       float64           `json:"size,omitempty"`
	MinSize    float64           `json:"min_size,omitempty"` // shrink floor; default = Size (no shrinking)
	Align      string            `json:"align,omitempty"`    // left (default), center, right
	Bold       bool              `json:"bold,omitempty"`
	Color      string            `json:"color,omitempty"`       // theme colour name, default ink
	LineHeight float64           `json:"line_height,omitempty"` // multiple of the font size, default 1.3
	Label      map[string]string `json:"label,omitempty"`       // prefix by language, drawn only when the slot has text

	// image
	Fit    string  `json:"fit,omitempty"`    // cover (default, the only value)
	Shape  string  `json:"shape,omitempty"`  // rect (default), rounded, circle (needs w == h)
	Radius float64 `json:"radius,omitempty"` // rounded images and rects

	// rect
	Fill string `json:"fill,omitempty"` // theme colour name
}
