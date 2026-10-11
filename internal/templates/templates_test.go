package templates

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/danyaa666/smemories/internal/notefields"
)

func TestListHasBuiltIns(t *testing.T) {
	l := List()
	have := map[string]bool{}
	for _, i := range l {
		have[i.ID] = true
	}
	// The built-ins are a minimum: a new template dropped into embed/ must not need a test edit.
	for _, id := range []string{"classic", "modern"} {
		if !have[id] {
			t.Fatalf("List() lacks built-in %q: %+v", id, l)
		}
	}
	for _, i := range l {
		if i.Name["en"] == "" || i.Name["vi"] == "" {
			t.Errorf("%s: names %v", i.ID, i.Name)
		}
		if tt, ok := Get(i.ID); !ok || tt.ID != i.ID {
			t.Errorf("Get(%q) failed", i.ID)
		}
	}
	if _, ok := Get("nope"); ok {
		t.Error("Get of an unknown id succeeded")
	}
}

// mutate parses the classic template into a generic tree, applies fn and re-encodes it.
func mutate(t *testing.T, fn func(root map[string]any)) []byte {
	t.Helper()
	data, err := embedded.ReadFile("embed/classic.json")
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	fn(root)
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func el(root map[string]any, page, element int) map[string]any {
	return root["pages"].([]any)[page].(map[string]any)["elements"].([]any)[element].(map[string]any)
}

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name string
		fn   func(root map[string]any)
		want []string // all substrings must appear
	}{
		{"unknown slot", func(r map[string]any) { el(r, 0, 2)["slot"] = "tittle" },
			[]string{`template "classic"`, "page 1 (cover)", "element 3", `unknown slot "tittle"`}},
		{"slot of another page kind", func(r map[string]any) { el(r, 0, 2)["slot"] = "full_name" },
			[]string{"page 1 (cover)", "element 3", `unknown slot "full_name"`}},
		{"wrong element type for slot", func(r map[string]any) { el(r, 0, 0)["type"] = "text"; el(r, 0, 0)["size"] = 12 },
			[]string{"page 1 (cover)", "element 1", `slot "cover_photo" is a image slot`}},
		{"outside the page", func(r map[string]any) { el(r, 0, 2)["x"] = 100 },
			[]string{"page 1 (cover)", "element 3", "outside the 148 x 210 mm area"}},
		{"negative position", func(r map[string]any) { el(r, 1, 0)["y"] = -1 },
			[]string{"page 2 (profile)", "element 1", "outside"}},
		{"zero size", func(r map[string]any) { el(r, 0, 2)["w"] = 0 }, []string{"element 3", "w and h must be > 0"}},
		{"invalid unit", func(r map[string]any) { r["unit"] = "pt" }, []string{`invalid unit "pt"`}},
		{"missing unit", func(r map[string]any) { delete(r, "unit") }, []string{"invalid unit"}},
		{"missing font", func(r map[string]any) { r["theme"].(map[string]any)["font"] = "Comic Sans" },
			[]string{`missing font "Comic Sans"`}},
		{"bad colour", func(r map[string]any) { r["theme"].(map[string]any)["colors"].(map[string]any)["ink"] = "red" },
			[]string{`colour "ink"`}},
		{"unknown colour name", func(r map[string]any) { el(r, 0, 2)["color"] = "pink" }, []string{"element 3", `color "pink"`}},
		{"min_size above size", func(r map[string]any) { el(r, 0, 2)["min_size"] = 40 }, []string{"element 3", "min_size"}},
		{"bad align", func(r map[string]any) { el(r, 0, 2)["align"] = "justify" }, []string{`unknown align "justify"`}},
		{"bad type", func(r map[string]any) { el(r, 0, 2)["type"] = "video" }, []string{`unknown element type "video"`}},
		{"unknown page size", func(r map[string]any) { r["page_sizes"] = []any{"A3"} }, []string{`unknown page size "A3"`}},
		{"missing vi name", func(r map[string]any) { delete(r["name"].(map[string]any), "vi") }, []string{`language "vi"`}},
		{"bad id", func(r map[string]any) { r["id"] = "../x" }, []string{"id must match"}},
		{"duplicate page kind", func(r map[string]any) {
			p := r["pages"].([]any)
			r["pages"] = append(p, p[0])
		}, []string{`more than one "cover" page`}},
		{"missing back page", func(r map[string]any) { r["pages"] = r["pages"].([]any)[:3] }, []string{`no "back" page`}},
		{"notes without flow", func(r map[string]any) { delete(r["pages"].([]any)[2].(map[string]any), "flow") },
			[]string{"page 3 (notes)", "needs a flow"}},
		{"flow element outside the item", func(r map[string]any) {
			f := r["pages"].([]any)[2].(map[string]any)["flow"].(map[string]any)
			f["elements"].([]any)[1].(map[string]any)["w"] = 500
		}, []string{"page 3 (notes), flow", "element 2", "outside"}},
		{"page slot used inside flow", func(r map[string]any) {
			f := r["pages"].([]any)[2].(map[string]any)["flow"].(map[string]any)
			f["elements"].([]any)[1].(map[string]any)["slot"] = "title"
		}, []string{"flow", `unknown slot "title"`}},
		{"too many elements", func(r map[string]any) {
			p := r["pages"].([]any)[0].(map[string]any)
			e := p["elements"].([]any)
			for len(e) <= maxElements {
				e = append(e, e[1])
			}
			p["elements"] = e
		}, []string{"page 1 (cover)", "exceeds 64"}},
		{"circle needs a square", func(r map[string]any) { el(r, 1, 0)["shape"] = "circle" }, []string{"a circle needs w == h"}},
		{"A5 reference with Letter", func(r map[string]any) { r["page_sizes"] = []any{"A5", "Letter"} },
			[]string{`template "classic"`, `page size "Letter"`, "aspect ratio", "A5 reference"}},
		{"Letter reference with A5", func(r map[string]any) { r["reference"] = "Letter"; r["page_sizes"] = []any{"Letter", "A5"} },
			[]string{`template "classic"`, `page size "A5"`, "Letter reference"}},
		{"Letter reference with A4", func(r map[string]any) { r["reference"] = "Letter"; r["page_sizes"] = []any{"A4"} },
			[]string{`page size "A4"`, "aspect ratio"}},
		{"bad reference", func(r map[string]any) { r["reference"] = "A4" }, []string{`invalid reference "A4"`}},
		{"outside the Letter reference page", func(r map[string]any) {
			r["reference"] = "Letter"
			r["page_sizes"] = []any{"Letter"}
			el(r, 0, 2)["x"] = 215
		}, []string{"element 3", "outside the 215.9 x 279.4 mm area"}},
		{"unknown field", func(r map[string]any) { el(r, 0, 2)["colour"] = "ink" }, []string{"unknown field"}},
		{"note field not in the catalogue", func(r map[string]any) {
			r["note_fields"] = []any{map[string]any{"id": "name"}, map[string]any{"id": "crush"}}
		},
			[]string{`template "classic"`, `note_fields entry 2 ("crush")`, "catalogue"}},
		{"note field twice", func(r map[string]any) {
			r["note_fields"] = []any{map[string]any{"id": "name"}, map[string]any{"id": "name"}}
		},
			[]string{`note_fields entry 2 ("name")`, "listed twice"}},
		{"too many note fields", func(r map[string]any) {
			var l []any
			for range 10 {
				l = append(l, map[string]any{"id": "name"})
			}
			r["note_fields"] = l
		}, []string{"note_fields needs 1..9 entries, has 10"}},
		{"empty note fields", func(r map[string]any) { r["note_fields"] = []any{} }, []string{"note_fields needs 1..9 entries, has 0"}},
		{"unknown note field key", func(r map[string]any) { r["note_fields"] = []any{map[string]any{"id": "name", "optional": true}} },
			[]string{"unknown field"}},
		{"unknown renderer", func(r map[string]any) { r["renderer"] = "svg" }, []string{`unknown renderer "svg"`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(mutate(t, tc.fn))
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err, w)
				}
			}
		})
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "{", "[]", `{"id":"x"} {}`, strings.Repeat(" ", maxSpecBytes+1)} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("Parse(%.20q) succeeded", in)
		}
	}
}

