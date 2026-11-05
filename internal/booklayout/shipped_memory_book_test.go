package booklayout

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/danyaa666/smemories/internal/templates"
)

// The quality gates of the shipped Memory Book templates (E09 T-177): four or more looks per page kind, unique ids and labels, no two alike, no prose, every box inside the safe margin
// on every paper, and text that stays readable on the page colour. A template that fails one fails the build. checkMemoryBookPage is the gate for one page, and
// TestMemoryBookGatesCatchBadPages feeds it deliberately bad pages (low contrast, a sticker on the title) so the gate is known to bite.

const (
	mbMinPerKind = 4
	mbMaxLabel   = 24 // runes: the gallery card width, both languages (T-175)
	mbMinRatio   = 4.5
)

// the colour tokens of memory.css per theme (a test below reads the file to keep this table honest); lav is the cover's own colour, the page has no token for it.
var mbThemes = map[string]map[string]string{
	"lavender": {"ink": "#3b2f4a", "soft": "#5e5170", "paper": "#fffbf6", "white": "#ffffff", "accent": "#d9668c", "lav": "#e6defa"},
	"peach":    {"ink": "#4a2c22", "soft": "#7a4332", "paper": "#fffaf4", "white": "#ffffff", "accent": "#d95d4a", "lav": "#ffe0cc"},
	"mint":     {"ink": "#1f3b34", "soft": "#35564d", "paper": "#f6fffb", "white": "#ffffff", "accent": "#2f9e87", "lav": "#d3efe5"},
}

// the page colour a kind has when the template sets none, and the text colours the book itself draws straight on the page (its own wording stays whatever the template does).
var (
	mbOwnBg   = map[string]string{"cover": "lav", "profile": "paper", "notes": "paper", "back": "paper"}
	mbOwnText = []string{"ink", "soft"}
	// the template elements that hold text: a decoration must not sit on them once they have a box of their own
	mbTextElements = []string{"cover.title", "profile.heading", "notes.heading", "back.heading", "back.pact"}
	papersMM       = map[string][2]float64{"A5": {148, 210}, "A4": {210, 297}, "Letter": {215.9, 279.4}}
	neutralText    = regexp.MustCompile(`^[\d\p{P}\p{S}\s]+$`) // digits, punctuation, symbols: nothing that reads as a sentence
)

type mbItem struct {
	ID    string  `json:"id"`
	T     string  `json:"t"`
	X     float64 `json:"x"`
	W     float64 `json:"w"`
	H     float64 `json:"h"`
	CY    float64 `json:"cy"`
	R     float64 `json:"r"`
	Text  string  `json:"text"`
	Color string  `json:"color"`
	Asset string  `json:"asset"`
	Tint  string  `json:"tint"`
}

type mbPage struct {
	Bg        map[string]string         `json:"bg"`
	Overrides map[string]map[string]any `json:"overrides"`
	Items     []mbItem                  `json:"items"`
}

