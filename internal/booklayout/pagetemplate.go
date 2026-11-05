package booklayout

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/templates"
	"github.com/danyaa666/smemories/internal/textx"
)

// Page templates (E09 T-172, ADR 0005 Q3): the value of pages.<kind> of a layout document (bg, overrides, items) with a name, shipped as
// embedded files and checked with Validate, the one validator every layout path uses. A shipped file that fails it is a build failure
// (TestShippedPageTemplates) and, at run time, is left out and counted (see Shipped). Format and how to add one: docs/templates.md.
const (
	// MaxTemplateBytes caps one shipped file (40 items of 500 characters fit with room to spare).
	MaxTemplateBytes = 64 << 10
	// MaxLabelRunes caps a template name.
	MaxLabelRunes = 60
	// SourceAuthored is the only source a shipped template may have: we drew it ourselves. Anything derived from another project waits for
	// the owner's licence decision (ADR 0005 Q6, Q-064).
	SourceAuthored = "authored"
)

var templateIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// PageTemplate is one shipped page template as the read route returns it (Page is the canonical page value, as Validate re-encodes it).
type PageTemplate struct {
	ID        string          `json:"id"`
	LabelEN   string          `json:"label_en"`
	LabelVI   string          `json:"label_vi"`
	Kind      string          `json:"kind"`
	ItemCount int             `json:"item_count"`
	Page      json.RawMessage `json:"page"`
}

// templateFile is the shipped file: the response members plus where it came from.
type templateFile struct {
	ID      string          `json:"id"`
	LabelEN string          `json:"label_en"`
	LabelVI string          `json:"label_vi"`
	Kind    string          `json:"kind"`
	Source  string          `json:"source"`
	Licence string          `json:"licence"` // a file in the same folder
	Page    json.RawMessage `json:"page"`
}

// LoadPageTemplates reads every <book template id>/<id>.json of fsys, checks each against that book template's layout block with Validate
// (as a one-page document, no media) and returns the good ones by book template id, sorted by English label then id, and one error per bad
// file, each naming the file. It never panics: the caller decides what a bad file costs (Shipped).
func LoadPageTemplates(fsys fs.FS) (map[string][]PageTemplate, []error) {
	out := map[string][]PageTemplate{}
	var errs []error
	dirs, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return out, []error{apperr.Wrap(apperr.Internal, err, "page templates")}
	}
	for _, d := range dirs {
		if !d.IsDir() {
			errs = append(errs, bad(d.Name(), "a file outside a book template folder"))
			continue
		}
		files, err := fs.Glob(fsys, d.Name()+"/*.json")
		if err != nil {
			errs = append(errs, bad(d.Name(), err.Error()))
			continue
		}
		for _, f := range files {
			t, err := loadOne(fsys, d.Name(), f)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			out[d.Name()] = append(out[d.Name()], *t)
		}
	}
	for _, l := range out {
		slices.SortFunc(l, func(a, b PageTemplate) int {
			return cmp.Or(strings.Compare(a.LabelEN, b.LabelEN), strings.Compare(a.ID, b.ID))
		})
	}
	return out, errs
}

func bad(file, msg string) error { return apperr.New(apperr.Internal, "page template "+file+": "+msg) }

func loadOne(fsys fs.FS, tpl, file string) (*PageTemplate, error) {
	fail := func(msg string) (*PageTemplate, error) { return nil, bad(file, msg) }
	block := layoutOf(&tpl)
	if block == nil {
		return fail("the folder is not a book template with a layout block")
	}
	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		return fail(err.Error())
	}
	if len(data) > MaxTemplateBytes {
		return fail("larger than " + strconv.Itoa(MaxTemplateBytes) + " bytes")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f templateFile
	if err := dec.Decode(&f); err != nil {
		return fail("bad JSON: " + err.Error())
	}
	if _, err := dec.Token(); err != io.EOF {
		return fail("bad JSON: data after the object")
	}
	if !templateIDRe.MatchString(f.ID) || f.ID != strings.TrimSuffix(path.Base(file), ".json") {
		return fail("id must be lower-case letters, digits and hyphens and equal the file name")
	}
	for _, l := range []string{f.LabelEN, f.LabelVI} {
		if c, ok := textx.Clean(l, 1, MaxLabelRunes); !ok || c != l {
			return fail("label_en and label_vi must be plain text of 1 to " + strconv.Itoa(MaxLabelRunes) + " characters")
		}
	}
	kind, ok := block.Kind(f.Kind)
	if !ok {
		return fail("unknown kind " + strconv.Quote(f.Kind))
	}
	if f.Source != SourceAuthored {
		return fail(`source must be "authored"`)
	}
	if f.Licence == "" || f.Licence != path.Base(f.Licence) || strings.HasPrefix(f.Licence, ".") {
		return fail("licence must name a file in the same folder")
	}
	if lic, err := fs.ReadFile(fsys, path.Join(tpl, f.Licence)); err != nil || len(bytes.TrimSpace(lic)) == 0 {
		return fail("licence file " + f.Licence + " is missing or empty")
	}
	if len(f.Page) == 0 || f.Page[0] != '{' {
		return fail("page must be an object")
	}
	doc := `{"v":1,"pages":{` + strconv.Quote(kind.ID) + `:` + string(f.Page) + `}}`
	out, probs := Validate([]byte(doc), block, nil) // no media: an image item or a picture background is refused
	if len(probs) > 0 {
		parts := make([]string, len(probs))
		for i, p := range probs {
			parts[i] = p.Path + " " + p.Code
		}
		return fail("not a valid page: " + strings.Join(parts, ", "))
	}
	var v struct {
		Pages map[string]json.RawMessage `json:"pages"`
	}
	_ = json.Unmarshal(out, &v)
	page := v.Pages[kind.ID]
	var p struct {
		Items []struct {
			Text string `json:"text"`
		} `json:"items"`
	}
	_ = json.Unmarshal(page, &p)
	for _, it := range p.Items {
		if strings.Contains(it.Text, "://") || strings.Contains(strings.ToLower(it.Text), "www.") {
			return fail("a text item holds a web address")
		}
	}
	return &PageTemplate{ID: f.ID, LabelEN: f.LabelEN, LabelVI: f.LabelVI, Kind: kind.ID, ItemCount: len(p.Items), Page: page}, nil
}

var (
	shippedOnce sync.Once
	shipped     map[string][]PageTemplate
	shippedErrs []error
)

// Shipped returns the page templates embedded in the binary by book template id, and the errors of the files that were left out. It reads
// the files once. The caller decides what an error costs: the API stops in dev and test and logs and counts it in production, so a broken
// file never takes the service down.
func Shipped() (map[string][]PageTemplate, []error) {
	shippedOnce.Do(func() { shipped, shippedErrs = LoadPageTemplates(templates.PageTemplateFS()) })
	return shipped, shippedErrs
}

// templatesFor returns the shipped templates of a book template, optionally of one kind; the slice is new, the entries are shared.
func templatesFor(tpl, kind string) []PageTemplate {
	all, _ := Shipped()
	var out []PageTemplate
	for _, t := range all[tpl] {
		if kind == "" || t.Kind == kind {
			out = append(out, t)
		}
	}
	return out
}
