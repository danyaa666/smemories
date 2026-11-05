package templates

import (
	"fmt"
	"strings"
	"testing"
)

// manifestWith is a valid html manifest whose design block is block (raw JSON).
func manifestWith(renderer, block string) []byte {
	r := ""
	if renderer != "" {
		r = fmt.Sprintf(`"renderer":%q,`, renderer)
	}
	return []byte(`{"id":"d","name":{"en":"D","vi":"D"},` + r + `"page_sizes":["A5"],"design":` + block + `}`)
}

const goodBlock = `{"themes":["a","b"],"font_sets":["f"],
  "sections":[{"id":"cover","hideable":false,"movable":false,"fixed":"first"},{"id":"mid","hideable":true,"movable":true,"fixed":null},{"id":"end","hideable":true,"movable":false,"fixed":"last"}],
  "texts":[{"id":"cover.title","max":40}],"images":[{"id":"cover.bg","choices":["x","y"]}]}`

func TestDesignBlockValidation(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	list := func(n int) string { // n unique ids
		ids := make([]string, n)
		for i := range ids {
			ids[i] = fmt.Sprintf(`"i%d"`, i)
		}
		return "[" + strings.Join(ids, ",") + "]"
	}
	sec := func(id, fixed string) string {
		return fmt.Sprintf(`{"id":%q,"hideable":false,"movable":false,"fixed":%s}`, id, fixed)
	}
	cases := []struct {
		name, block, want string
	}{
		{"no themes", `{"themes":[],"font_sets":["f"]}`, "design.themes needs 1..8"},
		{"nine themes", `{"themes":` + list(9) + `,"font_sets":["f"]}`, "design.themes needs 1..8"},
		{"no font sets", `{"themes":["a"],"font_sets":[]}`, "design.font_sets needs 1..6"},
		{"seven font sets", `{"themes":["a"],"font_sets":` + list(7) + `}`, "design.font_sets needs 1..6"},
		{"dup theme", `{"themes":["a","a"],"font_sets":["f"]}`, `id "a" is listed twice`},
		{"upper-case id", `{"themes":["Ab"],"font_sets":["f"]}`, `id "Ab" must match`},
		{"space in id", `{"themes":["a b"],"font_sets":["f"]}`, `must match`},
		{"empty id", `{"themes":[""],"font_sets":["f"]}`, `must match`},
		{"41-char id", `{"themes":["` + long(41) + `"],"font_sets":["f"]}`, `must match`},
		{"dup section", `{"themes":["a"],"font_sets":["f"],"sections":[` + sec("s", "null") + `,` + sec("s", "null") + `]}`, `id "s" is listed twice`},
		{"bad fixed", `{"themes":["a"],"font_sets":["f"],"sections":[` + sec("s", `"middle"`) + `]}`, `fixed "middle"`},
		{"first not first", `{"themes":["a"],"font_sets":["f"],"sections":[` + sec("s", "null") + `,` + sec("c", `"first"`) + `]}`, "must be listed first"},
		{"last not last", `{"themes":["a"],"font_sets":["f"],"sections":[` + sec("e", `"last"`) + `,` + sec("s", "null") + `]}`, "must be listed last"},
		{"two firsts", `{"themes":["a"],"font_sets":["f"],"sections":[` + sec("a", `"first"`) + `,` + sec("b", `"first"`) + `]}`, "more than one section fixed"},
		{"fixed and movable", `{"themes":["a"],"font_sets":["f"],"sections":[{"id":"a","movable":true,"fixed":"first"}]}`, "cannot be movable"},
		{"too many sections", `{"themes":["a"],"font_sets":["f"],"sections":[` + strings.TrimSuffix(strings.Repeat(sec("s", "null")+",", 17), ",") + `]}`, "design.sections has 17"},
		{"text max 0", `{"themes":["a"],"font_sets":["f"],"texts":[{"id":"t","max":0}]}`, "max 0 must be in 1..200"},
		{"text max 201", `{"themes":["a"],"font_sets":["f"],"texts":[{"id":"t","max":201}]}`, "max 201 must be in 1..200"},
		{"dup text", `{"themes":["a"],"font_sets":["f"],"texts":[{"id":"t","max":5},{"id":"t","max":5}]}`, `id "t" is listed twice`},
		{"no choices", `{"themes":["a"],"font_sets":["f"],"images":[{"id":"i","choices":[]}]}`, "choices needs 1..8"},
		{"nine choices", `{"themes":["a"],"font_sets":["f"],"images":[{"id":"i","choices":` + list(9) + `}]}`, "choices needs 1..8"},
		{"dup image", `{"themes":["a"],"font_sets":["f"],"images":[{"id":"i","choices":["x"]},{"id":"i","choices":["x"]}]}`, `id "i" is listed twice`},
		{"dup choice", `{"themes":["a"],"font_sets":["f"],"images":[{"id":"i","choices":["x","x"]}]}`, `id "x" is listed twice`},
		{"unknown key", `{"themes":["a"],"font_sets":["f"],"colors":[]}`, "unknown field"},
		{"go renderer", goodBlock, "only an html template has a design block"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			renderer := "html"
			if c.name == "go renderer" {
				renderer = "" // the default renderer is Go
			}
			_, err := Parse(manifestWith(renderer, c.block))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
	t.Run("valid", func(t *testing.T) {
		tt, err := Parse(manifestWith("html", goodBlock))
		if err != nil {
			t.Fatal(err)
		}
		if tt.Design.Themes[0] != "a" || tt.Design.Sections[0].Pin() != FixedFirst || tt.Design.Sections[1].Pin() != "" {
			t.Errorf("design = %+v", tt.Design)
		}
	})
}

func TestNavyClassicDesign(t *testing.T) {
	d, ok := Design("navy-classic")
	if !ok {
		t.Fatal("navy-classic has no design block")
	}
	if len(d.Themes) == 0 || d.Themes[0] != "navy" || len(d.FontSets) == 0 || d.FontSets[0] != "classic" || len(d.Images) != 0 || d.Images == nil {
		t.Errorf("defaults = %v %v %v", d.Themes, d.FontSets, d.Images)
	}
	var ids []string
	for _, s := range d.Sections {
		ids = append(ids, s.ID)
	}
	if fmt.Sprint(ids) != "[cover profile notes back]" || d.Sections[0].Pin() != FixedFirst || d.Sections[3].Pin() != FixedLast ||
		d.Sections[0].Hideable || !d.Sections[1].Hideable || !d.Sections[1].Movable || d.Sections[2].Hideable || !d.Sections[2].Movable || !d.Sections[3].Hideable {
		t.Errorf("sections = %+v", d.Sections)
	}
	if len(d.Texts) != 5 {
		t.Errorf("texts = %+v", d.Texts)
	}
}

func TestMemoryBookDesign(t *testing.T) {
	d, ok := Design("memory-book")
	if !ok {
		t.Fatal("memory-book has no design block")
	}
	if len(d.Themes) == 0 || d.Themes[0] != "lavender" || d.FontSets[0] != "playful" || d.Images[0].Choices[0] != "dots" {
		t.Errorf("defaults = %v %v %v", d.Themes, d.FontSets, d.Images)
	}
	if len(d.Sections) != 4 || d.Sections[0].Pin() != FixedFirst || d.Sections[3].Pin() != FixedLast {
		t.Errorf("sections = %+v", d.Sections)
	}
	for _, id := range []string{"classic", "modern", "nope"} {
		if _, ok := Design(id); ok {
			t.Errorf("Design(%q) = ok, want none", id)
		}
	}
}

// T-153: the design.layout block is validated when the manifest loads.
func TestLayoutBlockValidation(t *testing.T) {
	const kinds = `"page_kinds":[{"id":"cover","repeats":false,"editable":["cover.title"]}]`
	with := func(l string) string {
		return `{"themes":["a"],"font_sets":["f"],"layout":{` + l + `}}`
	}
	good := kinds + `,"text_fonts":["display"],"colors":["ink"],"text_size":{"min":8,"max":96},"safe_margin_mm":5`
	cases := []struct{ name, block, want string }{
		{"no page kinds", with(`"page_kinds":[],"text_fonts":["d"],"colors":["i"],"text_size":{"min":8,"max":9},"safe_margin_mm":0`), "page_kinds needs 1..8"},
		{"bad kind id", with(strings.Replace(good, `"id":"cover"`, `"id":"Cover"`, 1)), `id "Cover" must match`},
		{"dup kind", with(strings.Replace(good, `}]`, `},{"id":"cover","repeats":true,"editable":[]}]`, 1)), `id "cover" is listed twice`},
		{"dup editable", with(strings.Replace(good, `["cover.title"]`, `["a","a"]`, 1)), `id "a" is listed twice`},
		{"no fonts", with(strings.Replace(good, `["display"]`, `[]`, 1)), "text_fonts needs 1..4"},
		{"no colours", with(strings.Replace(good, `["ink"]`, `[]`, 1)), "colors needs 1..32"},
		{"size inverted", with(strings.Replace(good, `"min":8,"max":96`, `"min":50,"max":20`, 1)), "text_size 50..20"},
		{"size too big", with(strings.Replace(good, `"max":96`, `"max":201`, 1)), "text_size 8..201"},
		{"margin", with(strings.Replace(good, `"safe_margin_mm":5`, `"safe_margin_mm":31`, 1)), "safe_margin_mm 31"},
		{"unknown key", with(good + `,"z":1`), "unknown field"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(manifestWith("html", c.block))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
	tt, err := Parse(manifestWith("html", with(good)))
	if err != nil {
		t.Fatal(err)
	}
	if k, ok := tt.Design.Layout.Kind("cover"); !ok || k.Editable[0] != "cover.title" {
		t.Errorf("layout = %+v", tt.Design.Layout)
	}
	if _, ok := tt.Design.Layout.Kind("nope"); ok {
		t.Error("an unknown kind was found")
	}
	if l := mustDesign(t, "memory-book").Layout; l == nil || len(l.PageKinds) != 4 || !l.PageKinds[2].Repeats {
		t.Errorf("memory-book layout = %+v", l)
	}
	// T-166: Navy Classic has the same four kinds; ids are <kind>.<name> and every one belongs to its kind
	n := mustDesign(t, "navy-classic").Layout
	if n == nil || len(n.PageKinds) != 4 || !n.PageKinds[2].Repeats || n.PageKinds[0].Repeats || n.SafeMarginMM != 8 {
		t.Fatalf("navy-classic layout = %+v", n)
	}
	for _, k := range n.PageKinds {
		for _, id := range k.Editable {
			if !strings.HasPrefix(id, k.ID+".") {
				t.Errorf("navy-classic: %q is not an element of %q", id, k.ID)
			}
		}
	}
	if fmt.Sprint(n.Colors) != "[ink paper gold bronze]" || fmt.Sprint(n.TextFonts) != "[display body]" {
		t.Errorf("navy-classic colours %v fonts %v", n.Colors, n.TextFonts)
	}
}

func mustDesign(t *testing.T, id string) *DesignBlock {
	t.Helper()
	d, ok := Design(id)
	if !ok {
		t.Fatalf("%s has no design block", id)
	}
	return d
}
