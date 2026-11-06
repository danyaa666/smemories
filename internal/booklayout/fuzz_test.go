package booklayout

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/danyaa666/smemories/internal/httpx"
)

// Fuzz targets of the layout parsers (E09 T-179). `go test` runs the seeds below and the files under testdata/fuzz (a finding is committed
// there as a regression seed); `go test -fuzz=FuzzValidate -fuzztime=5m ./internal/booklayout` explores. The properties: never panics, never
// takes a second, an accepted document is canonical (validating it again returns the same bytes), within the size and item limits, and a
// rejected one comes with at most MaxProblems problems and no document.

const fuzzMaxTime = time.Second

// hostileDocs are QA's hostile cases and the ones this task added: deep nesting, huge numbers, NaN-looking strings, lone surrogates, NUL and
// bidi characters, duplicated keys, thousands of items and a megabyte of text.
func hostileDocs() []string {
	long := strings.Repeat("a", 1<<20)
	manyItems := `{"v":1,"pages":{"cover":{"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, 10000), ",") + `]}}}`
	return []string{
		page(item("a1b2c3", textItem)),
		page(item("a1b2c3", textItem), item("d4e5f6", `"t":"asset","asset":"shape.star","tint":"accent"`), item("g7h8i9", `"t":"image","media":"`+mediaA+`","fit":"cover","radius":20`)),
		`{"v":1,"pages":{"cover":{"bg":{"asset":"shape.star"},"overrides":{"cover.photo":{"x":120,"w":500,"h":500,"cy":400,"r":0,"hidden":false},"cover.title":{"hidden":true}}},"profile":{"bg":{"color":"soft"}},"notes":{},"back":{}}}`,
		strings.Repeat("[", 100000),
		strings.Repeat("[", 20) + strings.Repeat("]", 20),
		`{"v":1,"pages":{"cover":` + strings.Repeat(`{"a":`, 17) + `1` + strings.Repeat("}", 17) + `}}`,
		manyItems,
		`{"v":1,"pages":{"cover":{"items":[` + strings.Repeat(`{"id":"a1b2c3","t":"text","text":"x"},`, 1000) + `{}]}}}`,
		page(item("a1b2c3", `"t":"text","text":"`+long+`"`)),
		page(item("a1b2c3", `"t":"text","text":"\ud800 lone"`)),
		page(item("a1b2c3", `"t":"text","text":"nul \u0000 byte"`)),
		page(item("a1b2c3", `"t":"text","text":"bidi \u202e override"`)),
		page(item("a1b2c3", `"t":"text","text":"Việt"`)),
		page(item("a1b2c3", `"t":"text","text":"क़"`)), // U+0958 grows under NFC? (it decomposes and is excluded from composition)
		page(item("a1b2c3", `"t":"text","text":"NaN"`) + `,{"id":"a1b2c3","t":"text","text":"dup"}`),
		`{"v":1,"v":2,"pages":{},"pages":{"cover":{}}}`,
		`{"v":1e999,"pages":{}}`, `{"v":NaN,"pages":{}}`, `{"v":1,"pages":{"cover":{"items":[{"id":"a1b2c3","t":"text","text":"x","x":1e400,"w":-0,"h":1.5,"cy":99999999999999999999,"r":9223372036854775808}]}}}`,
		`{"v":1,"pages":{"` + "\u202e" + `":{}}}`, `{"v":1,"pages":{"cover":{"` + strings.Repeat("k", 5000) + `":1}}}`,
		`{"v":1,"pages":{}}` + "\x00", "\xff\xfe", ``, `null`, `{}`,
	}
}

func shippedDocSeeds(t testing.TB) []string {
	var out []string
	all, _ := Shipped()
	for _, list := range all {
		for _, pt := range list {
			out = append(out, string(pageDoc(pt.Kind, pt.Page)))
		}
	}
	return out
}

func timed(t *testing.T, what string, f func()) {
	t.Helper()
	start := time.Now()
	f()
	if d := time.Since(start); d > fuzzMaxTime {
		t.Fatalf("%s took %v", what, d)
	}
}

// accepted checks what every accepted document must be.
func accepted(t *testing.T, out []byte, again func([]byte) ([]byte, int, []Problem)) {
	t.Helper()
	if len(out) == 0 || len(out) > MaxBytes {
		t.Fatalf("accepted a document of %d bytes", len(out))
	}
	if !json.Valid(out) || bytes.ContainsAny(out, "\n") {
		t.Fatalf("not compact JSON: %.200q", out)
	}
	if n := itemCount(out); n > MaxItemsTotal {
		t.Fatalf("%d items", n)
	}
	var d struct {
		Pages map[string]struct {
			Items []struct {
				Text string `json:"text"`
			} `json:"items"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatal(err)
	}
	for _, p := range d.Pages {
		if len(p.Items) > MaxItemsPerPage {
			t.Fatalf("%d items on a page", len(p.Items))
		}
		for _, it := range p.Items {
			if utf8.RuneCountInString(it.Text) > MaxTextRunes || !utf8.ValidString(it.Text) {
				t.Fatalf("text of %d characters", utf8.RuneCountInString(it.Text))
			}
		}
	}
	same, dropped, probs := again(out)
	if len(probs) != 0 || dropped != 0 || !bytes.Equal(same, out) {
		t.Fatalf("not canonical: validating again gave %v dropped=%d\n%.300s\n%.300s", probs, dropped, out, same)
	}
}

