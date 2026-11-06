package booklayout

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/danyaa666/smemories/internal/templates"
)

// The quality gates of the shipped Navy Classic templates (E09 T-178): four or more looks per page kind, unique ids and labels, no two alike, no prose, every box inside the safe
// margin and inside the inner rule of the gold frame on every paper, and the book's own text readable on the page colour in every theme. A template that fails one fails the build.
// checkNavyPage is the gate for one page, and TestNavyGatesCatchBadPages feeds it deliberately bad pages so the gate is known to bite.
//
// The names carry the prefix nc because the Memory Book gates (shipped_memory_book_test.go, T-177) live in the same package; merging the two helper sets is a follow-up.

const (
	ncMinPerKind = 4
	ncMaxLabel   = 24 // runes: the gallery card width, both languages (T-175)
	ncMinRatio   = 4.5
	ncDesignW    = 816.0 // web/src/book/sizes.ts DESIGN_W: every template is drawn in a box 816 px wide, as high as the paper's ratio says
	ncRuleGap    = 4.0   // design px of air between a decoration and the frame's inner rule
)

// the colour tokens of navy.css per theme (TestNavyThemeTableMatchesCSS keeps this table honest).
var ncThemes = map[string]map[string]string{
	"navy":     {"ink": "#1b2a4a", "paper": "#f7f3ea", "gold": "#c9a86a", "bronze": "#7a5c2b"},
	"burgundy": {"ink": "#4a1a2b", "paper": "#faf3ee", "gold": "#d9b57c", "bronze": "#8a4a2e"},
	"forest":   {"ink": "#143a2f", "paper": "#f4f6ee", "gold": "#cfb06e", "bronze": "#6b5a1f"},
}

var (
	// the page colour a kind has when the template sets none, and the text colours the book itself draws on it: light and gold on the dark pages, dark and bronze on the cream ones.
	ncOwnBg   = map[string]string{"cover": "ink", "profile": "paper", "notes": "paper", "back": "ink"}
	ncOwnText = map[string][]string{"cover": {"paper", "gold"}, "profile": {"ink", "bronze"}, "notes": {"ink", "bronze"}, "back": {"paper", "gold"}}
	// the inner rule of the gold frame, in design px from the page edge (navy.tsx: Frame outer/inner); the profile has none
	ncFrameInset = map[string]float64{"cover": 44, "profile": 0, "notes": 34, "back": 44}
	// the template elements that hold text: a decoration must not sit on one that the template gives a box of its own
	ncTextElements = []string{"cover.school", "cover.year", "cover.title", "cover.motto", "cover.class", "profile.eyebrow", "profile.heading", "notes.eyebrow", "notes.heading", "back.thanks", "back.footer"}
	ncPapersMM     = map[string][2]float64{"A5": {148, 210}, "A4": {210, 297}, "Letter": {215.9, 279.4}}
	ncNeutralText  = regexp.MustCompile(`^[\d\p{P}\p{S}\s]+$`) // digits, punctuation, symbols: nothing that reads as a sentence
)

type ncItem struct {
	ID    string  `json:"id"`
	T     string  `json:"t"`
	X     float64 `json:"x"`
	W     float64 `json:"w"`
	H     float64 `json:"h"`
	CY    float64 `json:"cy"`
	R     float64 `json:"r"`
	Text  string  `json:"text"`
	Color string  `json:"color"`
	Tint  string  `json:"tint"`
}

type ncPage struct {
	Bg        map[string]string         `json:"bg"`
	Overrides map[string]map[string]any `json:"overrides"`
	Items     []ncItem                  `json:"items"`
}

