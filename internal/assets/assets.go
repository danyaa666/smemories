// Package assets is the server's list of the sticker, shape and frame assets the book editor may use (epic E09, T-151).
// The pack lives in web/src/book/assets; registry.json here is a byte copy of web/src/book/assets/registry.json made by
// scripts/sync-assets.sh, and a test fails when the two differ. A layout that names an id not listed here is rejected.
package assets

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed registry.json
var raw []byte

// Asset is one registry entry (the fields the server cares about; the web side reads the same file).
type Asset struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"` // sticker, shape or frame
	LabelEN  string   `json:"label_en"`
	LabelVI  string   `json:"label_vi"`
	Tags     []string `json:"tags"`
	Tintable bool     `json:"tintable"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	Licence  string   `json:"licence"` // CC0, OFL or authored
}

var (
	once  sync.Once
	list  []Asset
	index map[string]Asset
)

func load() {
	var f struct {
		Assets []Asset `json:"assets"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		panic(err) // the embedded file is pinned by TestRegistry
	}
	list = f.Assets
	index = make(map[string]Asset, len(list))
	for _, a := range list {
		index[a.ID] = a
	}
}

// All returns every asset in registry order. The slice is shared: do not modify it.
func All() []Asset {
	once.Do(load)
	return list
}

// Get returns the asset with this id.
func Get(id string) (Asset, bool) {
	once.Do(load)
	a, ok := index[id]
	return a, ok
}

// Has reports whether id is a registered asset.
func Has(id string) bool {
	_, ok := Get(id)
	return ok
}