func FuzzValidate(f *testing.F) {
	for _, s := range append(hostileDocs(), shippedDocSeeds(f)...) {
		f.Add([]byte(s))
	}
	m := manifest(f)
	own := own(mediaA)
	f.Fuzz(func(t *testing.T, raw []byte) {
		var out []byte
		var probs []Problem
		timed(t, "Validate", func() { out, probs = Validate(raw, m, own) })
		if len(probs) > MaxProblems || (out == nil) == (len(probs) == 0) {
			t.Fatalf("out=%d bytes, %d problems", len(out), len(probs))
		}
		if len(probs) == 0 {
			accepted(t, out, func(b []byte) ([]byte, int, []Problem) { o, p := Validate(b, m, own); return o, 0, p })
		}
		timed(t, "Sanitize", func() { _ = Sanitize(raw, m, own) })
		timed(t, "MediaRefs", func() { _ = MediaRefs(raw) })
	})
}

func FuzzValidateRestore(f *testing.F) {
	for _, s := range append(hostileDocs(), shippedDocSeeds(f)...) {
		f.Add([]byte(s))
	}
	m := manifest(f)
	own := own(mediaA)
	f.Fuzz(func(t *testing.T, raw []byte) {
		var out []byte
		var probs []Problem
		timed(t, "ValidateRestore", func() { out, _, probs = ValidateRestore(raw, m, own) })
		if len(probs) > MaxProblems || (out == nil) == (len(probs) == 0) {
			t.Fatalf("out=%d bytes, %d problems", len(out), len(probs))
		}
		if len(probs) == 0 {
			accepted(t, out, func(b []byte) ([]byte, int, []Problem) { return ValidateRestore(b, m, own) })
			// what a restore accepts a save accepts too, once it names nothing that is gone
			if o, p := Validate(out, m, own); len(p) != 0 || !bytes.Equal(o, out) {
				if len(p) == 0 || p[0].Code != CodeUnknownID && p[0].Code != CodeNotOwnMedia {
					t.Fatalf("restore accepted what a save refuses: %v", p)
				}
			}
		}
	})
}

func FuzzStrip(f *testing.F) {
	for _, s := range []string{
		`{"bg":{"media":"` + mediaA + `"},"overrides":{"cover.photo":{"x":1}},"items":[{"id":"aaaaaa","t":"text","text":"Hi"},{"id":"bbbbbb","t":"image","media":"` + mediaA + `"}]}`,
		`{"bg":{"color":"soft"}}`, `{}`, `[]`, `null`, `{"items":null}`, `{"items":{}}`, `{"items":[1,"x",null,[],{"t":5}]}`, `{"bg":[]}`, `{"bg":{"media":1},"items":[]}`,
		`{"items":[` + strings.Repeat(`{"t":"image"},`, 10000) + `{}]}`, strings.Repeat("[", 100000),
	} {
		f.Add([]byte(s))
	}
	all, _ := Shipped()
	for _, list := range all {
		for _, pt := range list {
			f.Add([]byte(pt.Page))
		}
	}
	f.Fuzz(func(t *testing.T, page []byte) {
		var out json.RawMessage
		var n int
		var err error
		timed(t, "strip", func() { out, n, err = strip(page) })
		if err != nil {
			if out != nil || n != 0 {
				t.Fatalf("an error with a result: %q %d", out, n)
			}
			return
		}
		var p map[string]json.RawMessage
		if json.Unmarshal(out, &p) != nil || p == nil {
			t.Fatalf("not an object: %.200q", out)
		}
		again, m, err := strip(out)
		if err != nil || m != 0 || !bytes.Equal(again, out) {
			t.Fatalf("not idempotent: n=%d err=%v\n%.300s\n%.300s", m, err, out, again)
		}
		var items []struct {
			T string `json:"t"`
		}
		_ = json.Unmarshal(p["items"], &items)
		for _, it := range items {
			if it.T == "image" {
				t.Fatal("an image item is left")
			}
		}
	})
}

