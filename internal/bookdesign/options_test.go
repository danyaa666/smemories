package bookdesign

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/templates"
)

func memory(t testing.TB) *templates.DesignBlock {
	t.Helper()
	d, ok := templates.Design("memory-book")
	if !ok {
		t.Fatal("memory-book has no design block")
	}
	return d
}

// Every rejection of requirement 3 and the fuzz cases of AC3: nothing is accepted, and the error is ERROR_INVALID_DESIGN naming the field.
func TestValidateRejects(t *testing.T) {
	d := memory(t)
	runes := func(n int) string { return strings.Repeat("ê", n) }
	cases := []struct{ name, doc, field string }{
		{"not an object: array", `[]`, "JSON object"},
		{"not an object: null", `null`, "JSON object"},
		{"not an object: string", `"x"`, "JSON object"},
		{"empty body", ``, "JSON object"},
		{"broken json", `{"theme":`, "not valid JSON"},
		{"data after the object", `{} {}`, "after the object"},
		{"unknown top-level key", `{"colors":{}}`, "unknown key"},
		{"unknown key in sections", `{"sections":{"x":1}}`, "unknown key"},
		{"version 2", `{"v":2}`, "v must be 1"},
		{"version 0", `{"v":0}`, "v must be 1"},
		{"version as string", `{"v":"1"}`, "v:"},
		{"version as float", `{"v":1.5}`, "v:"},
		{"theme unknown", `{"theme":"neon"}`, "theme: unknown id"},
		{"theme wrong type", `{"theme":3}`, "theme:"},
		{"theme case differs", `{"theme":"Lavender"}`, "theme: unknown id"},
		{"fonts unknown", `{"fonts":"comic"}`, "fonts: unknown id"},
		{"fonts list", `{"fonts":["playful"]}`, "fonts:"},
		{"order missing a section", `{"sections":{"order":["cover","profile","notes"]}}`, "sections.order"},
		{"order with an extra section", `{"sections":{"order":["cover","profile","notes","back","back"]}}`, "sections.order"},
		{"order duplicate", `{"sections":{"order":["cover","profile","profile","back"]}}`, "listed twice"},
		{"order unknown id", `{"sections":{"order":["cover","profile","notes","zzz"]}}`, "unknown id"},
		{"order empty", `{"sections":{"order":[]}}`, "sections.order"},
		{"order wrong type", `{"sections":{"order":"cover"}}`, "sections.order:"},
		{"fixed first moved", `{"sections":{"order":["profile","cover","notes","back"]}}`, "fixed section cover"},
		{"fixed last moved", `{"sections":{"order":["cover","profile","back","notes"]}}`, "fixed section back"},
		{"hidden non-hideable cover", `{"sections":{"hidden":["cover"]}}`, "cover cannot be hidden"},
		{"hidden non-hideable notes", `{"sections":{"hidden":["notes"]}}`, "notes cannot be hidden"},
		{"hidden unknown", `{"sections":{"hidden":["zzz"]}}`, "sections.hidden: unknown id"},
		{"hidden duplicate", `{"sections":{"hidden":["profile","profile"]}}`, "listed twice"},
		{"hidden wrong type", `{"sections":{"hidden":{"profile":true}}}`, "sections.hidden:"},
		{"text unknown id", `{"texts":{"cover.nope":"x"}}`, "texts: unknown id"},
		{"text wrong type", `{"texts":{"cover.belongs":5}}`, "texts:"},
		{"text is a list", `{"texts":["x"]}`, "texts:"},
		{"text too long by one", `{"texts":{"cover.belongs":"` + runes(41) + `"}}`, "at most 40"},
		{"text 5000 runes", `{"texts":{"cover.belongs":"` + runes(5000) + `"}}`, "larger than 4096"},
		{"text newline", `{"texts":{"cover.belongs":"a\nb"}}`, "no control characters"},
		{"text tab", `{"texts":{"cover.belongs":"a\tb"}}`, "no control characters"},
		{"text NUL", `{"texts":{"cover.belongs":"a\u0000b"}}`, "no control characters"},
		{"text bidi override", `{"texts":{"cover.belongs":"a\u202eb"}}`, "no control characters"},
		{"text line separator", `{"texts":{"cover.belongs":"a\u2028b"}}`, "no control characters"},
		{"text 41 runes after trim", `{"texts":{"cover.belongs":"  ` + runes(41) + `  "}}`, "at most 40"},
		{"image unknown id", `{"images":{"cover.nope":"dots"}}`, "images: unknown id"},
		{"image unknown choice", `{"images":{"cover.pattern":"waves"}}`, "unknown choice"},
		{"image wrong type", `{"images":{"cover.pattern":["dots"]}}`, "images:"},
		{"over 4096 bytes", `{"texts":{"cover.belongs":"a"},"pad":"` + strings.Repeat("x", 4100) + `"}`, "larger than 4096"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := Validate(d, []byte(c.doc))
			if err == nil {
				t.Fatalf("accepted %q as %s", c.doc, out)
			}
			if got := apperr.CodeOf(err); got != CodeInvalidDesign || got.HTTP != 400 {
				t.Errorf("code %v, want ERROR_INVALID_DESIGN 400", got)
			}
			if !strings.Contains(err.Error(), c.field) {
				t.Errorf("error %q does not name %q", err, c.field)
			}
		})
	}
}

