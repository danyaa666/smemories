// Package booklayout stores the free-layout document of a book (E09 stage 2, T-153, ADR 0004): what a student moved, hid or added on
// the pages of their book. The server validates every document against the template manifest's design.layout block, the asset registry
// and the owner's own design images before it stores it, keeps it with optimistic concurrency, and hands a re-validated copy to the
// print route. Format and rules: docs/templates.md, "Layout document".
package booklayout

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/danyaa666/smemories/internal/assets"
	"github.com/danyaa666/smemories/internal/templates"
	"github.com/danyaa666/smemories/internal/textx"
	"github.com/danyaa666/smemories/internal/ulid"
)

// Limits of a document (spec E09_T-153).
const (
	Version         = 1
	MaxBytes        = 128 << 10
	MaxProblems     = 20
	MaxItemsPerPage = 40
	MaxItemsTotal   = 150
	MaxTextRunes    = 500
	MinBox, MaxBox  = 10, 2000
	MaxRadius       = 500
	pagePermille    = 1000 // a page is 1000 units wide, and 1000 units high for cy

	// pageRatioPermille is the A5 page height over its width (210 / 148) in thousandths. A box's height h is in thousandths of the page
	// WIDTH while its centre cy is in thousandths of the page HEIGHT, so cy may leave [0, 1000] by at most h*ratio/2 (the spec's
	// "h/2*k"). The A5 ratio is the largest of the supported papers, so a box that is legal on any paper passes (paper independent).
	pageRatioPermille = 1419
)

// Problem codes.
const (
	CodeInvalidJSON = "invalid_json"
	CodeBadVersion  = "bad_version"
	CodeWrongType   = "wrong_type"
	CodeUnknownKey  = "unknown_key"
	CodeMissing     = "missing"
	CodeOutOfRange  = "out_of_range"
	CodeTooLong     = "too_long"
	CodeTooMany     = "too_many"
	CodeBadText     = "bad_text"
	CodeBadID       = "bad_id"
	CodeDuplicate   = "duplicate"
	CodeUnknownID   = "unknown_id"
	CodeNotTintable = "not_tintable"
	CodeNotOwnMedia = "not_own_media"
	CodeNotOneOf    = "not_one_of"
)

// Problem is one thing wrong with a document: Path is like pages.cover.items[2].x, Code one of the Code* values.
type Problem struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

type checker struct {
	m       *templates.LayoutBlock
	own     map[string]bool // the owner's design images of this book
	lenient bool            // read path: drop what does not validate instead of reporting it
	probs   []Problem
	n       int             // problems seen, also past the cap
	ids     map[string]bool // item ids seen
	total   int
}

func (c *checker) add(path, code string) {
	c.n++
	if len(c.probs) < MaxProblems {
		c.probs = append(c.probs, Problem{path, code})
	}
}

// Validate checks raw against the manifest block and the owner's design images (ownMediaIDs; see MediaRefs) and returns the document
// to store (the canonical compact form of what was validated) or at most MaxProblems problems. Nothing is stored for a rejected document.
func Validate(raw []byte, m *templates.LayoutBlock, ownMediaIDs map[string]bool) ([]byte, []Problem) {
	c := &checker{m: m, own: ownMediaIDs, ids: map[string]bool{}}
	out := c.document(raw)
	if c.n > 0 {
		return nil, c.probs
	}
	return out, nil
}

// Sanitize is Validate for the print route: a part that no longer validates (an asset removed from the registry, a deleted media id, a
// page kind or element the manifest dropped) is left out and the rest is kept. It returns nil when nothing usable is left.
func Sanitize(raw []byte, m *templates.LayoutBlock, ownMediaIDs map[string]bool) []byte {
	c := &checker{m: m, own: ownMediaIDs, ids: map[string]bool{}, lenient: true}
	return c.document(raw)
}

// MediaRefs lists the media ids a document names (page backgrounds and image items), without checking anything else, so the caller can
// ask the media service which of them are the owner's design images. At most 2*MaxItemsTotal ids, each once.
func MediaRefs(raw []byte) []string {
	root, _ := decode(raw)
	var ids []string
	add := func(v any) {
		if s, ok := v.(string); ok && ulid.Valid(s) && !slices.Contains(ids, s) && len(ids) < 2*MaxItemsTotal {
			ids = append(ids, s)
		}
	}
	pages, _ := root["pages"].(map[string]any)
	for _, p := range pages {
		page, _ := p.(map[string]any)
		if bg, ok := page["bg"].(map[string]any); ok {
			add(bg["media"])
		}
		items, _ := page["items"].([]any)
		for _, it := range items {
			if m, ok := it.(map[string]any); ok && m["t"] == "image" {
				add(m["media"])
			}
		}
	}
	return ids
}