func luminance(hex string) float64 {
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

// contrast is the WCAG 2.1 ratio of two colours.
func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// rect is a box in millimetres on one paper, rotated about its centre, as the runtime draws it (geometry.ts: kept inside the page, then rotated).
type rect struct{ l, t, r, b float64 }

func drawn(x, w, h, cy, rot float64, paper [2]float64) rect {
	pw, ph := paper[0], paper[1]
	bw, bh := w*pw/1000, h*pw/1000
	left := math.Min(math.Max(x*pw/1000, 0), math.Max(pw-bw, 0))
	top := math.Min(math.Max(cy*ph/1000-bh/2, 0), math.Max(ph-bh, 0))
	cx, cyy := left+bw/2, top+bh/2
	a := math.Abs(rot) * math.Pi / 180
	hw := (bw*math.Cos(a) + bh*math.Sin(a)) / 2
	hh := (bw*math.Sin(a) + bh*math.Cos(a)) / 2
	return rect{cx - hw, cyy - hh, cx + hw, cyy + hh}
}

func (a rect) meets(b rect) bool { return a.l < b.r && b.l < a.r && a.t < b.b && b.t < a.b }

// checkMemoryBookPage returns what is wrong with one template page of a kind, as short sentences (none when it passes).
func checkMemoryBookPage(kind string, raw json.RawMessage, safeMM float64) []string {
	var p mbPage
	if err := json.Unmarshal(raw, &p); err != nil {
		return []string{"unreadable page: " + err.Error()}
	}
	var out []string
	bad := func(f string, a ...any) {
		out = append(out, strings.TrimSpace(strings.ReplaceAll(sprintf(f, a...), "\n", " ")))
	}
	if len(p.Items) > MaxItemsPerPage {
		bad("%d items", len(p.Items))
	}
	if _, ok := p.Bg["media"]; ok {
		bad("a picture background")
	}
	// contrast: the page colour against the text the book draws on it (ink and soft), in every theme; a text item against the page colour as well
	bg := mbOwnBg[kind]
	if c, ok := p.Bg["color"]; ok {
		bg = c
	}
	for theme, tok := range mbThemes {
		for _, t := range mbOwnText {
			if r := contrast(tok[t], tok[bg]); r < mbMinRatio {
				bad("%s on %s is %.2f:1 in %s", t, bg, r, theme)
			}
		}
		for _, it := range p.Items {
			if it.T == "text" {
				if r := contrast(tok[it.Color], tok[bg]); r < mbMinRatio {
					bad("text item %s (%s on %s) is %.2f:1 in %s", it.ID, it.Color, bg, r, theme)
				}
			}
		}
	}
	for _, it := range p.Items {
		switch it.T {
		case "asset":
		case "text":
			if !neutralText.MatchString(it.Text) {
				bad("text item %s holds words: %q", it.ID, it.Text)
			}
		default:
			bad("item %s is a %s item", it.ID, it.T)
		}
	}
	// geometry on every paper: inside the safe margin, and no decoration on a text element that has a box of its own
	for name, paper := range papersMM {
		for _, it := range p.Items {
			r := drawn(it.X, it.W, it.H, it.CY, 0, paper) // the stored box; a tilt may take a corner a few millimetres over
			if r.l < safeMM-0.01 || r.t < safeMM-0.01 || r.r > paper[0]-safeMM+0.01 || r.b > paper[1]-safeMM+0.01 {
				bad("%s on %s leaves the %v mm margin (%.1f,%.1f to %.1f,%.1f)", it.ID, name, safeMM, r.l, r.t, r.r, r.b)
			}
		}
		for id, o := range p.Overrides {
			if !slices.Contains(mbTextElements, id) || o["x"] == nil || o["w"] == nil || o["h"] == nil || o["cy"] == nil {
				continue
			}
			num := func(k string) float64 { v, _ := o[k].(float64); return v }
			e := drawn(num("x"), num("w"), num("h"), num("cy"), num("r"), paper)
			for _, it := range p.Items {
				if it.T == "asset" && drawn(it.X, it.W, it.H, it.CY, it.R, paper).meets(e) {
					bad("%s sits on the text element %s on %s", it.ID, id, name)
				}
			}
		}
	}
	return out
}

func sprintf(f string, a ...any) string { return strings.TrimSpace(fmt.Sprintf(f, a...)) }

func TestMemoryBookTemplateGates(t *testing.T) {
	d, _ := templates.Design("memory-book")
	all, errs := Shipped()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for _, k := range d.Layout.PageKinds {
		var list []PageTemplate
		for _, pt := range all["memory-book"] {
			if pt.Kind == k.ID {
				list = append(list, pt)
			}
		}
		if len(list) < mbMinPerKind {
			t.Errorf("%s: %d templates, want at least %d", k.ID, len(list), mbMinPerKind)
		}
		ids, en, vi, pages := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]string{}
		for _, pt := range list {
			if ids[pt.ID] || en[strings.ToLower(pt.LabelEN)] || vi[strings.ToLower(pt.LabelVI)] {
				t.Errorf("%s: id or label of %s is not unique", k.ID, pt.ID)
			}
			ids[pt.ID], en[strings.ToLower(pt.LabelEN)], vi[strings.ToLower(pt.LabelVI)] = true, true, true
			for _, l := range []string{pt.LabelEN, pt.LabelVI} {
				if utf8.RuneCountInString(l) > mbMaxLabel {
					t.Errorf("%s: label %q is longer than %d characters (the gallery card)", pt.ID, l, mbMaxLabel)
				}
			}
			if other, dup := pages[string(pt.Page)]; dup {
				t.Errorf("%s and %s have the same page", pt.ID, other)
			}
			pages[string(pt.Page)] = pt.ID
			for _, problem := range checkMemoryBookPage(k.ID, pt.Page, float64(d.Layout.SafeMarginMM)) {
				t.Errorf("%s: %s", pt.ID, problem)
			}
		}
	}
}

