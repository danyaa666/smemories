package templates

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
)

// The JSON files sit directly in embed/; background assets live in embed/<template id>/.
//
//go:embed embed
var embedded embed.FS

// assetRoot is embed/ as a file system, the base of every asset path in a spec.
var assetRoot = mustSub(embedded, "embed")

func mustSub(f fs.FS, dir string) fs.FS {
	s, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return s
}

const maxSpecBytes = 256 << 10

// Parse decodes and validates one template against the embedded assets. Unknown JSON fields are errors,
// so typos are caught.
func Parse(data []byte) (*Template, error) { return ParseFS(data, assetRoot) }

// ParseFS is Parse with the background assets read from assets (paths in the spec are relative to it).
func ParseFS(data []byte, assets fs.FS) (*Template, error) {
	if len(data) > maxSpecBytes {
		return nil, fmt.Errorf("template spec is %d bytes, the limit is %d", len(data), maxSpecBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var t Template
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("template spec: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("template spec: unexpected data after the JSON object")
	}
	t.assets = assets
	if err := Validate(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

// registry is loaded once at start-up. The embedded JSON is trusted and covered by tests, so a bad file
// is a programming error and panics.
var registry = mustLoad()

func mustLoad() map[string]*Template {
	files, err := fs.Glob(embedded, "embed/*.json")
	if err != nil || len(files) == 0 {
		panic("templates: no embedded templates")
	}
	m := map[string]*Template{}
	for _, f := range files {
		data, err := embedded.ReadFile(f)
		if err != nil {
			panic(err)
		}
		t, err := Parse(data)
		if err != nil {
			panic(fmt.Sprintf("templates: %s: %v", f, err))
		}
		m[t.ID] = t
	}
	return m
}

// Info is the public summary of a template.
type Info struct {
	ID   string
	Name map[string]string
}

// List returns the built-in templates ordered by id.
func List() []Info {
	out := make([]Info, 0, len(registry))
	for _, t := range registry {
		out = append(out, Info{ID: t.ID, Name: t.Name})
	}
	slices.SortFunc(out, func(a, b Info) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Get returns a built-in template by id. The result is shared: do not modify it.
func Get(id string) (*Template, bool) {
	t, ok := registry[id]
	return t, ok
}

// ForPageSize returns the built-in templates that declare support for size, ordered by id. Use it for the
// template picker and the export check: a template that does not list a book's size must not be used for it.
func ForPageSize(size string) []Info {
	var out []Info
	for _, i := range List() {
		if registry[i.ID].Supports(size) {
			out = append(out, i)
		}
	}
	return out
}
