package booklayout

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/templates"
	"github.com/danyaa666/smemories/internal/ulid"
)

func manifest(t testing.TB) *templates.LayoutBlock {
	t.Helper()
	d, ok := templates.Design("memory-book")
	if !ok || d.Layout == nil {
		t.Fatal("memory-book has no layout block")
	}
	return d.Layout
}

var mediaA = ulid.New(time.Unix(1_800_000_000, 0))

func own(ids ...string) map[string]bool {
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// item builds an item JSON object with a valid box, plus extra members (a JSON fragment such as `"t":"text","text":"x"`).
func item(id, extra string) string {
	return fmt.Sprintf(`{"id":%q,%s,"x":100,"w":400,"h":120,"cy":500,"r":0}`, id, extra)
}

func page(items ...string) string {
	return `{"v":1,"pages":{"cover":{"items":[` + strings.Join(items, ",") + `]}}}`
}

const textItem = `"t":"text","text":"Lớp 12A1","font":"display","size":28,"color":"accent","align":"center","bold":true`

func TestValidateAcceptsAndRoundTrips(t *testing.T) {
	doc := `{"v":1,"pages":{
	  "cover":{"bg":{"asset":"shape.star"},
	    "overrides":{"cover.photo":{"x":120,"w":500,"h":500,"cy":400,"r":0,"hidden":false},"cover.title":{"hidden":true}},
	    "items":[` + item("a1b2c3", textItem) + `,` + item("d4e5f6", `"t":"asset","asset":"shape.star","tint":"accent"`) + `,` +
		item("g7h8i9", `"t":"image","media":"`+mediaA+`","fit":"cover","radius":20`) + `]},
	  "profile":{"bg":{"color":"soft"}},"notes":{},"back":{"bg":{"media":"` + mediaA + `"}}}}`
	out, probs := Validate([]byte(doc), manifest(t), own(mediaA))
	if len(probs) != 0 {
		t.Fatalf("problems: %v", probs)
	}
	var a, b any
	_ = json.Unmarshal([]byte(doc), &a)
	_ = json.Unmarshal(out, &b)
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Fatalf("not the same document:\n%s\n%s", doc, out)
	}
	again, probs := Validate(out, manifest(t), own(mediaA))
	if len(probs) != 0 || string(again) != string(out) {
		t.Fatalf("the canonical form is not stable: %v\n%s\n%s", probs, out, again)
	}
}

