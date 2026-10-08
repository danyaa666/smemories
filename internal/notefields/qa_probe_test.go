package notefields

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestQAProbes(t *testing.T) {
	refs := []FieldRef{{"name", true}, {"nickname", false}, {"message", true}, {"wish", false}}
	ok := func(name string, a map[string]string) map[string]string {
		t.Helper()
		got, err := Validate(refs, a)
		if err != nil {
			t.Errorf("%s: unexpected %v", name, err)
		}
		return got
	}
	bad := func(name string, a map[string]string, want string) {
		t.Helper()
		_, err := Validate(refs, a)
		if err == nil || code(err) != want {
			t.Errorf("%s: err=%v want %s", name, err, want)
		}
	}
	base := func(k, v string) map[string]string { return map[string]string{"name": "a", "message": "m", k: v} }

	// decomposed Vietnamese: e + U+0302 + U+0323 (5 bytes) == U+1EC7 after NFC
	nfd := "Vie\u0302\u0323t"
	if got := ok("nfd", map[string]string{"name": nfd, "message": "m"}); got["name"] != "Vi\u1ec7t" {
		t.Errorf("nfd not composed: %q", got["name"])
	}
	// 60 decomposed e-circumflex-dot-below = 300 bytes: valid after NFC (60 chars) but > 4*60 bytes
	_, err := Validate(refs, map[string]string{"name": strings.Repeat("e\u0302\u0323", 60), "message": "m"})
	t.Logf("60 x NFD(\u1ec7) name (300 bytes): err=%v", err)
	// 48 of them = 240 bytes, boundary of the byte guard
	if _, err := Validate(refs, map[string]string{"name": strings.Repeat("e\u0302\u0323", 48), "message": "m"}); err != nil {
		t.Errorf("48 x NFD: %v", err)
	}
	// decomposed text in 2000-char message at 1500 chars (7500 bytes)
	ok("nfd long", base("message", strings.Repeat("e\u0302\u0323", 1500)))
	// 61 composed after NFC from decomposed -> invalid
	bad("nfd 61", map[string]string{"name": strings.Repeat("e\u0302", 61), "message": "m"}, "invalid_answer:name")
	// 60 chars exactly at limit decomposed with 2-byte marks only (e+U+0301 = 3 bytes)
	ok("nfd 60 acute", map[string]string{"name": strings.Repeat("e\u0301", 60), "message": "m"})

	// bidi & invisible
	for _, r := range []string{"\u202e", "\u202d", "\u202a", "\u202c", "\u2066", "\u2067", "\u2068", "\u2069", "\u200e", "\u200f", "\u061c", "\u200b", "\u200c", "\u2060", "\xef\xbb\xbf", "\u00ad", "\u180e", "\u2028", "\u2029", "\u0085", "\x7f", "\x1b", "\u2062", "\U000E0041"} {
		bad("name "+r, base("name", "a"+r+"b"), "invalid_answer:name")
		bad("msg "+r, base("message", "a"+r+"b"), "invalid_answer:message")
		if r != "\u2028" && r != "\u2029" && r != "\u0085" { // Unicode whitespace at the edge is trimmed, not rejected
			bad("edge "+r, base("nickname", "x"+r), "invalid_answer:nickname")
		}
	}
	// allowed: ZWJ, VS16, VS15, NBSP inside, combining marks
	for _, v := range []string{"\U0001F469\u200d\U0001F393", "\u2764\ufe0f", "\u2764\ufe0e", "a\u00a0b", "\u0e01\u0e34", "\U0001F468\u200d\U0001F469\u200d\U0001F467\u200d\U0001F466", "\U0001F44D\U0001F3FD"} {
		if got := ok("allowed "+v, base("name", v)); got["name"] != v {
			t.Errorf("changed %q -> %q", v, got["name"])
		}
	}
	// whitespace only
	for _, v := range []string{"", " ", "\n", "\t", "\u00a0", "\u3000", " \r\n \n"} {
		if v == "\t" {
			continue // TrimSpace trims a lone tab, so it is "missing"
		}
		_, err := Validate(refs, map[string]string{"name": v, "message": "m"})
		if err == nil || code(err) != "missing_answer:name" {
			t.Errorf("required ws %q: %v", v, err)
		}
		if got := ok("opt ws", base("nickname", v)); len(got) != 2 {
			t.Errorf("optional ws %q kept: %v", v, got)
		}
	}
	// trailing newline in short text trimmed (observation)
	t.Logf("short 'a\\n' -> %v", func() any { g, e := Validate(refs, base("name", "a\n")); return []any{g, e} }())
	// inner newline in short text
	bad("short inner nl", base("name", "a\nb"), "invalid_answer:name")
	bad("short crlf", base("name", "a\r\nb"), "invalid_answer:name")
	// long text keeps blank lines and counts \n as char
	if got := ok("nl", base("message", "a\n\n\nb")); got["message"] != "a\n\n\nb" {
		t.Errorf("nl: %q", got["message"])
	}
	// CRLF counts as one char: 2000 chars made of "x\r\n" pieces
	ok("crlf limit", base("message", strings.Repeat("x", 1000)+strings.Repeat("\r\n", 1000)))
	bad("limit 501 wish", base("wish", strings.Repeat("a", 501)), "invalid_answer:wish")
	ok("limit 500 wish", base("wish", strings.Repeat("a", 500)))
	// limit is after trim: 500 chars + padding spaces up to 4*500 bytes ok
	ok("padded", base("wish", "  "+strings.Repeat("a", 500)+"  "))
	// 10 MB
	big := strings.Repeat("a", 10<<20)
	st := time.Now()
	for _, k := range []string{"name", "message", "wish"} {
		bad("10MB "+k, base(k, big), "invalid_answer:"+k)
	}
	bigUni := strings.Repeat("e\u0302\u0323", 2<<20)
	bad("10MB nfd", base("message", bigUni), "invalid_answer:message")
	bad("10MB ctrl", base("message", strings.Repeat("\x00", 10<<20)), "invalid_answer:message")
	if d := time.Since(st); d > time.Second {
		t.Errorf("10MB probes took %v", d)
	}
	// many answers keys: unknown id with huge value rejected quickly, unknown ids sorted
	_, err = Validate(refs, map[string]string{"zzz": big, "aaa": "1", "name": "a", "message": "m"})
	if code(err) != "unknown_field:aaa" {
		t.Errorf("unknown order: %v", err)
	}
	// invalid utf-8 fragments
	for _, v := range []string{"\xff", "\xc0\x80", "\xed\xa0\x80", "a\xe2\x82"} {
		bad("utf8 "+v, base("name", v), "invalid_answer:name")
	}
	// U+FFFD literal (valid char sent by client)
	t.Logf("literal U+FFFD in name: %v", func() any { _, e := Validate(refs, base("name", "a\ufffdb")); return e }())

	// refs edge cases
	if _, err := Validate(nil, nil); err != nil {
		t.Errorf("nil refs nil answers: %v", err)
	}
	if got, err := Validate(nil, nil); err != nil || got == nil {
		t.Errorf("nil refs returns %v %v (nil map?)", got, err)
	}
	_, err = Validate([]FieldRef{{"name", true}, {"name", true}}, map[string]string{"name": "a"})
	var fe *FieldError
	t.Logf("dup refs: %v asFieldError=%v", err, errors.As(err, &fe))
	if err == nil {
		t.Error("dup refs accepted")
	}
	_, err = Validate([]FieldRef{{"", true}}, nil)
	if !errors.Is(err, ErrUnknownField) {
		t.Errorf("empty id: %v", err)
	}
	_, err = Validate([]FieldRef{{"Name", true}}, nil)
	if !errors.Is(err, ErrUnknownField) {
		t.Errorf("case: %v", err)
	}
	// answers nil with all optional
	if got, err := Validate([]FieldRef{{"wish", false}}, nil); err != nil || len(got) != 0 {
		t.Errorf("all optional nil: %v %v", got, err)
	}
	// input map not mutated
	in := map[string]string{"name": "  a  ", "message": "m"}
	ok("mut", in)
	if in["name"] != "  a  " {
		t.Error("input mutated")
	}
	// Info does not alias catalogue: mutate result
	i1, _ := Info(Default())
	i1[0].ID = "x"
	i2, _ := Info(Default())
	if i2[0].ID != "name" {
		t.Error("Info aliases catalogue")
	}
	d := Default()
	d[0].ID = "x"
	if Default()[0].ID != "name" {
		t.Error("Default aliases")
	}
	// Info empty refs -> [] not null
	if i, err := Info(nil); err != nil || i == nil {
		t.Errorf("Info(nil)=%v %v", i, err)
	}
}