func TestPerPage(t *testing.T) {
	c, _ := Get("classic")
	if n := c.Pages[2].Flow.PerPage(); n != 3 {
		t.Errorf("classic per page = %d, want 3", n)
	}
}

// docs/templates.md must name every slot, so a template can be written from the documentation alone.
func TestDocsListEverySlot(t *testing.T) {
	doc, err := os.ReadFile("../../docs/templates.md")
	if err != nil {
		t.Fatal(err)
	}
	for kind, slots := range Slots {
		for slot := range slots {
			if !strings.Contains(string(doc), "`"+slot+"`") {
				t.Errorf("docs/templates.md does not mention slot %q (%s)", slot, kind)
			}
		}
	}
}

func TestTextBoxMustFitOneLine(t *testing.T) {
	_, err := Parse(mutate(t, func(r map[string]any) { el(r, 0, 2)["h"] = 3 }))
	if err == nil || !strings.Contains(err.Error(), "too short for one line") {
		t.Errorf("err = %v", err)
	}
}

// T-037 AC4: a Letter-reference template may use the whole Letter page and declares only Letter.
func TestLetterReference(t *testing.T) {
	tt, err := Parse(mutate(t, func(r map[string]any) {
		r["id"] = "letter-test"
		r["reference"] = "Letter"
		r["page_sizes"] = []any{"Letter"}
		el(r, 0, 2)["x"] = 90 // w=120: outside A5 (148 wide), inside Letter
	}))
	if err != nil {
		t.Fatal(err)
	}
	if w, h := tt.RefDims(); w != 215.9 || h != 279.4 {
		t.Errorf("RefDims = %g x %g", w, h)
	}
	c, _ := Get("classic")
	if w, h := c.RefDims(); w != 148 || h != 210 || c.Reference != "" {
		t.Errorf("classic reference = %q %g x %g, want the A5 default", c.Reference, w, h)
	}
}