func TestValidateRejects(t *testing.T) {
	long := strings.Repeat("a", 501)
	cases := []struct{ name, doc, path, code string }{
		{"not json", `{"v":1,`, "", CodeInvalidJSON},
		{"array", `[]`, "", CodeInvalidJSON},
		{"null", `null`, "", CodeInvalidJSON},
		{"trailing data", `{"v":1,"pages":{}} {}`, "", CodeInvalidJSON},
		{"no version", `{"pages":{}}`, "v", CodeMissing},
		{"version 2", `{"v":2,"pages":{}}`, "v", CodeBadVersion},
		{"version string", `{"v":"1","pages":{}}`, "v", CodeWrongType},
		{"no pages", `{"v":1}`, "pages", CodeMissing},
		{"pages array", `{"v":1,"pages":[]}`, "pages", CodeWrongType},
		{"unknown top key", `{"v":1,"pages":{},"x":1}`, "x", CodeUnknownKey},
		{"unknown page kind", `{"v":1,"pages":{"poster":{}}}`, "pages.poster", CodeUnknownID},
		{"page not object", `{"v":1,"pages":{"cover":[]}}`, "pages.cover", CodeWrongType},
		{"unknown page key", `{"v":1,"pages":{"cover":{"z":1}}}`, "pages.cover.z", CodeUnknownKey},
		{"override of an element the manifest does not list", `{"v":1,"pages":{"cover":{"overrides":{"cover.nope":{"x":1}}}}}`, "pages.cover.overrides.cover.nope", CodeUnknownID},
		{"override of another page's element", `{"v":1,"pages":{"cover":{"overrides":{"back.pact":{"hidden":true}}}}}`, "pages.cover.overrides.back.pact", CodeUnknownID},
		{"override unknown key", `{"v":1,"pages":{"cover":{"overrides":{"cover.photo":{"z":1}}}}}`, "pages.cover.overrides.cover.photo.z", CodeUnknownKey},
		{"override hidden type", `{"v":1,"pages":{"cover":{"overrides":{"cover.photo":{"hidden":1}}}}}`, "pages.cover.overrides.cover.photo.hidden", CodeWrongType},
		{"override w too small", `{"v":1,"pages":{"cover":{"overrides":{"cover.photo":{"w":9}}}}}`, "pages.cover.overrides.cover.photo.w", CodeOutOfRange},
		{"bg two keys", `{"v":1,"pages":{"cover":{"bg":{"asset":"shape.star","color":"ink"}}}}`, "pages.cover.bg", CodeNotOneOf},
		{"bg empty", `{"v":1,"pages":{"cover":{"bg":{}}}}`, "pages.cover.bg", CodeNotOneOf},
		{"bg unknown asset", `{"v":1,"pages":{"cover":{"bg":{"asset":"shape.nope"}}}}`, "pages.cover.bg.asset", CodeUnknownID},
		{"bg unknown colour", `{"v":1,"pages":{"cover":{"bg":{"color":"pink"}}}}`, "pages.cover.bg.color", CodeUnknownID},
		{"bg media not own", `{"v":1,"pages":{"cover":{"bg":{"media":"` + mediaA + `"}}}}`, "pages.cover.bg.media", CodeNotOwnMedia},
		{"items not array", `{"v":1,"pages":{"cover":{"items":{}}}}`, "pages.cover.items", CodeWrongType},
		{"item not object", page(`1`), "pages.cover.items[0]", CodeWrongType},
		{"item no id", page(`{"t":"text","text":"x","x":1,"w":10,"h":10,"cy":1}`), "pages.cover.items[0].id", CodeMissing},
		{"item id too short", page(item("abcde", textItem)), "pages.cover.items[0].id", CodeBadID},
		{"item id too long", page(item(strings.Repeat("a", 27), textItem)), "pages.cover.items[0].id", CodeBadID},
		{"item id characters", page(item("abc-def", textItem)), "pages.cover.items[0].id", CodeBadID},
		{"duplicate item id", page(item("abcdef", textItem), item("abcdef", textItem)), "pages.cover.items[1].id", CodeDuplicate},
		{"no type", page(item("abcdef", `"text":"x"`)), "pages.cover.items[0].t", CodeMissing},
		{"unknown type", page(item("abcdef", `"t":"video"`)), "pages.cover.items[0].t", CodeNotOneOf},
		{"item unknown key", page(item("abcdef", textItem+`,"z":1`)), "pages.cover.items[0].z", CodeUnknownKey},
		{"asset key on text", page(item("abcdef", textItem+`,"asset":"shape.star"`)), "pages.cover.items[0].asset", CodeUnknownKey},
		{"text missing", page(item("abcdef", `"t":"text"`)), "pages.cover.items[0].text", CodeMissing},
		{"text type", page(item("abcdef", `"t":"text","text":5`)), "pages.cover.items[0].text", CodeWrongType},
		{"text 501 runes", page(item("abcdef", `"t":"text","text":"`+long+`"`)), "pages.cover.items[0].text", CodeTooLong},
		{"text control character", page(item("abcdef", `"t":"text","text":"a\u0007b"`)), "pages.cover.items[0].text", CodeBadText},
		{"text carriage return", page(item("abcdef", `"t":"text","text":"a\rb"`)), "pages.cover.items[0].text", CodeBadText},
		{"text bidi override", page(item("abcdef", `"t":"text","text":"a\u202eb"`)), "pages.cover.items[0].text", CodeBadText},
		{"unknown font", page(item("abcdef", `"t":"text","text":"x","font":"comic"`)), "pages.cover.items[0].font", CodeUnknownID},
		{"unknown text colour", page(item("abcdef", `"t":"text","text":"x","color":"pink"`)), "pages.cover.items[0].color", CodeUnknownID},
		{"size below", page(item("abcdef", `"t":"text","text":"x","size":7`)), "pages.cover.items[0].size", CodeOutOfRange},
		{"size above", page(item("abcdef", `"t":"text","text":"x","size":97`)), "pages.cover.items[0].size", CodeOutOfRange},
		{"size fraction", page(item("abcdef", `"t":"text","text":"x","size":12.5`)), "pages.cover.items[0].size", CodeWrongType},
		{"align", page(item("abcdef", `"t":"text","text":"x","align":"justify"`)), "pages.cover.items[0].align", CodeNotOneOf},
		{"bold type", page(item("abcdef", `"t":"text","text":"x","bold":"yes"`)), "pages.cover.items[0].bold", CodeWrongType},
		{"asset missing", page(item("abcdef", `"t":"asset"`)), "pages.cover.items[0].asset", CodeMissing},
		{"unknown asset", page(item("abcdef", `"t":"asset","asset":"sticker.nope"`)), "pages.cover.items[0].asset", CodeUnknownID},
		{"tint on a non-tintable asset", page(item("abcdef", `"t":"asset","asset":"sticker.gold-star","tint":"accent"`)), "pages.cover.items[0].tint", CodeNotTintable},
		{"unknown tint", page(item("abcdef", `"t":"asset","asset":"shape.star","tint":"pink"`)), "pages.cover.items[0].tint", CodeUnknownID},
		{"image media missing", page(item("abcdef", `"t":"image"`)), "pages.cover.items[0].media", CodeMissing},
		{"image media not own", page(item("abcdef", `"t":"image","media":"`+mediaA+`"`)), "pages.cover.items[0].media", CodeNotOwnMedia},
		{"image media not an id", page(item("abcdef", `"t":"image","media":"x"`)), "pages.cover.items[0].media", CodeNotOwnMedia},
		{"image fit", page(item("abcdef", `"t":"image","media":"`+mediaA+`","fit":"fill"`)), "pages.cover.items[0].fit", CodeNotOneOf},
		{"image radius", page(item("abcdef", `"t":"image","media":"`+mediaA+`","radius":501`)), "pages.cover.items[0].radius", CodeOutOfRange},
		{"image radius negative", page(item("abcdef", `"t":"image","media":"`+mediaA+`","radius":-1`)), "pages.cover.items[0].radius", CodeOutOfRange},
		{"x missing", page(`{"id":"abcdef",` + textItem + `,"w":10,"h":10,"cy":1}`), "pages.cover.items[0].x", CodeMissing},
		{"x as string", page(`{"id":"abcdef",` + textItem + `,"x":"1","w":10,"h":10,"cy":1}`), "pages.cover.items[0].x", CodeWrongType},
		{"x as float", page(`{"id":"abcdef",` + textItem + `,"x":1.5,"w":10,"h":10,"cy":1}`), "pages.cover.items[0].x", CodeWrongType},
		{"x as exponent", page(`{"id":"abcdef",` + textItem + `,"x":1e2,"w":10,"h":10,"cy":1}`), "pages.cover.items[0].x", CodeWrongType},
		{"x huge", page(`{"id":"abcdef",` + textItem + `,"x":99999999999999999999,"w":10,"h":10,"cy":1}`), "pages.cover.items[0].x", CodeOutOfRange},
		{"r above", page(`{"id":"abcdef",` + textItem + `,"x":1,"w":10,"h":10,"cy":1,"r":181}`), "pages.cover.items[0].r", CodeOutOfRange},
		{"r below", page(`{"id":"abcdef",` + textItem + `,"x":1,"w":10,"h":10,"cy":1,"r":-181}`), "pages.cover.items[0].r", CodeOutOfRange},
		{"w 9", box(`"x":0,"w":9,"h":10,"cy":0`), "pages.cover.items[0].w", CodeOutOfRange},
		{"w 2001", box(`"x":0,"w":2001,"h":10,"cy":0`), "pages.cover.items[0].w", CodeOutOfRange},
		{"h 9", box(`"x":0,"w":10,"h":9,"cy":0`), "pages.cover.items[0].h", CodeOutOfRange},
		{"h 2001", box(`"x":0,"w":10,"h":2001,"cy":0`), "pages.cover.items[0].h", CodeOutOfRange},
		{"x one left of the limit", box(`"x":-201,"w":400,"h":10,"cy":0`), "pages.cover.items[0].x", CodeOutOfRange},
		{"x one right of the limit", box(`"x":801,"w":400,"h":10,"cy":0`), "pages.cover.items[0].x", CodeOutOfRange},
		{"x odd width, one left", box(`"x":-200,"w":399,"h":10,"cy":0`), "pages.cover.items[0].x", CodeOutOfRange},
		{"cy above the page", box(`"x":0,"w":10,"h":100,"cy":-72`), "pages.cover.items[0].cy", CodeOutOfRange}, // 100*1419/2000 = 70
		{"cy below the page", box(`"x":0,"w":10,"h":100,"cy":1071`), "pages.cover.items[0].cy", CodeOutOfRange},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, probs := Validate([]byte(c.doc), manifest(t), own())
			if out != nil || len(probs) == 0 {
				t.Fatalf("accepted: %s", out)
			}
			for _, p := range probs {
				if p.Path == c.path && p.Code == c.code {
					return
				}
			}
			t.Fatalf("want %s %s, got %v", c.path, c.code, probs)
		})
	}
}

