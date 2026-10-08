package notefields

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCatalogueGuard(t *testing.T) {
	want := []string{"name", "nickname", "relationship", "message", "how_we_met", "first_impression", "best_memory", "wish", "advice"}
	if len(fields) != len(want) {
		t.Fatalf("catalogue has %d fields, want %d", len(fields), len(want))
	}
	seen := map[string]bool{}
	for i, f := range fields {
		if f.ID != want[i] {
			t.Errorf("field %d is %q, want %q (ids are permanent)", i, f.ID, want[i])
		}
		if seen[f.ID] {
			t.Errorf("duplicate id %q", f.ID)
		}
		seen[f.ID] = true
		if f.Label.En == "" || f.Label.Vi == "" || f.Hint.En == "" || f.Hint.Vi == "" {
			t.Errorf("%s: missing label or hint", f.ID)
		}
		if f.MaxLength <= 0 {
			t.Errorf("%s: limit %d", f.ID, f.MaxLength)
		}
		if f.Kind != ShortText && f.Kind != LongText {
			t.Errorf("%s: kind %q", f.ID, f.Kind)
		}
	}
	limits := map[string]int{"name": 60, "nickname": 40, "relationship": 60, "message": 2000, "how_we_met": 500, "first_impression": 500, "best_memory": 500, "wish": 500, "advice": 500}
	for id, n := range limits {
		if byID[id].MaxLength != n {
			t.Errorf("%s limit %d, want %d", id, byID[id].MaxLength, n)
		}
	}
}

func TestDefault(t *testing.T) {
	got := Default()
	want := []FieldRef{{"name", true}, {"relationship", false}, {"message", true}}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("Default() = %v", got)
	}
}

func code(err error) string {
	var fe *FieldError
	if errors.As(err, &fe) {
		return fe.Code + ":" + fe.ID
	}
	return "other:" + err.Error()
}