// The largest legal documents fit 4096 bytes, and a document that only just fits in raw form but not once normalised is refused.
func TestValidateAccepts(t *testing.T) {
	d := memory(t)
	cases := []struct{ name, doc, want string }{
		{"empty object", `{}`, `{"v":1}`},
		{"version only", `{"v":1}`, `{"v":1}`},
		{"null keys are absent", `{"theme":null,"texts":null,"sections":null}`, `{"v":1}`},
		{"full document", `{"v":1,"theme":"peach","fonts":"classic","sections":{"order":["cover","notes","profile","back"],"hidden":["profile","back"]},"texts":{"profile.heading":"Về mình"},"images":{"cover.pattern":"plain"}}`,
			`{"v":1,"theme":"peach","fonts":"classic","sections":{"order":["cover","notes","profile","back"],"hidden":["profile","back"]},"texts":{"profile.heading":"Về mình"},"images":{"cover.pattern":"plain"}}`},
		{"default order and nothing hidden", `{"sections":{"order":["cover","profile","notes","back"],"hidden":[]}}`, `{"v":1,"sections":{"order":["cover","profile","notes","back"]}}`},
		{"empty text means the default", `{"texts":{"cover.belongs":"   "}}`, `{"v":1}`},
		{"text is trimmed and made NFC", `{"texts":{"cover.belongs":"  Ví dụ  "}}`, `{"v":1,"texts":{"cover.belongs":"Ví dụ"}}`}, // "u" + combining marks in the input
		{"exactly max runes", `{"texts":{"cover.belongs":"` + strings.Repeat("ê", 40) + `"}}`, `{"v":1,"texts":{"cover.belongs":"` + strings.Repeat("ê", 40) + `"}}`},
		{"HTML is plain text", `{"texts":{"cover.belongs":"<b>hi</b>"}}`, `{"v":1,"texts":{"cover.belongs":"<b>hi</b>"}}`},
	}
	// The decomposed "Ví dụ" (V i + U+0301, d u + U+0323 ...) for the NFC case.
	cases[6].doc = "{\"texts\":{\"cover.belongs\":\"  Ví dụ  \"}}"
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := Validate(d, []byte(c.doc))
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != c.want {
				t.Errorf("stored %s\nwant   %s", out, c.want)
			}
		})
	}
}

// All eight texts at their longest, in Vietnamese (3 bytes per character), would not fit 4096 bytes for a template with more room;
// the stored size check is on the document that is saved.
func TestValidateStoredSizeLimit(t *testing.T) {
	d := &templates.DesignBlock{Themes: []string{"a"}, FontSets: []string{"f"}}
	for i := range 16 {
		d.Texts = append(d.Texts, templates.DesignText{ID: fmt.Sprintf("t%d", i), Max: 200})
	}
	texts := map[string]string{}
	for _, tx := range d.Texts {
		texts[tx.ID] = strings.Repeat("ệ", 90) // 90 runes x 3 bytes: 270 bytes each, 16 of them are over 4096 once encoded
	}
	raw, _ := json.Marshal(map[string]any{"texts": texts})
	if len(raw) <= MaxOptionsBytes {
		t.Fatalf("test document is only %d bytes", len(raw))
	}
	if _, err := Validate(d, raw); apperr.CodeOf(err) != CodeInvalidDesign {
		t.Errorf("err = %v, want ERROR_INVALID_DESIGN", err)
	}
	short := map[string]string{"t0": strings.Repeat("ệ", 90), "t1": strings.Repeat("ệ", 90)}
	raw, _ = json.Marshal(map[string]any{"texts": short})
	if out, err := Validate(d, raw); err != nil || len(out) > MaxOptionsBytes {
		t.Errorf("two long texts: %v", err)
	}
}