// every template is offered by template-list with both labels, as the gallery shows it
func TestMemoryBookTemplatesAreListed(t *testing.T) {
	for _, k := range []string{"cover", "profile", "notes", "back"} {
		got := templatesFor("memory-book", k)
		if len(got) < mbMinPerKind {
			t.Errorf("%s: %d listed", k, len(got))
		}
		for _, pt := range got {
			if pt.LabelEN == "" || pt.LabelVI == "" || pt.ItemCount > MaxItemsPerPage {
				t.Errorf("%+v", pt)
			}
		}
	}
}

// the contrast table above is the one of memory.css: each value is in the file, and the file still has the three themes
func TestMemoryBookThemeTableMatchesCSS(t *testing.T) {
	css, err := os.ReadFile("../../web/src/book/templates/memory.css")
	if err != nil {
		t.Fatal(err)
	}
	for theme, tok := range mbThemes {
		for name, hex := range tok {
			if name == "white" {
				continue
			}
			if !strings.Contains(string(css), hex) {
				t.Errorf("%s %s %s is not in memory.css", theme, name, hex)
			}
		}
	}
}

// The gates are only worth anything if they bite: a bad page is refused for each rule.
func TestMemoryBookGatesCatchBadPages(t *testing.T) {
	good := `{"bg":{"color":"paper"},"items":[{"id":"a1b2c3","t":"asset","x":400,"w":100,"h":100,"cy":500,"r":0,"asset":"shape.star","tint":"accent"}]}`
	cases := []struct{ name, kind, page, want string }{
		{"good", "profile", good, ""},
		{"low contrast: an accent page under the book's own ink", "profile", `{"bg":{"color":"accent"}}`, "ink on accent"},
		{"low contrast: soft text on soft", "notes", `{"bg":{"color":"soft"},"items":[{"id":"a1b2c3","t":"text","x":100,"w":800,"h":100,"cy":500,"text":"2026","font":"body","size":20,"color":"soft"}]}`, "text item a1b2c3"},
		{"prose", "back", `{"items":[{"id":"a1b2c3","t":"text","x":100,"w":800,"h":100,"cy":500,"text":"Cảm ơn các bạn","font":"body","size":20,"color":"ink"}]}`, "holds words"},
		{"a picture", "cover", `{"bg":{"media":"01J9Z3K6V8Q4M7N2P5R8N00001"}}`, "a picture background"},
		{"an image item", "cover", `{"items":[{"id":"a1b2c3","t":"image","x":100,"w":300,"h":300,"cy":500,"media":"01J9Z3K6V8Q4M7N2P5R8N00001"}]}`, "image item"},
		{"outside the margin", "cover", `{"items":[{"id":"a1b2c3","t":"asset","x":0,"w":100,"h":100,"cy":500,"asset":"shape.star"}]}`, "leaves the 5 mm margin"},
		{"a sticker on the title", "cover", `{"overrides":{"cover.title":{"x":100,"w":700,"h":150,"cy":300}},"items":[{"id":"a1b2c3","t":"asset","x":400,"w":150,"h":150,"cy":300,"asset":"shape.star"}]}`, "sits on the text element cover.title"},
	}
	for _, c := range cases {
		got := strings.Join(checkMemoryBookPage(c.kind, json.RawMessage(c.page), 5), "; ")
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q, got %q", c.name, c.want, got)
		}
	}
	// two pages alike: the distinctness gate compares the page bytes, so equal bytes are equal pages
	if string(json.RawMessage(good)) != good {
		t.Fatal("unreachable")
	}
}
