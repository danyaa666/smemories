package yearbook

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
)

func code(err error) string {
	if err == nil {
		return ""
	}
	return apperr.CodeOf(err).Name
}

func parseBook(t *testing.T, js string) BookInput {
	t.Helper()
	var in BookInput
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
		mk := func(v string) BookInput {
			b, _ := json.Marshal(map[string]string{tc.field: v})
			return parseBook(t, string(b))
		}
		if err := mk(rep(tc.max)).applyTo(&Yearbook{}); err != nil {
			t.Errorf("%s at max: %v", tc.field, err)
		}
		if got := code(mk(rep(tc.max + 1)).applyTo(&Yearbook{})); got != "ERROR_INVALID_"+strings.ToUpper(tc.field) {
			t.Errorf("%s over max: %q", tc.field, got)
		}
		if got := code(mk("a\x07b").applyTo(&Yearbook{})); got != "ERROR_INVALID_"+strings.ToUpper(tc.field) {
			t.Errorf("%s control char: %q", tc.field, got)
		}
		if err := mk("").applyTo(&Yearbook{}); (tc.min == 1) == (err == nil) {
			t.Errorf("%s empty: err=%v, min=%d", tc.field, err, tc.min)
		}
	}
}

func TestBookEnumsAndYear(t *testing.T) {
	bad := []struct{ js, want string }{
		{`{"language":"fr"}`, "ERROR_INVALID_LANGUAGE"},
		{`{"language":""}`, "ERROR_INVALID_LANGUAGE"},
		{`{"page_size":"A3"}`, "ERROR_INVALID_PAGE_SIZE"},
		{`{"page_size":"letter"}`, "ERROR_INVALID_PAGE_SIZE"},
		{`{"page_size":"LETTER"}`, "ERROR_INVALID_PAGE_SIZE"},
		{`{"page_size":"Legal"}`, "ERROR_INVALID_PAGE_SIZE"},
		{`{"graduation_year":1949}`, "ERROR_INVALID_GRADUATION_YEAR"},
		{`{"graduation_year":2101}`, "ERROR_INVALID_GRADUATION_YEAR"},
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
		in   ProfileInput
		want string
	}{
		{"ok minimal", ProfileInput{FullName: "Lan"}, ""},
		{"ok yesterday", ProfileInput{FullName: "Lan", Birthday: str("2026-10-06")}, ""},
		{"ok leap day", ProfileInput{FullName: "Lan", Birthday: str("2000-02-29")}, ""},
		{"empty name", ProfileInput{FullName: "  "}, "ERROR_INVALID_FULL_NAME"},
		{"long name", ProfileInput{FullName: strings.Repeat("a", 101)}, "ERROR_INVALID_FULL_NAME"},
		{"long nickname", ProfileInput{FullName: "a", Nickname: strings.Repeat("a", 51)}, "ERROR_INVALID_NICKNAME"},
		{"long quote", ProfileInput{FullName: "a", Quote: strings.Repeat("a", 501)}, "ERROR_INVALID_QUOTE"},
		{"max quote", ProfileInput{FullName: "a", Quote: strings.Repeat("ế", 500)}, ""},
		{"long hobbies", ProfileInput{FullName: "a", Hobbies: strings.Repeat("a", 301)}, "ERROR_INVALID_HOBBIES"},
		{"long plans", ProfileInput{FullName: "a", FuturePlans: strings.Repeat("a", 301)}, "ERROR_INVALID_FUTURE_PLANS"},
		{"newline in quote", ProfileInput{FullName: "a", Quote: "a\nb"}, "ERROR_INVALID_QUOTE"},
		{"today", ProfileInput{FullName: "a", Birthday: str("2026-10-07")}, "ERROR_INVALID_BIRTHDAY"},
		{"future", ProfileInput{FullName: "a", Birthday: str("2030-01-01")}, "ERROR_INVALID_BIRTHDAY"},
		{"not a date", ProfileInput{FullName: "a", Birthday: str("2001-02-30")}, "ERROR_INVALID_BIRTHDAY"},
		{"wrong format", ProfileInput{FullName: "a", Birthday: str("03/02/2001")}, "ERROR_INVALID_BIRTHDAY"},
		{"unpadded", ProfileInput{FullName: "a", Birthday: str("2001-2-3")}, "ERROR_INVALID_BIRTHDAY"},
		{"empty string", ProfileInput{FullName: "a", Birthday: str("")}, "ERROR_INVALID_BIRTHDAY"},
		{"too old", ProfileInput{FullName: "a", Birthday: str("1899-12-31")}, "ERROR_INVALID_BIRTHDAY"},
	}
	for _, tc := range tests {
		var p Profile
		if got := code(tc.in.applyTo(&p, now)); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// deref is the id as a slice, so nil and a value compare with slices.Equal.
func deref(s *string) []string {
	if s == nil {
		return nil
	}
	return []string{*s}
}

// T-061: template_id is validated against the registry and the book's page size (the size after the same change).
func TestBookTemplate(t *testing.T) {
	str := func(s string) *string { return &s }
	tests := []struct {
		name string
		book Yearbook
		js   string
		want string // error code, "" for success
		tmpl *string
	}{
		{"known template", Yearbook{PageSize: "A5"}, `{"template_id":"memory-book"}`, "", str("memory-book")},
		{"unknown template", Yearbook{PageSize: "A5"}, `{"template_id":"nope"}`, "ERROR_INVALID_TEMPLATE", nil},
		{"empty id", Yearbook{PageSize: "A5"}, `{"template_id":""}`, "ERROR_INVALID_TEMPLATE", nil},
		{"id over the column size", Yearbook{PageSize: "A5"}, `{"template_id":"` + strings.Repeat("a", maxTemplateID+1) + `"}`, "ERROR_INVALID_TEMPLATE", nil},
		{"template without the book's size", Yearbook{PageSize: "Letter"}, `{"template_id":"classic"}`, "ERROR_INVALID_TEMPLATE", nil},
		{"size and template together", Yearbook{PageSize: "A5"}, `{"template_id":"memory-book","page_size":"Letter"}`, "", str("memory-book")},
		{"null clears", Yearbook{PageSize: "A5", TemplateID: str("memory-book")}, `{"template_id":null}`, "", nil},
		{"absent keeps", Yearbook{PageSize: "A5", TemplateID: str("memory-book")}, `{"title":"x"}`, "", str("memory-book")},
		{"size change the template cannot print", Yearbook{PageSize: "A5", TemplateID: str("classic")}, `{"page_size":"Letter"}`, "ERROR_INVALID_TEMPLATE", nil},
		{"size change the template can print", Yearbook{PageSize: "A5", TemplateID: str("classic")}, `{"page_size":"A4"}`, "", str("classic")},
		{"size change and clear together", Yearbook{PageSize: "A5", TemplateID: str("classic")}, `{"page_size":"Letter","template_id":null}`, "", nil},
		{"size change on a template the registry dropped", Yearbook{PageSize: "A5", TemplateID: str("old-gone")}, `{"page_size":"A4"}`, "", str("old-gone")},
	}
	for _, tc := range tests {
		y := tc.book
		err := parseBook(t, tc.js).applyTo(&y)
		if got := code(err); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
			continue
		}
		if tc.want == "" && !slices.Equal(deref(y.TemplateID), deref(tc.tmpl)) {
			t.Errorf("%s: template %v, want %v", tc.name, y.TemplateID, tc.tmpl)
		}
	}
}