func TestResolveDefaultsAndStaleIDs(t *testing.T) {
	d := memory(t)
	defaults := &Resolved{
		Theme: "lavender", Fonts: "playful",
		Sections: ResolvedSections{Order: []string{"cover", "profile", "notes", "back"}, Hidden: []string{}},
		Texts:    map[string]string{}, Images: map[string]string{"cover.pattern": "dots"},
	}
	cases := []struct {
		name   string
		stored string
		want   *Resolved
	}{
		{"no row", "", defaults},
		{"empty document", `{"v":1}`, defaults},
		{"unreadable document", `{"theme":`, defaults},
		{"theme the manifest no longer has", `{"v":1,"theme":"retired","fonts":"classic"}`,
			&Resolved{Theme: "lavender", Fonts: "classic", Sections: defaults.Sections, Texts: map[string]string{}, Images: defaults.Images}},
		{"fonts the manifest no longer has", `{"v":1,"fonts":"retired","theme":"mint"}`,
			&Resolved{Theme: "mint", Fonts: "playful", Sections: defaults.Sections, Texts: map[string]string{}, Images: defaults.Images}},
		{"stale section, text, image and choice ids", `{"v":1,"sections":{"order":["gone","notes","profile"],"hidden":["gone","profile","cover"]},"texts":{"gone":"x","back.pact":"Hẹn"},"images":{"gone":"x","cover.pattern":"retired"}}`,
			&Resolved{Theme: "lavender", Fonts: "playful", Sections: ResolvedSections{Order: []string{"cover", "notes", "profile", "back"}, Hidden: []string{"profile"}},
				Texts: map[string]string{"back.pact": "Hẹn"}, Images: map[string]string{"cover.pattern": "dots"}}},
		{"fixed sections stay put even in a stored order that moved them", `{"v":1,"sections":{"order":["back","notes","profile","cover"]}}`,
			&Resolved{Theme: "lavender", Fonts: "playful", Sections: ResolvedSections{Order: []string{"cover", "notes", "profile", "back"}, Hidden: []string{}}, Texts: map[string]string{}, Images: defaults.Images}},
		{"a text longer than the manifest now allows is dropped", `{"v":1,"texts":{"back.pact":"` + strings.Repeat("a", 19) + `"}}`, defaults},
		{"a control character in a stored text is dropped", "{\"v\":1,\"texts\":{\"back.pact\":\"a\\nb\"}}", defaults},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stored []byte
			if c.stored != "" {
				stored = []byte(c.stored)
			}
			if got := Resolve(d, stored); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

// Whatever Validate accepts resolves to itself (nothing is lost), and neither function panics on any input.
func FuzzValidateResolve(f *testing.F) {
	d, _ := templates.Design("memory-book")
	for _, s := range []string{``, `{}`, `[]`, `{"v":1,"theme":"peach"}`, `{"sections":{"order":["cover","notes","profile","back"],"hidden":["back"]}}`,
		`{"texts":{"back.pact":"x"},"images":{"cover.pattern":"plain"}}`, `{"texts":{"cover.belongs":"\u0000"}}`, `{"sections":null}`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		out, err := Validate(d, raw)
		if err != nil {
			if apperr.CodeOf(err) != CodeInvalidDesign {
				t.Fatalf("code %v for %q", apperr.CodeOf(err), raw)
			}
			return
		}
		if len(out) > MaxOptionsBytes {
			t.Fatalf("stored %d bytes", len(out))
		}
		again, err := Validate(d, out)
		if err != nil || string(again) != string(out) {
			t.Fatalf("stored document does not validate to itself: %s -> %s, %v", out, again, err)
		}
		r := Resolve(d, out)
		if len(r.Sections.Order) != len(d.Sections) {
			t.Fatalf("resolved order %v", r.Sections.Order)
		}
	})
}