// T-037 AC5: the export and picker helper returns only templates that declare the size.
func TestForPageSize(t *testing.T) {
	ids := func(size string) []string {
		var out []string
		for _, i := range ForPageSize(size) {
			out = append(out, i.ID)
		}
		return out
	}
	for _, size := range []string{"A5", "A4"} {
		got := ids(size)
		if !slices.Contains(got, "classic") || !slices.Contains(got, "modern") {
			t.Errorf("ForPageSize(%s) = %v, want the built-ins", size, got)
		}
	}
	for _, id := range ids("Letter") {
		if id == "classic" || id == "modern" {
			t.Errorf("ForPageSize(Letter) offers %q, which does not declare Letter", id)
		}
		if tt, _ := Get(id); !tt.Supports("Letter") {
			t.Errorf("ForPageSize(Letter) offers %q without Letter", id)
		}
	}
	if got := ids("A3"); len(got) != 0 {
		t.Errorf("ForPageSize(A3) = %v", got)
	}
}

func TestNoteFields(t *testing.T) {
	for _, id := range []string{"classic", "modern"} {
		got, ok := NoteFields(id)
		if !ok || !slices.Equal(got, notefields.Default()) {
			t.Errorf("NoteFields(%q) = %v, %v; want the default set", id, got, ok)
		}
	}
	if got, ok := NoteFields("nope"); ok || got != nil {
		t.Errorf("NoteFields(unknown) = %v, %v", got, ok)
	}
	// The result is a copy: changing it must not change the template.
	a, _ := NoteFields("classic")
	a[0].Required = false
	if b, _ := NoteFields("classic"); !b[0].Required {
		t.Error("NoteFields returned shared state")
	}
}