func box(b string) string {
	return page(`{"id":"abcdef",` + textItem + `,` + b + `}`)
}

// Every limit at the boundary passes and one over fails.
func TestValidateBoundaries(t *testing.T) {
	ok := []struct{ name, doc string }{
		{"w 10 and 2000", box(`"x":-5,"w":10,"h":10,"cy":0`)},
		{"w 2000", box(`"x":-1000,"w":2000,"h":2000,"cy":500`)},
		{"x at the left limit", box(`"x":-200,"w":400,"h":10,"cy":0`)},
		{"x at the right limit", box(`"x":800,"w":400,"h":10,"cy":0`)},
		{"cy at the top limit", box(`"x":0,"w":10,"h":100,"cy":-70`)},
		{"cy at the bottom limit", box(`"x":0,"w":10,"h":100,"cy":1070`)},
		{"r -180 and 180", page(`{"id":"abcdef",`+textItem+`,"x":0,"w":10,"h":10,"cy":0,"r":-180}`, `{"id":"ghijkl",`+textItem+`,"x":0,"w":10,"h":10,"cy":0,"r":180}`)},
		{"text of 500 runes", page(item("abcdef", `"t":"text","text":"`+strings.Repeat("a", 500)+`"`))},
		{"500 Vietnamese runes", page(item("abcdef", `"t":"text","text":"`+strings.Repeat("ệ", 500)+`"`))},
		{"500 emoji", page(item("abcdef", `"t":"text","text":"`+strings.Repeat("😀", 500)+`"`))},
		{"text with newline", page(item("abcdef", `"t":"text","text":"a\nb"`))},
		{"script stays text", page(item("abcdef", `"t":"text","text":"<script>alert(1)</script>"`))},
		{"id of 6 and 26", page(item("abcdef", textItem), item(strings.Repeat("Z", 26), textItem))},
		{"size 8 and 96", page(item("abcdef", `"t":"text","text":"x","size":8`), item("ghijkl", `"t":"text","text":"x","size":96`))},
		{"radius 0 and 500", page(item("abcdef", `"t":"image","media":"`+mediaA+`","radius":0`), item("ghijkl", `"t":"image","media":"`+mediaA+`","radius":500`))},
	}
	for _, c := range ok {
		t.Run(c.name, func(t *testing.T) {
			if _, probs := Validate([]byte(c.doc), manifest(t), own(mediaA)); len(probs) != 0 {
				t.Fatalf("rejected: %v", probs)
			}
		})
	}
	ids := func(from, n int) []string {
		var s []string
		for i := from; i < from+n; i++ {
			s = append(s, item(fmt.Sprintf("it%04d", i), textItem))
		}
		return s
	}
	multi := func(per ...int) string { // one page kind per count
		kinds := []string{"cover", "profile", "notes", "back"}
		var parts []string
		from := 0
		for i, n := range per {
			parts = append(parts, fmt.Sprintf(`%q:{"items":[%s]}`, kinds[i], strings.Join(ids(from, n), ",")))
			from += n
		}
		return `{"v":1,"pages":{` + strings.Join(parts, ",") + `}}`
	}
	if _, probs := Validate([]byte(multi(40, 40, 40, 30)), manifest(t), own()); len(probs) != 0 {
		t.Fatalf("150 items rejected: %v", probs)
	}
	if _, probs := Validate([]byte(multi(40, 40, 40, 31)), manifest(t), own()); len(probs) != 1 || probs[0] != (Problem{"pages", CodeTooMany}) {
		t.Fatalf("151 items: %v", probs)
	}
	if _, probs := Validate([]byte(multi(41)), manifest(t), own()); len(probs) != 1 || probs[0] != (Problem{"pages.cover.items", CodeTooMany}) {
		t.Fatalf("41 items on a page: %v", probs)
	}
	if _, probs := Validate([]byte(multi(40)), manifest(t), own()); len(probs) != 0 {
		t.Fatalf("40 items rejected: %v", probs)
	}
}

