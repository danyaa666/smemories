package notes

import (
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/httpx"
)

func TestCheckIDs(t *testing.T) {
	id := func(c string) string { return strings.Repeat(c, publicIDLen) }
	many := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = strings.Repeat("A", publicIDLen-3) + string(rune('0'+i/100)) + string(rune('0'+i/10%10)) + string(rune('0'+i%10))
		}
		return out
	}
	for _, tc := range []struct {
		name       string
		ids        []string
		limit      int
		allowEmpty bool
		want       apperr.Code
		ok         bool
	}{
		{"one", []string{id("A")}, maxBatchIDs, false, apperr.Code{}, true},
		{"100", many(100), maxBatchIDs, false, apperr.Code{}, true},
		{"101", many(101), maxBatchIDs, false, CodeTooManyIDs, false},
		{"empty", nil, maxBatchIDs, false, apperr.Param, false},
		{"empty allowed", nil, maxReorderIDs, true, apperr.Code{}, true},
		{"duplicate", []string{id("A"), id("A")}, maxBatchIDs, false, apperr.Param, false},
		{"short", []string{"abc"}, maxBatchIDs, false, apperr.Param, false},
		{"lower case", []string{strings.ToLower(id("A"))}, maxBatchIDs, false, apperr.Param, false},
		{"same note twice in two cases", []string{id("A"), strings.ToLower(id("A"))}, maxBatchIDs, false, apperr.Param, false},
		{"300 reorder", many(300), maxReorderIDs, true, apperr.Code{}, true},
		{"301 reorder", many(301), maxReorderIDs, true, CodeTooManyIDs, false},
	} {
		err := checkIDs(tc.ids, tc.limit, tc.allowEmpty)
		if tc.ok != (err == nil) || (err != nil && apperr.CodeOf(err) != tc.want) {
			t.Errorf("%s: got %v", tc.name, err)
		}
	}
}

func TestStatusNames(t *testing.T) {
	for s, ok := range map[string]bool{StatusPending: true, StatusApproved: true, StatusHidden: true, "": false, "Approved": false, "deleted": false} {
		if validStatus(s) != ok {
			t.Errorf("validStatus(%q) = %v", s, !ok)
		}
	}
}

// A page token is bound to its list and carries the right kind of position.
func TestPageTokens(t *testing.T) {
	tk := httpx.NewCursors([]byte(strings.Repeat("k", 32)))
	scope := listScope("book", StatusApproved)
	got, err := openPos(tk, scope, StatusApproved, sealPos(tk, scope, StatusApproved, &notePos{sortOrder: 7}))
	if err != nil || got.sortOrder != 7 {
		t.Fatalf("approved position: %+v %v", got, err)
	}
	scope = listScope("book", "")
	got, err = openPos(tk, scope, "", sealPos(tk, scope, "", &notePos{createdAt: 1700000000000, rowID: 42}))
	if err != nil || got.createdAt != 1700000000000 || got.rowID != 42 {
		t.Fatalf("newest-first position: %+v %v", got, err)
	}
	// another list, a malformed token and a position of the wrong kind are all ERROR_PARAM
	for name, tok := range map[string]string{
		"other scope": sealPos(tk, listScope("other", ""), "", &notePos{createdAt: 1, rowID: 1}),
		"garbage":     "xx",
		"wrong kind":  tk.Seal(scope, "o:3"),
		"bad number":  tk.Seal(scope, "c:abc:1"),
	} {
		if _, err := openPos(tk, scope, "", tok); apperr.CodeOf(err) != apperr.Param {
			t.Errorf("%s: %v", name, err)
		}
	}
	if sealPos(tk, scope, "", nil) != "" {
		t.Error("the last page has no token")
	}
}

func TestIsPublicID(t *testing.T) {
	ok := "01HZX3K9QJ5V7M2N8P4R6T0WYZ"
	if !isPublicID(ok) {
		t.Fatal("canonical id rejected")
	}
	for _, bad := range []string{"", strings.ToLower(ok), ok[:25], ok + "A", "I" + ok[1:], "L" + ok[1:], "O" + ok[1:], "U" + ok[1:], ok[:25] + "-", ok[:25] + "é"} {
		if isPublicID(bad) {
			t.Fatalf("%q accepted", bad)
		}
	}
}
