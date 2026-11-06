package booklayout

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// E09 T-179: hostile documents fail fast, with bounded work and the right code, and never reach a map. (The same inputs through the
// real route are in TestHostileBodiesFailFast.)

type measured struct {
	took  time.Duration
	alloc uint64
}

func measure(f func()) measured {
	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	start := time.Now()
	f()
	d := time.Since(start)
	runtime.ReadMemStats(&b)
	return measured{d, b.TotalAlloc - a.TotalAlloc}
}

func TestHostileDocumentsFailFast(t *testing.T) {
	const (
		maxTime  = 200 * time.Millisecond
		maxAlloc = 4 << 20 // 4 MiB: a refusal before decoding reads the bytes once and allocates a problem list
		// A document inside the limits is decoded before its item count is known: 43 000 empty items (about 6 MB, 45 times the input) is the
		// worst a 128 KiB body can cost. 16 MiB is the ceiling; the body cap is what keeps it from growing.
		maxDecodeAlloc = 16 << 20
	)
	m, own := manifest(t), own(mediaA)
	cases := []struct {
		name, doc, code string
	}{
		{"100 000 levels, not closed", strings.Repeat("[", 100000), CodeTooDeep},
		{"100 000 levels, closed", strings.Repeat("[", 100000) + strings.Repeat("]", 100000), CodeTooLong}, // 200 KB: the size comes first
		{"60 000 levels, closed (120 KB)", strings.Repeat("[", 60000) + strings.Repeat("]", 60000), CodeTooDeep},
		{"17 levels under pages.cover", `{"v":1,"pages":{"cover":` + strings.Repeat(`{"a":`, 15) + `1` + strings.Repeat("}", 15) + `}}`, CodeTooDeep},
		{"a 2 MB string", `{"v":1,"pages":{"cover":{"items":[{"id":"a1b2c3","t":"text","text":"` + strings.Repeat("a", 2<<20) + `"}]}}}`, CodeTooLong},
		{"a 2 MB document of blanks", `{"v":1,"pages":{}}` + strings.Repeat(" ", 2<<20), CodeTooLong},
		{"200 000 tiny items", `{"v":1,"pages":{"cover":{"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, 200000), ",") + `]}}}`, CodeTooLong},
		{"43 000 tiny items (the most 128 KiB holds)", `{"v":1,"pages":{"cover":{"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, 43000), ",") + `]}}}`, CodeTooMany},
	}
	for _, c := range cases {
		if len(c.doc) > MaxBytes+4<<10 && c.code != CodeTooLong {
			t.Fatalf("%s: the case is over the route's cap and would not reach Validate", c.name)
		}
		var probs []Problem
		raw := []byte(c.doc)
		got := measure(func() { _, probs = Validate(raw, m, own) })
		if len(probs) == 0 || probs[0].Code != c.code || len(probs) > MaxProblems {
			t.Errorf("%s: problems %v, want first code %s", c.name, probs, c.code)
		}
		ceiling := uint64(maxAlloc)
		if probs[0].Code == CodeTooMany {
			ceiling = maxDecodeAlloc
		}
		if got.took > maxTime || got.alloc > ceiling {
			t.Errorf("%s: took %v and allocated %d bytes (ceilings %v and %d)", c.name, got.took, got.alloc, maxTime, ceiling)
		}
		t.Logf("%-45s %8v %9d bytes allocated  %s", c.name, got.took.Round(time.Microsecond), got.alloc, probs[0].Code)
		if _, _, probs := ValidateRestore([]byte(c.doc), m, own); len(probs) == 0 {
			t.Errorf("%s: ValidateRestore accepted it", c.name)
		}
		if out := Sanitize([]byte(c.doc), m, own); len(out) > MaxBytes {
			t.Errorf("%s: Sanitize returned %d bytes", c.name, len(out))
		}
	}
}

// NFC can make a text longer in bytes than it was sent (U+0958 is 3 bytes and becomes 6): a document that fits 128 KiB on the wire but
// not once stored is refused, so the stored size limit holds for every accepted document.
func TestStoredSizeLimitHoldsAfterNormalisation(t *testing.T) {
	text := strings.Repeat("\u0958", 240) // 720 bytes sent, 1440 stored (480 characters: inside the 500 limit)
	var pages []string
	for pi, kind := range []string{"cover", "profile", "notes"} {
		var items []string
		for i := range 40 {
			items = append(items, item(fmt.Sprintf("p%d%04d", pi, i), `"t":"text","text":"`+text+`"`))
		}
		pages = append(pages, `"`+kind+`":{"items":[`+strings.Join(items, ",")+`]}`)
	}
	doc := `{"v":1,"pages":{` + strings.Join(pages, ",") + `}}`
	if len(doc) > MaxBytes {
		t.Fatalf("the document is %d bytes: it must fit the wire limit for the test to mean anything", len(doc))
	}
	out, probs := Validate([]byte(doc), manifest(t), nil)
	if out != nil || len(probs) != 1 || probs[0].Code != CodeTooLong || probs[0].Path != "" {
		t.Fatalf("out=%d bytes probs=%v", len(out), probs)
	}
}

// Text items and names: control characters and bidi overrides are refused, a lone surrogate (which JSON decodes to U+FFFD) too; the
// Vietnamese composed and decomposed forms are stored as one text and counted in stored characters, so the 500 limit is the same for both.
func TestLayoutTextSafety(t *testing.T) {
	m := manifest(t)
	textDoc := func(jsonText string) string { return page(item("a1b2c3", `"t":"text","text":"`+jsonText+`"`)) }
	for name, bad := range map[string]string{
		"NUL": `a\u0000b`, "bidi override": `a\u202eb`, "bidi isolate": `a\u2066b\u2069`, "zero width space": `a\u200bb`, "BOM": `\ufeffa`, "line separator": `a\u2028b`,
		"escape": `a\u001bb`, "tab": `a\tb`, "lone carriage return": `a\rb`, "lone high surrogate": `a\ud800b`, "lone low surrogate": `a\udc00b`,
	} {
		out, probs := Validate([]byte(textDoc(bad)), m, nil)
		if len(probs) != 1 || probs[0].Code != CodeBadText || probs[0].Path != "pages.cover.items[0].text" {
			t.Errorf("%s: out=%q probs=%v", name, out, probs)
		}
	}
	// an empty text is a cleared box, not an error
	if _, probs := Validate([]byte(textDoc(`   `)), m, nil); len(probs) != 0 {
		t.Errorf("blank text: %v", probs)
	}
	// a newline is allowed in a text item, "\r\n" becomes "\n"
	out, probs := Validate([]byte(textDoc(`a\r\nb\nc`)), m, nil)
	if len(probs) != 0 || !strings.Contains(string(out), `"text":"a\nb\nc"`) {
		t.Errorf("newlines: %s %v", out, probs)
	}
	composed := "Tiếng Việt, ệ ữ ặ"
	decomposed := "Tiếng Việt, ệ ữ ặ"
	a, pa := Validate([]byte(textDoc(composed)), m, nil)
	b, pb := Validate([]byte(textDoc(decomposed)), m, nil)
	if len(pa)+len(pb) != 0 || string(a) != string(b) || !strings.Contains(string(a), composed) {
		t.Fatalf("composed %s %v, decomposed %s %v", a, pa, b, pb)
	}
	for _, n := range []int{MaxTextRunes, MaxTextRunes + 1} {
		for _, form := range []string{strings.Repeat("ệ", n), strings.Repeat("ệ", n), strings.Repeat("ệ", n)} {
			stored, probs := Validate([]byte(textDoc(form)), m, nil)
			if want := n == MaxTextRunes; (len(probs) == 0) != want {
				t.Errorf("%d letters: accepted=%v probs=%v", n, len(probs) == 0, probs)
			}
			if len(probs) == 0 && !strings.Contains(string(stored), strings.Repeat("ệ", n)) {
				t.Errorf("the stored form is not the composed one")
			}
		}
	}
	if utf8.RuneCountInString(decomposed) == utf8.RuneCountInString(composed) {
		t.Fatal("the decomposed sample is not longer: the test is not testing anything")
	}
}

// A problem path names the members the sender got wrong, but only short plain names: anything else (control or bidi characters, a long key) is "?".
func TestProblemPathsDoNotEchoSenderText(t *testing.T) {
	m := manifest(t)
	sentinel := "SENTINEL\u202e" + strings.Repeat("k", 1000)
	doc := `{"v":1,"` + sentinel + `":1,"pages":{"` + sentinel + `":{},"cover":{"` + sentinel + `":1,"overrides":{"` + sentinel + `":{}},"items":[{"id":"a1b2c3","t":"text","text":"` + sentinel + `\u0000","` + sentinel + `":1}]}}}`
	_, probs := Validate([]byte(doc), m, nil)
	if len(probs) == 0 {
		t.Fatal("accepted")
	}
	for _, p := range probs {
		if strings.Contains(p.Path, "SENTINEL") || strings.Contains(p.Path, "\u202e") || len(p.Path) > 60 {
			t.Errorf("a path echoes the sender: %q (%s)", p.Path, p.Code)
		}
	}
	if _, probs := Validate([]byte(`{"v":1,"pages":{"cover":{"items":[{"id":"a1b2c3","t":"text","text":"x","bogus":1}]}}}`), m, nil); len(probs) == 0 || probs[0].Path != "pages.cover.items[0].bogus" || probs[0].Code != CodeUnknownKey {
		t.Errorf("a plain unknown key is still named: %v", probs)
	}
}