func TestTextKeptAsTextAndCountedInRunes(t *testing.T) {
	doc := page(item("abcdef", `"t":"text","text":"<script>alert(1)</script> & \"q\""`))
	out, probs := Validate([]byte(doc), manifest(t), own())
	if len(probs) != 0 || !strings.Contains(string(out), `<script>alert(1)</script> & \"q\"`) {
		t.Fatalf("%v %s", probs, out)
	}
	// 501 emoji is 501 code points although a flag is more bytes than a letter; 500 combined Vietnamese letters stay 500 after NFC
	decomposed := strings.Repeat("ệ", 500) // 1500 runes before NFC, 500 after
	if _, probs := Validate([]byte(page(item("abcdef", `"t":"text","text":"`+decomposed+`"`))), manifest(t), own()); len(probs) != 0 {
		t.Fatalf("decomposed Vietnamese: %v", probs)
	}
	if _, probs := Validate([]byte(page(item("abcdef", `"t":"text","text":"`+strings.Repeat("😀", 501)+`"`))), manifest(t), own()); len(probs) == 0 {
		t.Fatal("501 emoji accepted")
	}
}

func TestProblemsAreCapped(t *testing.T) {
	var items []string
	for i := range 30 {
		items = append(items, item(fmt.Sprintf("bad%03d", i), `"t":"asset","asset":"nope"`))
	}
	_, probs := Validate([]byte(page(items...)), manifest(t), own())
	if len(probs) != MaxProblems {
		t.Fatalf("%d problems, want %d", len(probs), MaxProblems)
	}
}