func TestValidate(t *testing.T) {
	refs := []FieldRef{{"name", true}, {"nickname", false}, {"message", true}}
	nfd := "Việt"
	tests := []struct {
		name    string
		answers map[string]string
		want    map[string]string
		err     string
		is      error
	}{
		{"ok trims and drops empty optional", map[string]string{"name": " Lan ", "nickname": "  ", "message": "hi"}, map[string]string{"name": "Lan", "message": "hi"}, "", nil},
		{"nfd becomes nfc", map[string]string{"name": nfd, "message": "x"}, map[string]string{"name": "Việt", "message": "x"}, "", nil},
		{"vietnamese emoji zwj and variation selector pass", map[string]string{"name": "Nguyễn Thị Ánh 👩‍🎓", "message": "Chúc bạn ❤️"}, map[string]string{"name": "Nguyễn Thị Ánh 👩‍🎓", "message": "Chúc bạn ❤️"}, "", nil},
		{"crlf to lf in long text", map[string]string{"name": "a", "message": "a\r\nb\nc"}, map[string]string{"name": "a", "message": "a\nb\nc"}, "", nil},
		{"unknown answer", map[string]string{"name": "a", "message": "b", "wish": "c"}, nil, "unknown_field:wish", ErrUnknownField},
		{"unlisted catalogue field is unknown", map[string]string{"name": "a", "message": "b", "advice": "c"}, nil, "unknown_field:advice", ErrUnknownField},
		{"missing required", map[string]string{"name": "a"}, nil, "missing_answer:message", ErrMissing},
		{"whitespace-only required", map[string]string{"name": "   ", "message": "b"}, nil, "missing_answer:name", ErrMissing},
		{"empty required", map[string]string{"name": "a", "message": ""}, nil, "missing_answer:message", ErrMissing},
		{"newline in short text", map[string]string{"name": "a\nb", "message": "b"}, nil, "invalid_answer:name", ErrInvalid},
		{"nul", map[string]string{"name": "a", "message": "b\x00"}, nil, "invalid_answer:message", ErrInvalid},
		{"tab in long text", map[string]string{"name": "a", "message": "b\tc"}, nil, "invalid_answer:message", ErrInvalid},
		{"lone cr in long text", map[string]string{"name": "a", "message": "b\rc"}, nil, "invalid_answer:message", ErrInvalid},
		{"bidi override", map[string]string{"name": "a\u202eb", "message": "b"}, nil, "invalid_answer:name", ErrInvalid},
		{"zero width space", map[string]string{"name": "a", "message": "b\u200bc"}, nil, "invalid_answer:message", ErrInvalid},
		{"invalid utf8", map[string]string{"name": "a\xff", "message": "b"}, nil, "invalid_answer:name", ErrInvalid},
		{"whitespace-only optional with control char", map[string]string{"name": "a", "nickname": "\x00", "message": "b"}, nil, "invalid_answer:nickname", ErrInvalid},
		{"name at limit", map[string]string{"name": strings.Repeat("Đ", 60), "message": "b"}, map[string]string{"name": strings.Repeat("Đ", 60), "message": "b"}, "", nil},
		{"name over limit by one character", map[string]string{"name": strings.Repeat("Đ", 61), "message": "b"}, nil, "invalid_answer:name", ErrInvalid},
		{"nickname limit", map[string]string{"name": "a", "nickname": strings.Repeat("x", 41), "message": "b"}, nil, "invalid_answer:nickname", ErrInvalid},
		{"message at limit", map[string]string{"name": "a", "message": strings.Repeat("🎓", 2000)}, map[string]string{"name": "a", "message": strings.Repeat("🎓", 2000)}, "", nil},
		{"message over limit", map[string]string{"name": "a", "message": strings.Repeat("🎓", 2001)}, nil, "invalid_answer:message", ErrInvalid},
		{"huge value rejected", map[string]string{"name": "a", "message": strings.Repeat("x", 10<<20)}, nil, "invalid_answer:message", ErrInvalid},
		{"huge whitespace rejected", map[string]string{"name": "a", "message": strings.Repeat(" ", 10<<20)}, nil, "invalid_answer:message", ErrInvalid},
	}
	for _, tc := range tests {
		got, err := Validate(refs, tc.answers)
		if tc.err != "" {
			if err == nil || code(err) != tc.err || !errors.Is(err, tc.is) {
				t.Errorf("%s: err = %v, want %s", tc.name, err, tc.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
			continue
		}
		for k, v := range tc.want {
			if got[k] != v {
				t.Errorf("%s: %s = %q, want %q", tc.name, k, got[k], v)
			}
		}
	}
}

func TestLongTextLimitsAreCharacters(t *testing.T) {
	refs := []FieldRef{{"wish", true}}
	if _, err := Validate(refs, map[string]string{"wish": strings.Repeat("ệ", 500)}); err != nil {
		t.Errorf("500 characters: %v", err)
	}
	if _, err := Validate(refs, map[string]string{"wish": strings.Repeat("ệ", 501)}); err == nil {
		t.Error("501 characters accepted")
	}
	// 500 decomposed characters (3 code points each, 5 bytes) are within the limit after NFC.
	if _, err := Validate(refs, map[string]string{"wish": strings.Repeat("ệ", 400)}); err != nil {
		t.Errorf("decomposed 400 characters: %v", err)
	}
}

func TestBadRefs(t *testing.T) {
	for name, refs := range map[string][]FieldRef{
		"unknown":   {{"phone", true}},
		"duplicate": {{"name", true}, {"name", false}},
	} {
		if _, err := Validate(refs, nil); err == nil {
			t.Errorf("Validate %s refs: no error", name)
		}
		if _, err := Info(refs); err == nil {
			t.Errorf("Info %s refs: no error", name)
		}
	}
	if _, err := Info([]FieldRef{{"phone", true}}); !errors.Is(err, ErrUnknownField) {
		t.Errorf("Info unknown: %v", err)
	}
}

func TestInfoJSON(t *testing.T) {
	info, err := Info([]FieldRef{{"relationship", false}, {"name", true}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(info)
	want := `[{"id":"relationship","kind":"short_text","label":{"en":"Your relationship","vi":"Mối quan hệ"},"hint":{"en":"e.g. classmate, desk mate, teacher","vi":"VD: bạn cùng lớp, bạn cùng bàn, thầy cô"},"required":false,"max_length":60},` +
		`{"id":"name","kind":"short_text","label":{"en":"Your name","vi":"Tên của bạn"},"hint":{"en":"Your full name or the name they call you","vi":"Họ tên của bạn hoặc tên mọi người hay gọi"},"required":true,"max_length":60}]`
	if string(b) != want {
		t.Errorf("JSON =\n%s\nwant\n%s", b, want)
	}
	if l, _ := Info([]FieldRef{{"message", true}}); l[0].Kind != LongText || l[0].MaxLength != 2000 {
		t.Errorf("message info = %+v", l[0])
	}
}