func decode(raw []byte) (map[string]any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var root map[string]any
	if err := dec.Decode(&root); err != nil || root == nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return root, true
}

func (c *checker) document(raw []byte) []byte {
	root, ok := decode(raw)
	if !ok {
		c.add("", CodeInvalidJSON)
		return nil
	}
	c.keys("", root, "v", "pages")
	if v, ok := root["v"]; !ok {
		c.add("v", CodeMissing)
	} else if n, ok := c.integer("v", v); ok && n != Version {
		c.add("v", CodeBadVersion)
	}
	pages, ok := root["pages"].(map[string]any)
	if !ok {
		c.add("pages", wrongOrMissing(root, "pages"))
	}
	if c.lenient && c.n > 0 { // a document of another version, or without pages, is not read at all
		return nil
	}
	if !ok {
		return nil
	}
	names := make([]string, 0, len(pages))
	for k := range pages {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		p := "pages." + k
		kind, known := c.m.Kind(k)
		if !known {
			c.drop(pages, k, p, CodeUnknownID)
			continue
		}
		mark := c.n
		if page, ok := pages[k].(map[string]any); !ok {
			c.add(p, CodeWrongType)
		} else {
			c.page(p, page, kind)
		}
		if c.lenient && c.n != mark {
			c.n = mark
			delete(pages, k)
		}
	}
	if !c.lenient && c.total > MaxItemsTotal {
		c.add("pages", CodeTooMany)
	}
	return c.out(root)
}