func TestDeepNestingIsRefused(t *testing.T) {
	doc := `{"v":1,"pages":{"cover":{"items":` + strings.Repeat("[", 100000) + strings.Repeat("]", 100000) + `}}}`
	if out, probs := Validate([]byte(doc), manifest(t), own()); out != nil || len(probs) == 0 {
		t.Fatal("accepted")
	}
}

func TestMediaRefs(t *testing.T) {
	other := ulid.New(time.Unix(1_800_000_001, 0))
	doc := `{"v":1,"pages":{"cover":{"bg":{"media":"` + mediaA + `"},"items":[` +
		item("abcdef", `"t":"image","media":"`+other+`"`) + `,` + item("ghijkl", `"t":"image","media":"`+mediaA+`"`) + `,` +
		item("mnopqr", `"t":"image","media":"not-an-id"`) + `,` + item("stuvwx", `"t":"text","media":"`+other+`"`) + `]}}}`
	got := MediaRefs([]byte(doc))
	if len(got) != 2 || got[0] != mediaA && got[1] != mediaA {
		t.Fatalf("refs = %v", got)
	}
	if MediaRefs([]byte(`not json`)) != nil {
		t.Fatal("refs of garbage")
	}
}

func TestSanitizeDropsWhatNoLongerValidates(t *testing.T) {
	gone := ulid.New(time.Unix(1_800_000_002, 0))
	doc := `{"v":1,"pages":{
	  "poster":{"items":[]},
	  "cover":{"bg":{"asset":"shape.removed"},"overrides":{"cover.photo":{"x":10},"cover.removed":{"x":10}},
	    "items":[` + item("good01", textItem) + `,` + item("gone01", `"t":"image","media":"`+gone+`"`) + `,` +
		item("asset1", `"t":"asset","asset":"shape.removed"`) + `,` + item("good02", `"t":"image","media":"`+mediaA+`"`) + `]},
	  "back":{"bg":{"color":"ink"}}}}`
	out := Sanitize([]byte(doc), manifest(t), own(mediaA))
	var got struct {
		Pages map[string]struct {
			Bg        map[string]string
			Overrides map[string]any
			Items     []struct{ ID string }
		}
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	cover := got.Pages["cover"]
	if _, ok := got.Pages["poster"]; ok || cover.Bg != nil || len(cover.Overrides) != 1 || len(cover.Items) != 2 ||
		cover.Items[0].ID != "good01" || cover.Items[1].ID != "good02" || got.Pages["back"].Bg["color"] != "ink" {
		t.Fatalf("sanitized: %s", out)
	}
	// the result is itself a valid document
	if _, probs := Validate(out, manifest(t), own(mediaA)); len(probs) != 0 {
		t.Fatalf("sanitized document is invalid: %v", probs)
	}
	for _, bad := range []string{`not json`, `{"v":2,"pages":{}}`, `{"v":1}`, `{"pages":{}}`} {
		if Sanitize([]byte(bad), manifest(t), own()) != nil {
			t.Errorf("%s was read", bad)
		}
	}
	// the limits cut instead of failing
	var items []string
	for i := range 45 {
		items = append(items, item(fmt.Sprintf("it%04d", i), textItem))
	}
	big := Sanitize([]byte(page(items...)), manifest(t), own())
	var n struct {
		Pages struct{ Cover struct{ Items []any } }
	}
	_ = json.Unmarshal(big, &n)
	if len(n.Pages.Cover.Items) != MaxItemsPerPage {
		t.Fatalf("kept %d items", len(n.Pages.Cover.Items))
	}
}

func TestOverrideBoundsWithoutAndWithTheOtherFields(t *testing.T) {
	ov := func(body string) string {
		return `{"v":1,"pages":{"cover":{"overrides":{"cover.photo":{` + body + `}}}}}`
	}
	for body, ok := range map[string]bool{
		`"x":-1000`: true, `"x":-1001`: false, `"x":995`: true, `"x":996`: false,
		`"x":-100,"w":200`: true, `"x":-101,"w":200`: false, `"x":900,"w":200`: true, `"x":901,"w":200`: false,
		`"cy":-1419`: true, `"cy":-1420`: false, `"cy":2419`: true, `"cy":2420`: false,
		`"hidden":true`: true, ``: true,
	} {
		_, probs := Validate([]byte(ov(body)), manifest(t), own())
		if (len(probs) == 0) != ok {
			t.Errorf("override %s: ok=%v problems=%v", body, ok, probs)
		}
	}
}