func ncLuminance(hex string) float64 {
	var c [3]float64
	for i := range 3 {
		var v int
		for _, ch := range hex[1+2*i : 3+2*i] {
			v = v*16 + strings.IndexRune("0123456789abcdef", ch)
		}
		f := float64(v) / 255
		if f <= 0.03928 {
			c[i] = f / 12.92
		} else {
			c[i] = math.Pow((f+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
}

// ncContrast is the WCAG 2.1 ratio of two colours.
func ncContrast(a, b string) float64 {
	la, lb := ncLuminance(a), ncLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// ncRect is a box in design px on one paper, rotated about its centre, as the runtime draws it (geometry.ts: kept inside the page, then rotated).
type ncRect struct{ l, t, r, b float64 }

func ncDrawn(x, w, h, cy, rot, pageH float64) ncRect {
	bw, bh := w*ncDesignW/1000, h*ncDesignW/1000
	left := math.Min(math.Max(x*ncDesignW/1000, 0), math.Max(ncDesignW-bw, 0))
	top := math.Min(math.Max(cy*pageH/1000-bh/2, 0), math.Max(pageH-bh, 0))
	cx, cyy := left+bw/2, top+bh/2
	a := math.Abs(rot) * math.Pi / 180
	hw := (bw*math.Cos(a) + bh*math.Sin(a)) / 2
	hh := (bw*math.Sin(a) + bh*math.Cos(a)) / 2
	return ncRect{cx - hw, cyy - hh, cx + hw, cyy + hh}
}

func (a ncRect) meets(b ncRect) bool { return a.l < b.r && b.l < a.r && a.t < b.b && b.t < a.b }

// checkNavyPage returns what is wrong with one template page of a kind, as short sentences (none when it passes). safeMM is the manifest's safe margin.
func checkNavyPage(kind string, raw json.RawMessage, safeMM float64) []string {
	var p ncPage
	if err := json.Unmarshal(raw, &p); err != nil {
		return []string{"unreadable page: " + err.Error()}
	}
	var out []string
	bad := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	if len(p.Items) > MaxItemsPerPage {
		bad("%d items", len(p.Items))
	}
	if _, ok := p.Bg["media"]; ok {
		bad("a picture background")
	}
	// contrast: the page colour against the text the book draws on it, in every theme (a template cannot recolour it); a text item against the page colour as well
	bg := ncOwnBg[kind]
	if c, ok := p.Bg["color"]; ok {
		bg = c
	}
	for theme, tok := range ncThemes {
		for _, t := range ncOwnText[kind] {
			if r := ncContrast(tok[t], tok[bg]); r < ncMinRatio {
				bad("%s on %s is %.2f:1 in %s", t, bg, r, theme)
			}
		}
		for _, it := range p.Items {
			if it.T == "text" {
				if r := ncContrast(tok[it.Color], tok[bg]); r < ncMinRatio {
					bad("text item %s (%s on %s) is %.2f:1 in %s", it.ID, it.Color, bg, r, theme)
				}
			}
		}
	}
	for _, it := range p.Items {
		switch it.T {
		case "asset":
		case "text":
			if !ncNeutralText.MatchString(it.Text) {
				bad("text item %s holds words: %q", it.ID, it.Text)
			}
		default:
			bad("item %s is a %s item", it.ID, it.T)
		}
	}
	// geometry on every paper: inside the safe margin and inside the frame's inner rule (the rotated outline: a tilt must not take a corner across it), and no decoration on a
	// text element that has a box of its own (for the elements the template only tilts, tools/print-check does it on the real page)
	for name, mm := range ncPapersMM {
		pageH := ncDesignW * mm[1] / mm[0]
		safe := safeMM / mm[0] * ncDesignW
		in := math.Max(safe, ncFrameInset[kind]+ncRuleGap*b2f(ncFrameInset[kind] > 0))
		for _, it := range p.Items {
			r := ncDrawn(it.X, it.W, it.H, it.CY, it.R, pageH)
			if r.l < in-0.01 || r.t < in-0.01 || r.r > ncDesignW-in+0.01 || r.b > pageH-in+0.01 {
				bad("%s on %s leaves the %.0f px inner margin (%.0f,%.0f to %.0f,%.0f)", it.ID, name, in, r.l, r.t, r.r, r.b)
			}
		}
		for id, o := range p.Overrides {
			if !contains(ncTextElements, id) || o["x"] == nil || o["w"] == nil || o["h"] == nil || o["cy"] == nil {
				continue
			}
			num := func(k string) float64 { v, _ := o[k].(float64); return v }
			e := ncDrawn(num("x"), num("w"), num("h"), num("cy"), num("r"), pageH)
			for _, it := range p.Items {
				if it.T == "asset" && ncDrawn(it.X, it.W, it.H, it.CY, it.R, pageH).meets(e) {
					bad("%s sits on the text element %s on %s", it.ID, id, name)
				}
			}
		}
	}
	return out
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestNavyClassicTemplateGates(t *testing.T) {
	d, _ := templates.Design("navy-classic")
	all, errs := Shipped()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for _, k := range d.Layout.PageKinds {
		var list []PageTemplate
		for _, pt := range all["navy-classic"] {
			if pt.Kind == k.ID {
				list = append(list, pt)
			}
		}
		if len(list) < ncMinPerKind {
			t.Errorf("%s: %d templates, want at least %d", k.ID, len(list), ncMinPerKind)
		}
		ids, en, vi, pages := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]string{}
		for _, pt := range list {
			if ids[pt.ID] || en[strings.ToLower(pt.LabelEN)] || vi[strings.ToLower(pt.LabelVI)] {
				t.Errorf("%s: id or label of %s is not unique", k.ID, pt.ID)
			}
			ids[pt.ID], en[strings.ToLower(pt.LabelEN)], vi[strings.ToLower(pt.LabelVI)] = true, true, true
			for _, l := range []string{pt.LabelEN, pt.LabelVI} {
				if utf8.RuneCountInString(l) > ncMaxLabel {
					t.Errorf("%s: label %q is longer than %d characters (the gallery card)", pt.ID, l, ncMaxLabel)
				}
			}
			if other, dup := pages[string(pt.Page)]; dup {
				t.Errorf("%s and %s have the same page", pt.ID, other)
			}
			pages[string(pt.Page)] = pt.ID
			for _, problem := range checkNavyPage(k.ID, pt.Page, float64(d.Layout.SafeMarginMM)) {
				t.Errorf("%s: %s", pt.ID, problem)
			}
		}
	}
}

// every template is offered by template-list with both labels, as the gallery shows it
func TestNavyClassicTemplatesAreListed(t *testing.T) {
	for _, k := range []string{"cover", "profile", "notes", "back"} {
		got := templatesFor("navy-classic", k)
		if len(got) < ncMinPerKind {
			t.Errorf("%s: %d listed", k, len(got))
		}
		for _, pt := range got {
			if pt.LabelEN == "" || pt.LabelVI == "" || pt.ItemCount > MaxItemsPerPage {
				t.Errorf("%+v", pt)
			}
		}
	}
}

// the contrast table above is the one of navy.css: each value is in the file, and the file still has the three themes
func TestNavyThemeTableMatchesCSS(t *testing.T) {
	css, err := os.ReadFile("../../web/src/book/templates/navy.css")
	if err != nil {
		t.Fatal(err)
	}
	for theme, tok := range ncThemes {
		if !strings.Contains(string(css), `data-theme="`+theme+`"`) {
			t.Errorf("theme %s is not in navy.css", theme)
		}
		for name, hex := range tok {
			if !strings.Contains(string(css), hex) {
				t.Errorf("%s %s %s is not in navy.css", theme, name, hex)
			}
		}
	}
}

// The gates are only worth anything if they bite: a bad page is refused for each rule.
func TestNavyGatesCatchBadPages(t *testing.T) {
	good := `{"bg":{"color":"paper"},"items":[{"id":"a1b2c3","t":"asset","x":450,"w":100,"h":100,"cy":500,"r":0,"asset":"shape.star","tint":"bronze"}]}`
	cases := []struct{ name, kind, page, want string }{
		{"good", "profile", good, ""},
		{"low contrast: a cream page under the cover's light text", "cover", `{"bg":{"color":"paper"}}`, "paper on paper"},
		{"low contrast: gold page under the book's own bronze", "notes", `{"bg":{"color":"gold"}}`, "bronze on gold"},
		{"low contrast: gold text on a cream page", "notes", `{"items":[{"id":"a1b2c3","t":"text","x":100,"w":800,"h":100,"cy":500,"text":"2026","font":"body","size":20,"color":"gold"}]}`, "text item a1b2c3"},
		{"prose", "back", `{"items":[{"id":"a1b2c3","t":"text","x":100,"w":800,"h":100,"cy":500,"text":"Cảm ơn các bạn","font":"body","size":20,"color":"paper"}]}`, "holds words"},
		{"a picture", "cover", `{"bg":{"media":"01J9Z3K6V8Q4M7N2P5R8N00001"}}`, "a picture background"},
		{"an image item", "cover", `{"items":[{"id":"a1b2c3","t":"image","x":400,"w":200,"h":200,"cy":500,"media":"01J9Z3K6V8Q4M7N2P5R8N00001"}]}`, "image item"},
		{"outside the safe margin", "profile", `{"items":[{"id":"a1b2c3","t":"asset","x":0,"w":100,"h":100,"cy":500,"asset":"shape.star"}]}`, "leaves the"},
		{"across the frame's inner rule (inside the safe margin, outside the frame)", "cover", `{"items":[{"id":"a1b2c3","t":"asset","x":57,"w":100,"h":100,"cy":500,"asset":"shape.star"}]}`, "leaves the 48 px"},
		{"a tilt takes a corner over the rule", "cover", `{"items":[{"id":"a1b2c3","t":"asset","x":60,"w":20,"h":300,"cy":500,"r":90,"asset":"shape.rect"}]}`, "leaves the"},
		{"a sticker on the title", "cover", `{"overrides":{"cover.title":{"x":100,"w":700,"h":150,"cy":300}},"items":[{"id":"a1b2c3","t":"asset","x":400,"w":150,"h":150,"cy":300,"asset":"shape.star"}]}`, "sits on the text element cover.title"},
	}
	for _, c := range cases {
		got := strings.Join(checkNavyPage(c.kind, json.RawMessage(c.page), 8), "; ")
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q, got %q", c.name, c.want, got)
		}
	}
}
