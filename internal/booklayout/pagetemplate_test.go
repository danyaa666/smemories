package booklayout

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/danyaa666/smemories/internal/templates"
)

// the gate: every shipped file passes Validate, and the pipeline is proved by content for each page kind of both layout templates.
func TestShippedPageTemplates(t *testing.T) {
	all, errs := Shipped()
	for _, err := range errs {
		t.Error(err)
	}
	for _, tpl := range []string{"memory-book", "navy-classic"} {
		d, _ := templates.Design(tpl)
		for _, k := range d.Layout.PageKinds {
			n := 0
			for _, pt := range all[tpl] {
				if pt.Kind == k.ID {
					n++
				}
			}
			if n == 0 {
				t.Errorf("%s has no shipped page template for kind %s", tpl, k.ID)
			}
		}
	}
	// every file on disk was loaded (none silently skipped), and each has a licence file
	n := 0
	_ = fs.WalkDir(templates.PageTemplateFS(), ".", func(p string, e fs.DirEntry, err error) error {
		if strings.HasSuffix(p, ".json") {
			n++
		}
		return nil
	})
	got := 0
	for _, l := range all {
		got += len(l)
	}
	if n == 0 || n != got {
		t.Errorf("%d files, %d loaded", n, got)
	}
}

const goodPage = `{"bg":{"color":"soft"},"items":[{"id":"a1b2c3","t":"text","x":100,"w":800,"h":100,"cy":300,"r":0,"text":"Xin chào","font":"display","size":30,"color":"accent","align":"center","bold":true},` +
	`{"id":"d4e5f6","t":"asset","x":400,"w":200,"h":200,"cy":500,"r":0,"asset":"shape.star","tint":"accent"}]}`

func file(over map[string]string) string {
	m := map[string]string{"id": `"t1"`, "label_en": `"One"`, "label_vi": `"Một"`, "kind": `"cover"`, "source": `"authored"`, "licence": `"LICENCE.txt"`, "page": goodPage}
	for k, v := range over {
		m[k] = v
	}
	var parts []string
	for k, v := range m {
		if v != "" {
			parts = append(parts, `"`+k+`":`+v)
		}
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func load(files map[string]string) (map[string][]PageTemplate, []error) {
	fsys := fstest.MapFS{"memory-book/LICENCE.txt": {Data: []byte("CC0")}}
	for name, data := range files {
		fsys["memory-book/"+name] = &fstest.MapFile{Data: []byte(data)}
	}
	return LoadPageTemplates(fsys)
}

func TestLoadPageTemplates(t *testing.T) {
	var items []string
	for i := range 41 {
		items = append(items, fmt.Sprintf(`{"id":"i%05d","t":"asset","x":400,"w":100,"h":100,"cy":500,"r":0,"asset":"shape.star"}`, i))
	}
	cases := []struct{ name, data, want string }{
		{"t1.json", file(nil), ""},
		{"unknown-asset.json", file(map[string]string{"id": `"unknown-asset"`, "page": `{"items":[{"id":"a1b2c3","t":"asset","x":400,"w":100,"h":100,"cy":500,"r":0,"asset":"shape.nope"}]}`}), "not a valid page: pages.cover.items[0].asset unknown_id"},
		{"image.json", file(map[string]string{"id": `"image"`, "page": `{"items":[{"id":"a1b2c3","t":"image","x":400,"w":100,"h":100,"cy":500,"r":0,"media":"01J9Z3K6V8Q4M7N2P5R8N00001"}]}`}), "not a valid page"},
		{"media-bg.json", file(map[string]string{"id": `"media-bg"`, "page": `{"bg":{"media":"01J9Z3K6V8Q4M7N2P5R8N00001"}}`}), "not a valid page"},
		{"kind.json", file(map[string]string{"id": `"kind"`, "kind": `"poster"`}), `unknown kind "poster"`},
		{"licence.json", file(map[string]string{"id": `"licence"`, "licence": `"MISSING.txt"`}), "licence file MISSING.txt is missing or empty"},
		{"path.json", file(map[string]string{"id": `"path"`, "licence": `"../x.txt"`}), "licence must name a file"},
		{"source.json", file(map[string]string{"id": `"source"`, "source": `"doory"`}), `source must be "authored"`},
		{"over.json", file(map[string]string{"id": `"over"`, "page": `{"items":[` + strings.Join(items, ",") + `]}`}), "not a valid page"},
		{"name.json", file(map[string]string{"id": `"other"`}), "id must"},
		{"label.json", file(map[string]string{"id": `"label"`, "label_en": `"` + strings.Repeat("x", 61) + `"`}), "label_en and label_vi must be plain text"},
		{"ctl.json", file(map[string]string{"id": `"ctl"`, "label_vi": `"a\u200bb"`}), "label_en and label_vi must be plain text"},
		{"url.json", file(map[string]string{"id": `"url"`, "page": `{"items":[{"id":"a1b2c3","t":"text","x":100,"w":800,"h":100,"cy":300,"r":0,"text":"see https://example.com"}]}`}), "a text item holds a web address"},
		{"unknown-field.json", strings.Replace(file(map[string]string{"id": `"unknown-field"`}), "{", `{"extra":1,`, 1), "bad JSON"},
		{"broken.json", `{"id":`, "bad JSON"},
		{"big.json", file(map[string]string{"id": `"big"`, "label_en": `"` + strings.Repeat("x", MaxTemplateBytes) + `"`}), "larger than"},
	}
	files := map[string]string{}
	for _, c := range cases {
		files[c.name] = c.data
	}
	got, errs := load(files)
	if len(got["memory-book"]) != 1 || got["memory-book"][0].ID != "t1" || got["memory-book"][0].ItemCount != 2 {
		t.Fatalf("good templates: %+v", got)
	}
	for _, c := range cases[1:] {
		found := false
		for _, e := range errs {
			if strings.Contains(e.Error(), "memory-book/"+c.name+": ") && strings.Contains(e.Error(), c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no error containing %q in %v", c.name, c.want, errs)
		}
	}
	if len(errs) != len(cases)-1 {
		t.Errorf("%d errors for %d bad files: %v", len(errs), len(cases)-1, errs)
	}
}

func TestLoadPageTemplatesFolderRules(t *testing.T) {
	_, errs := LoadPageTemplates(fstest.MapFS{"nope/x.json": {Data: []byte(file(nil))}, "stray.json": {Data: []byte("{}")}})
	if len(errs) != 2 {
		t.Fatalf("errors: %v", errs)
	}
}

func TestShippedPageValueIsCanonical(t *testing.T) {
	all, _ := Shipped()
	for _, l := range all {
		for _, pt := range l {
			var v any
			if err := json.Unmarshal(pt.Page, &v); err != nil || len(pt.Page) > MaxTemplateBytes || pt.ItemCount > MaxItemsPerPage {
				t.Errorf("%s: %v", pt.ID, err)
			}
		}
	}
}
