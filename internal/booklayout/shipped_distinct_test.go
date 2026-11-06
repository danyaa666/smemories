package booklayout

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// The distinctness gate of the shipped page templates (E09 T-180). Two templates of the same page kind must not be the same look: the byte comparison of the other gates lets a copy with
// one colour changed through, and a gallery of four cards that differ only by page colour or by a sticker moved a few pixels is not a choice. So the decoration of every template is
// drawn on a coarse grid of the A5 page (the union of its item boxes, rotated as drawn, and of the boxes of any template element it moves or resizes), and two templates of a kind fail
// when the overlap of the two unions, intersection over union, is distinctMaxIoU or more. The page colour (bg) is not part of the comparison, and neither is a tilt that moves no box:
// a colour-only twin has the same boxes, so its overlap is 1. Two pages with no decoration at all are twins as well.

const (
	distinctMaxIoU = 0.8
	distinctCell   = 8.0 // design px per grid cell
)

// decorationMask is the set of grid cells that the boxes of a page cover on the A5 page.
func decorationMask(raw json.RawMessage) map[[2]int]bool {
	var p ncPage
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}
	pageH := ncDesignW * ncPapersMM["A5"][1] / ncPapersMM["A5"][0]
	mask := map[[2]int]bool{}
	fill := func(r ncRect) {
		for cx := int(r.l / distinctCell); float64(cx)*distinctCell < r.r; cx++ {
			for cy := int(r.t / distinctCell); float64(cy)*distinctCell < r.b; cy++ {
				mask[[2]int{cx, cy}] = true
			}
		}
	}
	for _, it := range p.Items {
		fill(ncDrawn(it.X, it.W, it.H, it.CY, it.R, pageH))
	}
	for _, o := range p.Overrides {
		if o["x"] == nil || o["w"] == nil || o["h"] == nil || o["cy"] == nil {
			continue // a tilt, or a move of one side only, adds no box
		}
		num := func(k string) float64 { v, _ := o[k].(float64); return v }
		fill(ncDrawn(num("x"), num("w"), num("h"), num("cy"), num("r"), pageH))
	}
	return mask
}

// decorationOverlap is the intersection over union of the decoration of two pages (1 when both have none).
func decorationOverlap(a, b json.RawMessage) float64 {
	ma, mb := decorationMask(a), decorationMask(b)
	if len(ma) == 0 && len(mb) == 0 {
		return 1
	}
	inter := 0
	for c := range ma {
		if mb[c] {
			inter++
		}
	}
	return float64(inter) / float64(len(ma)+len(mb)-inter)
}

// tooAlike lists the pairs of one kind whose overlap reaches the limit, as short sentences.
func tooAlike(list []PageTemplate) []string {
	var out []string
	for i := range list {
		for j := i + 1; j < len(list); j++ {
			if iou := decorationOverlap(list[i].Page, list[j].Page); iou >= distinctMaxIoU {
				out = append(out, fmt.Sprintf("%s and %s: decoration overlap %.2f (limit %.2f)", list[i].ID, list[j].ID, iou, distinctMaxIoU))
			}
		}
	}
	return out
}

func TestShippedTemplatesAreDistinct(t *testing.T) {
	all, errs := Shipped()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for tpl, list := range all {
		byKind := map[string][]PageTemplate{}
		for _, pt := range list {
			byKind[pt.Kind] = append(byKind[pt.Kind], pt)
		}
		var kinds []string
		for k := range byKind {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			for _, problem := range tooAlike(byKind[k]) {
				t.Errorf("%s %s: %s", tpl, k, problem)
			}
			worst := 0.0
			for i := range byKind[k] {
				for j := i + 1; j < len(byKind[k]); j++ {
					worst = max(worst, decorationOverlap(byKind[k][i].Page, byKind[k][j].Page))
				}
			}
			t.Logf("%s %s: %d templates, highest overlap of two looks %.2f", tpl, k, len(byKind[k]), worst)
		}
	}
}