func FuzzWithPage(f *testing.F) {
	for _, c := range [][3]string{
		{`{"v":1,"pages":{"cover":{"items":[]},"back":{}}}`, "cover", `{"items":[{"id":"cccccc"}]}`},
		{``, "profile", `{}`},
		{`{"v":1,"pages":[]}`, "cover", `{}`},
		{`{"v":1,"pages":null}`, "cover", `{}`},
		{`null`, "cover", `{}`},
		{`{"v":1,"pages":{"cover":{}}}`, "cover", `not json`},
		{`{"v":1,"pages":{"cover":{}}}`, "", `{}`},
		{`{"pages":{"a":1,"a":2}}`, "a", `[]`},
		{strings.Repeat("[", 100000), "cover", `{}`},
		{`{"v":1,"pages":{}}`, "cover", strings.Repeat("[", 100000)},
		{`{"v":1,"pages":{}}`, "cover", `{"items":[{"text":"a < b && c > d"}]}`},
	} {
		f.Add([]byte(c[0]), c[1], []byte(c[2]))
	}
	f.Fuzz(func(t *testing.T, cur []byte, kind string, page []byte) {
		if !utf8.ValidString(kind) { // the kind is a page kind id of the manifest, never raw input
			return
		}
		if len(cur) == 0 { // the service passes nil for "no document yet", never an empty one
			cur = nil
		}
		var out []byte
		var err error
		timed(t, "withPage", func() { out, err = withPage(cur, kind, page) })
		if err != nil {
			if out != nil {
				t.Fatalf("an error with a result: %.200q", out)
			}
			return
		}
		// json.Marshal writes < > & as \u003c...: the same page, other bytes (Validate re-encodes it canonically afterwards)
		var got, want any
		same := func(a, b []byte) bool { // numbers as written (1E700 is not a float64)
			da, db := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
			da.UseNumber()
			db.UseNumber()
			return da.Decode(&got) == nil && db.Decode(&want) == nil && reflect.DeepEqual(got, want)
		}
		if !json.Valid(out) || !same(pageOf(out, kind), page) {
			t.Fatalf("pages.%q is not the page given:\n%.200q\n%.200q", kind, pageOf(out, kind), page)
		}
	})
}

// FuzzRequestDecode is the decode path of the write routes: the body cap, httpx.DecodeJSON into the route's input, then the layout of a save
// through Validate. A refusal is always 400 or 413 and the handler never runs on a body it refused.
func FuzzRequestDecode(f *testing.F) {
	for _, s := range []string{
		`{"yearbook_id":"01K7QB1B2C3D4E5F6G7H8J9K0M","base_version":0,"layout":` + page(item("a1b2c3", textItem)) + `}`,
		`{"yearbook_id":"x","base_version":-1,"layout":null}`, `{"layout":` + strings.Repeat("[", 100000), `{"layout":` + strings.Repeat("[", 20) + strings.Repeat("]", 20) + `}`,
		`{"yearbook_id":"a","yearbook_id":"b","base_version":1e999}`, `{"kind":"cover","source":"own","template_id":"x","base_version":0,"yearbook_id":"y"}`,
		`{"yearbook_id":"y","kind":"cover","name":"` + "\u202e" + `"}`, `{"unknown":1}`, `{} {}`, `[]`, `null`, ``, `{"name":"\ud800"}`,
	} {
		f.Add([]byte(s), uint8(0))
	}
	m := manifest(f)
	f.Fuzz(func(t *testing.T, body []byte, route uint8) {
		var in any
		switch route % 3 {
		case 0:
			in = &saveInput{}
		case 1:
			in = &applyInput{}
		default:
			in = &templateCreateInput{}
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		var ok bool
		timed(t, "decode", func() { ok = httpx.DecodeJSON(w, r, in) })
		if !ok {
			if w.Code != http.StatusBadRequest && w.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("refused with %d", w.Code)
			}
			return
		}
		if s, isSave := in.(*saveInput); isSave && len(s.Layout) > 0 && len(s.Layout) <= MaxBytes {
			timed(t, "Validate", func() { _, _ = Validate(s.Layout, m, nil) })
		}
	})
}
