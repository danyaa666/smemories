package yearbook

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func code(err error) string {
	var ve ValidationError
	if errors.As(err, &ve) {
		return ve.Code
	}
	return ""
}

func parseBook(t *testing.T, js string) bookInput {
	t.Helper()
	var in bookInput
	if err := json.Unmarshal([]byte(js), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestBookBoundaries(t *testing.T) {
	rep := func(n int) string { return strings.Repeat("Đ", n) }
	tests := []struct {
		field, value string
		max          int
		min          int
	}{
		{"title", "title", maxTitle, 1},
		{"school_name", "school_name", maxSchool, 0},
		{"class_name", "class_name", maxClass, 0},
		{"motto", "motto", maxMotto, 0},
	}
	for _, tc := range tests {
		mk := func(v string) bookInput {
			b, _ := json.Marshal(map[string]string{tc.field: v})
			return parseBook(t, string(b))
		}
		if err := mk(rep(tc.max)).applyTo(&Yearbook{}); err != nil {
			t.Errorf("%s at max: %v", tc.field, err)
		}
		if got := code(mk(rep(tc.max + 1)).applyTo(&Yearbook{})); got != "invalid_"+tc.field {
			t.Errorf("%s over max: %q", tc.field, got)
		}
		if got := code(mk("a\x07b").applyTo(&Yearbook{})); got != "invalid_"+tc.field {
			t.Errorf("%s control char: %q", tc.field, got)
		}
		if err := mk("").applyTo(&Yearbook{}); (tc.min == 1) == (err == nil) {
			t.Errorf("%s empty: err=%v, min=%d", tc.field, err, tc.min)
		}
	}
}

func TestBookEnumsAndYear(t *testing.T) {
	bad := []struct{ js, want string }{
		{`{"language":"fr"}`, "invalid_language"},
		{`{"language":""}`, "invalid_language"},
		{`{"page_size":"A3"}`, "invalid_page_size"},
		{`{"page_size":"letter"}`, "invalid_page_size"},
		{`{"page_size":"LETTER"}`, "invalid_page_size"},
		{`{"page_size":"Legal"}`, "invalid_page_size"},
		{`{"graduation_year":1949}`, "invalid_graduation_year"},
		{`{"graduation_year":2101}`, "invalid_graduation_year"},
	}
	for _, tc := range bad {
		if got := code(parseBook(t, tc.js).applyTo(&Yearbook{})); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.js, got, tc.want)
		}
	}
	for _, js := range []string{`{"graduation_year":1950}`, `{"graduation_year":2100}`, `{"language":"vi","page_size":"A4"}`, `{"page_size":"Letter"}`} {
		if err := parseBook(t, js).applyTo(&Yearbook{}); err != nil {
			t.Errorf("%s: %v", js, err)
		}
	}
}

func TestBookPatchSemantics(t *testing.T) {
	g := 2020
	y := Yearbook{Title: "Keep", Motto: "Old", GraduationYear: &g}
	// absent fields untouched; an explicit "" clears an optional text field; null clears the year
	if err := parseBook(t, `{"motto":"","graduation_year":null}`).applyTo(&y); err != nil {
		t.Fatal(err)
	}
	if y.Title != "Keep" || y.Motto != "" || y.GraduationYear != nil {
		t.Fatalf("unexpected %+v", y)
	}
	// null on a text field counts as absent
	if err := parseBook(t, `{"title":null}`).applyTo(&y); err != nil || y.Title != "Keep" {
		t.Fatalf("title null: %v %q", err, y.Title)
	}
	// NFC and trim
	if err := parseBook(t, `{"title":"  Việt "}`).applyTo(&y); err != nil || y.Title != "Việt" {
		t.Fatalf("nfc: %v %q", err, y.Title)
	}
}

func TestProfileValidation(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	str := func(s string) *string { return &s }
	tests := []struct {
		name string
		in   profileInput
		want string
	}{
		{"ok minimal", profileInput{FullName: "Lan"}, ""},
		{"ok yesterday", profileInput{FullName: "Lan", Birthday: str("2026-10-06")}, ""},
		{"ok leap day", profileInput{FullName: "Lan", Birthday: str("2000-02-29")}, ""},
		{"empty name", profileInput{FullName: "  "}, "invalid_full_name"},
		{"long name", profileInput{FullName: strings.Repeat("a", 101)}, "invalid_full_name"},
		{"long nickname", profileInput{FullName: "a", Nickname: strings.Repeat("a", 51)}, "invalid_nickname"},
		{"long quote", profileInput{FullName: "a", Quote: strings.Repeat("a", 501)}, "invalid_quote"},
		{"max quote", profileInput{FullName: "a", Quote: strings.Repeat("ế", 500)}, ""},
		{"long hobbies", profileInput{FullName: "a", Hobbies: strings.Repeat("a", 301)}, "invalid_hobbies"},
		{"long plans", profileInput{FullName: "a", FuturePlans: strings.Repeat("a", 301)}, "invalid_future_plans"},
		{"newline in quote", profileInput{FullName: "a", Quote: "a\nb"}, "invalid_quote"},
		{"today", profileInput{FullName: "a", Birthday: str("2026-10-07")}, "invalid_birthday"},
		{"future", profileInput{FullName: "a", Birthday: str("2030-01-01")}, "invalid_birthday"},
		{"not a date", profileInput{FullName: "a", Birthday: str("2001-02-30")}, "invalid_birthday"},
		{"wrong format", profileInput{FullName: "a", Birthday: str("03/02/2001")}, "invalid_birthday"},
		{"unpadded", profileInput{FullName: "a", Birthday: str("2001-2-3")}, "invalid_birthday"},
		{"empty string", profileInput{FullName: "a", Birthday: str("")}, "invalid_birthday"},
		{"too old", profileInput{FullName: "a", Birthday: str("1899-12-31")}, "invalid_birthday"},
	}
	for _, tc := range tests {
		var p Profile
		if got := code(tc.in.applyTo(&p, now)); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCursorRoundTripAndTamper(t *testing.T) {
	c := cursor{time.Date(2026, 10, 7, 12, 0, 0, 123000000, time.UTC).UnixMilli(), "01J9Z3K6V8Q4M7N2P5R8T0W1XY"}
	got, ok := parseCursor(formatCursor(c))
	if !ok || got.updatedAt != c.updatedAt || got.publicID != c.publicID {
		t.Fatalf("round trip: %+v %v", got, ok)
	}
	for _, bad := range []string{"", "!!!", "YWJj", formatCursor(c) + "x", "MTIzLnNob3J0", "OTk5OTk5OTk5OTk5OTk5OTk5LjAxSjlaM0s2VjhRNE03TjJQNVI4VDBXMVhZ"} {
		if _, ok := parseCursor(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
}