// The gate is only worth anything if it bites: a copy of a shipped page with its colour changed, or with every item moved by a few pixels, is refused; a different look passes.
func TestDistinctGateCatchesTwins(t *testing.T) {
	all, _ := Shipped()
	var a, b PageTemplate
	for _, pt := range all["memory-book"] {
		if pt.ID == "back-stickers" {
			a = pt
		}
		if pt.ID == "back-thanks" {
			b = pt
		}
	}
	var doc map[string]any
	if err := json.Unmarshal(a.Page, &doc); err != nil {
		t.Fatal(err)
	}
	twin := func(mutate func(map[string]any)) PageTemplate {
		var c map[string]any
		_ = json.Unmarshal(a.Page, &c)
		mutate(c)
		raw, _ := json.Marshal(c)
		return PageTemplate{ID: "twin", Kind: a.Kind, Page: raw}
	}
	colour := twin(func(c map[string]any) { c["bg"] = map[string]any{"color": "white"} })
	if string(colour.Page) == string(a.Page) {
		t.Fatal("the colour twin is the same bytes: the test is not testing the colour")
	}
	nudged := twin(func(c map[string]any) {
		for _, it := range c["items"].([]any) {
			m := it.(map[string]any)
			m["x"] = m["x"].(float64) + 6 // about 5 px
		}
	})
	cases := []struct {
		name      string
		list      []PageTemplate
		wantAlike bool
	}{
		{"a copy with the page colour changed", []PageTemplate{a, colour}, true},
		{"a copy with every item moved 5 px", []PageTemplate{a, nudged}, true},
		{"two pages with no decoration", []PageTemplate{{ID: "x", Page: json.RawMessage(`{"bg":{"color":"white"}}`)}, {ID: "y", Page: json.RawMessage(`{"bg":{"color":"paper"}}`)}}, true},
		{"two different looks of the kind", []PageTemplate{a, b}, false},
	}
	for _, c := range cases {
		got := strings.Join(tooAlike(c.list), "; ")
		if (got != "") != c.wantAlike {
			t.Errorf("%s: want alike=%v, got %q", c.name, c.wantAlike, got)
		}
	}
}

// The limit itself: pages whose decoration overlaps at 0.79 pass and at 0.81 fail (QA of T-180: without a pair on each side of 0.8 a ">=" turned
// into ">" or a dropped override survived). One box of 99.96 px against a box nested in it, so the overlap is the ratio of their widths in grid cells
// (79/100, 80/100 and 81/100); the same box given as a full-box override instead of an item counts the same, a tilt-only override adds nothing.
func TestDistinctLimitBoundary(t *testing.T) {
	box := func(w int) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"items":[{"id":"aaaaaa","t":"asset","asset":"star","x":0,"w":%d,"h":100,"cy":500,"r":0}]}`, w))
	}
	boxAsOverride := func(w int) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"overrides":{"cover.photo":{"x":0,"w":%d,"h":100,"cy":500,"r":0}}}`, w))
	}
	base := box(980) // 100 cells wide
	for _, c := range []struct {
		name string
		page json.RawMessage
		iou  float64
		like bool
	}{
		{"79 of 100 cells", box(770), 0.79, false},
		{"80 of 100 cells: the limit itself fails", box(780), 0.80, true},
		{"81 of 100 cells", box(790), 0.81, true},
		{"79 of 100 cells, as an override", boxAsOverride(770), 0.79, false},
		{"81 of 100 cells, as an override", boxAsOverride(790), 0.81, true},
		{"a tilt only", json.RawMessage(`{"overrides":{"cover.photo":{"r":5}}}`), 0, false},
	} {
		got := decorationOverlap(base, c.page)
		if c.iou > 0 && (got < c.iou-0.005 || got > c.iou+0.005) {
			t.Errorf("%s: overlap %.3f, want %.2f", c.name, got, c.iou)
		}
		list := []PageTemplate{{ID: "a", Page: base}, {ID: "b", Page: c.page}}
		if alike := len(tooAlike(list)) > 0; alike != c.like {
			t.Errorf("%s: overlap %.3f, alike=%v, want %v", c.name, got, alike, c.like)
		}
	}
}