// out is the canonical document: compact, keys sorted, HTML characters not escaped (the table stays readable; the web renders text as text).
func (c *checker) out(root map[string]any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(root); err != nil {
		return nil
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// drop reports a problem, or in lenient mode removes the key silently.
func (c *checker) drop(m map[string]any, key, path, code string) {
	if c.lenient {
		delete(m, key)
		return
	}
	c.add(path, code)
}

func wrongOrMissing(m map[string]any, k string) string {
	if _, ok := m[k]; ok {
		return CodeWrongType
	}
	return CodeMissing
}

// keys reports every key of m not in allowed (lenient: removes it).
func (c *checker) keys(path string, m map[string]any, allowed ...string) {
	for k := range m {
		if !slices.Contains(allowed, k) {
			c.drop(m, k, join(path, k), CodeUnknownKey)
		}
	}
}

func join(path, k string) string {
	if path == "" {
		return k
	}
	return path + "." + k
}

func (c *checker) page(p string, page map[string]any, kind templates.LayoutPageKind) {
	c.keys(p, page, "bg", "overrides", "items")
	if bg, ok := page["bg"]; ok {
		c.background(p+".bg", page, bg)
	}
	if ov, ok := page["overrides"]; ok {
		m, ok := ov.(map[string]any)
		if !ok {
			c.add(p+".overrides", CodeWrongType)
		} else {
			c.overrides(p+".overrides", m, kind)
		}
	}
	if it, ok := page["items"]; ok {
		arr, ok := it.([]any)
		if !ok {
			c.add(p+".items", CodeWrongType)
		} else {
			page["items"] = c.items(p+".items", arr)
		}
	}
}

func (c *checker) background(p string, page map[string]any, v any) {
	mark := c.n
	bg, ok := v.(map[string]any)
	if !ok {
		c.add(p, CodeWrongType)
	} else {
		c.keys(p, bg, "asset", "media", "color")
		if len(bg) != 1 {
			c.add(p, CodeNotOneOf)
		}
		if s, ok := bg["asset"]; ok && c.str(p+".asset", s) {
			c.asset(p+".asset", s.(string))
		}
		if s, ok := bg["media"]; ok && c.str(p+".media", s) {
			c.media(p+".media", s.(string))
		}
		if s, ok := bg["color"]; ok && c.str(p+".color", s) {
			c.token(p+".color", s.(string), c.m.Colors)
		}
	}
	if c.lenient && c.n != mark {
		c.n = mark
		delete(page, "bg")
	}
}

func (c *checker) overrides(p string, ov map[string]any, kind templates.LayoutPageKind) {
	ids := make([]string, 0, len(ov))
	for id := range ov {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		ep := p + "." + id
		if !slices.Contains(kind.Editable, id) {
			c.drop(ov, id, ep, CodeUnknownID)
			continue
		}
		mark := c.n
		if o, ok := ov[id].(map[string]any); !ok {
			c.add(ep, CodeWrongType)
		} else {
			c.keys(ep, o, "x", "w", "h", "cy", "r", "hidden")
			c.box(ep, o, false)
			if h, ok := o["hidden"]; ok {
				if _, ok := h.(bool); !ok {
					c.add(ep+".hidden", CodeWrongType)
				}
			}
		}
		if c.lenient && c.n != mark {
			c.n = mark
			delete(ov, id)
		}
	}
}

func (c *checker) items(p string, arr []any) []any {
	limit := min(len(arr), MaxItemsPerPage)
	if !c.lenient { // the items past the page limit are not looked at, but count towards the document total
		c.total += len(arr) - limit
		if len(arr) > limit {
			c.add(p, CodeTooMany)
		}
	}
	kept := make([]any, 0, limit)
	for i, v := range arr[:limit] {
		if c.lenient && c.total >= MaxItemsTotal {
			break
		}
		mark := c.n
		ip := p + "[" + strconv.Itoa(i) + "]"
		if m, ok := v.(map[string]any); !ok {
			c.add(ip, CodeWrongType)
		} else {
			c.item(ip, m)
		}
		if c.lenient && c.n != mark {
			c.n = mark
			continue
		}
		c.total++
		kept = append(kept, v)
	}
	return kept
}

func alnum(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

func (c *checker) item(p string, m map[string]any) {
	if id, ok := m["id"]; !ok {
		c.add(p+".id", CodeMissing)
	} else if c.str(p+".id", id) {
		s := id.(string)
		switch {
		case len(s) < 6 || len(s) > 26 || !alnum(s):
			c.add(p+".id", CodeBadID)
		case c.ids[s]:
			c.add(p+".id", CodeDuplicate)
		default:
			c.ids[s] = true
		}
	}
	t, _ := m["t"].(string)
	switch t {
	case "text":
		c.keys(p, m, "id", "t", "x", "w", "h", "cy", "r", "text", "font", "size", "color", "align", "bold")
		c.text(p, m)
	case "asset":
		c.keys(p, m, "id", "t", "x", "w", "h", "cy", "r", "asset", "tint")
		c.assetItem(p, m)
	case "image":
		c.keys(p, m, "id", "t", "x", "w", "h", "cy", "r", "media", "fit", "radius")
		c.imageItem(p, m)
	default:
		c.add(p+".t", wrongOrMissingOr(m, "t", CodeNotOneOf))
		return
	}
	c.box(p, m, true)
}

func wrongOrMissingOr(m map[string]any, k, other string) string {
	if _, ok := m[k]; !ok {
		return CodeMissing
	}
	return other
}

func (c *checker) text(p string, m map[string]any) {
	if v, ok := m["text"]; !ok {
		c.add(p+".text", CodeMissing)
	} else if c.str(p+".text", v) {
		s := v.(string)
		clean, ok := textx.CleanMultiline(s, 0, math.MaxInt32)
		switch {
		case !ok:
			c.add(p+".text", CodeBadText)
		case utf8.RuneCountInString(clean) > MaxTextRunes:
			c.add(p+".text", CodeTooLong)
		default:
			m["text"] = clean
		}
	}
	if v, ok := m["font"]; ok && c.str(p+".font", v) {
		c.token(p+".font", v.(string), c.m.TextFonts)
	}
	if v, ok := m["color"]; ok && c.str(p+".color", v) {
		c.token(p+".color", v.(string), c.m.Colors)
	}
	if v, ok := m["size"]; ok {
		if n, ok := c.integer(p+".size", v); ok && (n < int64(c.m.TextSize.Min) || n > int64(c.m.TextSize.Max)) {
			c.add(p+".size", CodeOutOfRange)
		}
	}
	if v, ok := m["align"]; ok && c.str(p+".align", v) && !slices.Contains([]string{"left", "center", "right"}, v.(string)) {
		c.add(p+".align", CodeNotOneOf)
	}
	if v, ok := m["bold"]; ok {
		if _, ok := v.(bool); !ok {
			c.add(p+".bold", CodeWrongType)
		}
	}
}

func (c *checker) assetItem(p string, m map[string]any) {
	var a assets.Asset
	found := false
	if v, ok := m["asset"]; !ok {
		c.add(p+".asset", CodeMissing)
	} else if c.str(p+".asset", v) {
		a, found = assets.Get(v.(string))
		if !found {
			c.add(p+".asset", CodeUnknownID)
		}
	}
	if v, ok := m["tint"]; ok && c.str(p+".tint", v) {
		c.token(p+".tint", v.(string), c.m.Colors)
		if found && !a.Tintable {
			c.add(p+".tint", CodeNotTintable)
		}
	}
}

func (c *checker) imageItem(p string, m map[string]any) {
	if v, ok := m["media"]; !ok {
		c.add(p+".media", CodeMissing)
	} else if c.str(p+".media", v) {
		c.media(p+".media", v.(string))
	}
	if v, ok := m["fit"]; ok && c.str(p+".fit", v) && !slices.Contains([]string{"cover", "contain"}, v.(string)) {
		c.add(p+".fit", CodeNotOneOf)
	}
	if v, ok := m["radius"]; ok {
		if n, ok := c.integer(p+".radius", v); ok && (n < 0 || n > MaxRadius) {
			c.add(p+".radius", CodeOutOfRange)
		}
	}
}

// box checks x, w, h, cy and r. An item (full) needs x, w, h and cy; an override gives any of them.
func (c *checker) box(p string, m map[string]any, full bool) {
	v := map[string]int64{}
	for _, k := range []string{"x", "w", "h", "cy", "r"} {
		raw, ok := m[k]
		if !ok {
			if full && k != "r" {
				c.add(p+"."+k, CodeMissing)
			}
			continue
		}
		if n, ok := c.integer(p+"."+k, raw); ok {
			v[k] = n
			m[k] = n
		}
	}
	for _, k := range []string{"w", "h"} {
		if n, ok := v[k]; ok && (n < MinBox || n > MaxBox) {
			c.add(p+"."+k, CodeOutOfRange)
		}
	}
	if n, ok := v["r"]; ok && (n < -180 || n > 180) {
		c.add(p+".r", CodeOutOfRange)
	}
	// x is the left edge: at most half the box may leave the page on either side (2x >= -w and 2x <= 2000-w). Without w, the widest box.
	if x, ok := v["x"]; ok {
		lo, hi := -int64(MaxBox), int64(2*pagePermille-MinBox) // doubled units; without w, the widest and the narrowest box
		if w, known := v["w"]; known {
			lo, hi = -w, 2*pagePermille-w
		}
		if 2*x < lo || 2*x > hi {
			c.add(p+".x", CodeOutOfRange)
		}
	}
	if cy, ok := v["cy"]; ok {
		h, known := v["h"]
		if !known {
			h = MaxBox
		}
		slack := h * pageRatioPermille / (2 * pagePermille)
		if cy < -slack || cy > pagePermille+slack {
			c.add(p+".cy", CodeOutOfRange)
		}
	}
}

// integer accepts a JSON number written as an integer (no fraction, no exponent).
func (c *checker) integer(p string, v any) (int64, bool) {
	num, ok := v.(json.Number)
	if !ok {
		c.add(p, CodeWrongType)
		return 0, false
	}
	n, err := strconv.ParseInt(string(num), 10, 64)
	if err != nil {
		code := CodeWrongType
		if !strings.ContainsAny(string(num), ".eE") {
			code = CodeOutOfRange // an integer too big for int64
		}
		c.add(p, code)
		return 0, false
	}
	return n, true
}

func (c *checker) str(p string, v any) bool {
	if _, ok := v.(string); !ok {
		c.add(p, CodeWrongType)
		return false
	}
	return true
}

func (c *checker) token(p, s string, allowed []string) {
	if !slices.Contains(allowed, s) {
		c.add(p, CodeUnknownID)
	}
}

func (c *checker) asset(p, id string) {
	if !assets.Has(id) {
		c.add(p, CodeUnknownID)
	}
}

func (c *checker) media(p, id string) {
	if !c.own[id] {
		c.add(p, CodeNotOwnMedia)
	}
}
