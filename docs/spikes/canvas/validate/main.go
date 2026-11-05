//go:build spike

// Validates every converted layout in docs/spikes/canvas/out/layouts with the product's own validator (internal/booklayout) against the
// memory-book manifest. The build tag keeps this spike out of `go build ./...`, `go vet ./...` and CI:
//
//	go run -tags spike ./docs/spikes/canvas/validate [-dir docs/spikes/canvas/out/layouts]
//
// Exit status 1 when any file is rejected. Also writes docs/spikes/canvas/results/validate.json.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/danyaa666/smemories/internal/booklayout"
	"github.com/danyaa666/smemories/internal/templates"
)

func main() {
	dir := flag.String("dir", "docs/spikes/canvas/out/layouts", "converted layouts")
	flag.Parse()
	tpl, ok := templates.Get("memory-book")
	if !ok || tpl.Design == nil || tpl.Design.Layout == nil {
		fmt.Println("memory-book has no layout block")
		os.Exit(2)
	}
	files, _ := filepath.Glob(filepath.Join(*dir, "*.json"))
	sort.Strings(files)
	type result struct {
		File     string               `json:"file"`
		OK       bool                 `json:"ok"`
		Items    int                  `json:"items"`
		Problems []booklayout.Problem `json:"problems,omitempty"`
		Same     bool                 `json:"canonical_same"`
	}
	// negative controls: the validator must refuse these, otherwise "0 rejected" proves nothing
	for name, doc := range map[string]string{
		"unknown asset":      `{"v":1,"pages":{"cover":{"items":[{"id":"abcdef","t":"asset","asset":"shape.nope","x":0,"w":100,"h":100,"cy":500}]}}}`,
		"colour not a token": `{"v":1,"pages":{"cover":{"bg":{"color":"#ff0000"},"items":[]}}}`,
		"box off the page":   `{"v":1,"pages":{"cover":{"items":[{"id":"abcdef","t":"asset","asset":"shape.circle","x":5000,"w":100,"h":100,"cy":500}]}}}`,
		"501 code points":    `{"v":1,"pages":{"cover":{"items":[{"id":"abcdef","t":"text","text":"` + strings.Repeat("a", 501) + `","x":0,"w":100,"h":100,"cy":500}]}}}`,
	} {
		if _, probs := booklayout.Validate([]byte(doc), tpl.Design.Layout, map[string]bool{}); len(probs) == 0 {
			fmt.Println("NEGATIVE CONTROL NOT REFUSED:", name)
			os.Exit(2)
		}
	}
	fmt.Println("4 negative controls refused")
	var out []result
	bad := 0
	perKind := map[string][2]int{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			panic(err)
		}
		doc, probs := booklayout.Validate(raw, tpl.Design.Layout, map[string]bool{})
		var parsed struct {
			Pages map[string]struct {
				Items []any `json:"items"`
			} `json:"pages"`
		}
		_ = json.Unmarshal(raw, &parsed)
		n := 0
		kind := ""
		for k, p := range parsed.Pages {
			n += len(p.Items)
			kind = k
		}
		// the validator re-encodes canonically (compact, keys sorted): the converter's own output must already be that form
		same := strings.TrimSpace(string(raw)) == string(doc)
		out = append(out, result{filepath.Base(f), len(probs) == 0, n, probs, same})
		c := perKind[kind]
		if len(probs) == 0 && same {
			c[0]++
		} else {
			bad++
			fmt.Println("REJECTED", filepath.Base(f), probs)
		}
		c[1]++
		perKind[kind] = c
	}
	b, _ := json.MarshalIndent(map[string]any{"files": len(files), "rejected": bad, "per_kind_ok_of_total": perKind, "results": out}, "", " ")
	_ = os.WriteFile("docs/spikes/canvas/results/validate.json", append(b, '\n'), 0o644)
	fmt.Printf("%d files, %d rejected, per kind (ok, total): %v\n", len(files), bad, perKind)
	if bad > 0 {
		os.Exit(1)
	}
}