func TestNoteFieldsCustomAndDefault(t *testing.T) {
	custom := mutate(t, func(r map[string]any) {
		r["note_fields"] = []any{map[string]any{"id": "name", "required": true}, map[string]any{"id": "how_we_met"}, map[string]any{"id": "message", "required": true}}
	})
	tt, err := Parse(custom)
	if err != nil {
		t.Fatal(err)
	}
	want := []notefields.FieldRef{{ID: "name", Required: true}, {ID: "how_we_met"}, {ID: "message", Required: true}}
	if got := tt.noteFields(); !slices.Equal(got, want) {
		t.Errorf("custom = %v, want %v", got, want)
	}
	none := mutate(t, func(r map[string]any) { delete(r, "note_fields") })
	if tt, err = Parse(none); err != nil || !slices.Equal(tt.noteFields(), notefields.Default()) {
		t.Errorf("no note_fields: %v, %v", tt.noteFields(), err)
	}
}

// A built-in whose note_fields are all valid catalogue entries works with notefields.Info (the form builder).
func TestBuiltInNoteFieldsResolve(t *testing.T) {
	for _, i := range List() {
		refs, _ := NoteFields(i.ID)
		if _, err := notefields.Info(refs); err != nil {
			t.Errorf("%s: %v", i.ID, err)
		}
	}
}

const htmlManifest = `{"id":"polaroid","renderer":"html","name":{"en":"Polaroid","vi":"Polaroid"},"page_sizes":["A5","A4","Letter"],
 "note_fields":[{"id":"name","required":true},{"id":"best_memory"}]}`

func TestHTMLManifest(t *testing.T) {
	tt, err := Parse([]byte(htmlManifest))
	if err != nil {
		t.Fatal(err)
	}
	if !tt.IsHTML() || len(tt.Pages) != 0 || !tt.Supports("Letter") {
		t.Errorf("parsed %+v", tt)
	}
	// Registry with the built-ins plus the html manifest: List returns both kinds, NoteFields and ForPageSize see it.
	reg := mustLoadFS(fstest.MapFS{
		"t/polaroid.json": {Data: []byte(htmlManifest)},
		"t/classic.json":  {Data: mustRead(t, "embed/classic.json")},
	}, "t", assetRoot)
	var got []string
	for _, i := range listOf(reg) {
		got = append(got, i.ID+":"+i.Renderer)
	}
	if want := []string{"classic:go", "polaroid:html"}; !slices.Equal(got, want) {
		t.Errorf("list = %v, want %v", got, want)
	}
	if want := []notefields.FieldRef{{ID: "name", Required: true}, {ID: "best_memory"}}; !slices.Equal(reg["polaroid"].noteFields(), want) {
		t.Errorf("html note fields = %v", reg["polaroid"].noteFields())
	}
	// Without note_fields the default applies.
	nf, err := Parse([]byte(`{"id":"p2","renderer":"html","name":{"en":"P","vi":"P"},"page_sizes":["A5"]}`))
	if err != nil || !slices.Equal(nf.noteFields(), notefields.Default()) {
		t.Errorf("html without note_fields: %v, %v", nf, err)
	}
}

func TestHTMLManifestErrors(t *testing.T) {
	tests := []struct{ name, json, want string }{
		{"pages", `{"id":"p","renderer":"html","name":{"en":"P","vi":"P"},"page_sizes":["A5"],"pages":[{"kind":"cover","elements":[]}]}`, "an html template has no pages"},
		{"no sizes", `{"id":"p","renderer":"html","name":{"en":"P","vi":"P"},"page_sizes":[]}`, "page_sizes is empty"},
		{"bad size", `{"id":"p","renderer":"html","name":{"en":"P","vi":"P"},"page_sizes":["A3"]}`, `unknown page size "A3"`},
		{"missing vi", `{"id":"p","renderer":"html","name":{"en":"P"},"page_sizes":["A5"]}`, `language "vi"`},
		{"bad note field", `{"id":"p","renderer":"html","name":{"en":"P","vi":"P"},"page_sizes":["A5"],"note_fields":[{"id":"x"}]}`, `note_fields entry 1 ("x")`},
	}
	for _, tc := range tests {
		_, err := Parse([]byte(tc.json))
		if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), `template "p"`) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := embedded.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
