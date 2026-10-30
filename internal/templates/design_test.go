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

func TestMemoryBookDesign(t *testing.T) {
	d, ok := Design("memory-book")
	if !ok {
		t.Fatal("memory-book has no design block")
	}
	if len(d.Themes) != 3 || d.Themes[0] != "lavender" || d.FontSets[0] != "playful" || d.Images[0].Choices[0] != "dots" {
		t.Errorf("defaults = %v %v %v", d.Themes, d.FontSets, d.Images)
	}
	if len(d.Sections) != 4 || d.Sections[0].Pin() != FixedFirst || d.Sections[3].Pin() != FixedLast {
		t.Errorf("sections = %+v", d.Sections)
	}
	for _, id := range []string{"classic", "modern", "navy-classic", "nope"} {
		if _, ok := Design(id); ok {
			t.Errorf("Design(%q) = ok, want none", id)
		}
	}
}
