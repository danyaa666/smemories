package class

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/httpx"
)

func TestAtLeast(t *testing.T) {
	for _, tc := range []struct {
		role, min string
		want      bool
	}{
		{RoleAdmin, RoleAdmin, true}, {RoleAdmin, RoleStudent, true}, {RoleStudent, RoleStudent, true},
		{RoleStudent, RoleAdmin, false}, {"", RoleStudent, false}, {"owner", RoleStudent, false}, {RoleAdmin, "owner", false},
	} {
		if got := atLeast(tc.role, tc.min); got != tc.want {
			t.Errorf("atLeast(%q,%q) = %v", tc.role, tc.min, got)
		}
	}
}

func TestClean(t *testing.T) {
	year := func(y int) *int { return &y }
	out, err := (&ClassInput{Name: "  Lớp  12A1 ", SchoolName: " THPT ", GraduationYear: year(2026), Language: "vi"}).clean()
	if err != nil || out.Name != "Lớp  12A1" || out.SchoolName != "THPT" || *out.GraduationYear != 2026 {
		t.Fatalf("clean = %+v, %v", out, err)
	}
	for name, tc := range map[string]struct {
		in   ClassInput
		code string
	}{
		"name":     {ClassInput{Name: " ", Language: "en"}, "ERROR_INVALID_NAME"},
		"school":   {ClassInput{Name: "x", SchoolName: strings.Repeat("s", maxSchool+1), Language: "en"}, "ERROR_INVALID_SCHOOL_NAME"},
		"year":     {ClassInput{Name: "x", GraduationYear: year(minGradYear - 1), Language: "en"}, "ERROR_INVALID_GRADUATION_YEAR"},
		"language": {ClassInput{Name: "x", Language: "de"}, "ERROR_INVALID_LANGUAGE"},
	} {
		if _, err := tc.in.clean(); apperr.CodeOf(err).Name != tc.code || apperr.CodeOf(err).HTTP != http.StatusBadRequest {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCursorIsBoundToItsList(t *testing.T) {
	c := httpx.NewCursors([]byte(strings.Repeat("k", 32)))
	tok := sealPos(c, classListScope(1), 42)
	if row, err := openPos(c, classListScope(1), tok); err != nil || row != 42 {
		t.Fatalf("open = %d, %v", row, err)
	}
	for name, scope := range map[string]string{"other user": classListScope(2), "other list": memberListScope(1, "C"), "other class": memberListScope(1, "D")} {
		if _, err := openPos(c, scope, tok); !apperr.Is(err, apperr.Param) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if sealPos(c, classListScope(1), 0) != "" {
		t.Error("the last page has a token")
	}
	if _, err := openPos(c, classListScope(1), c.Seal(classListScope(1), "0")); !apperr.Is(err, apperr.Param) {
		t.Errorf("row 0: %v", err)
	}
	if _, err := openPos(c, classListScope(1), c.Seal(classListScope(1), "-3")); !apperr.Is(err, apperr.Param) {
		t.Errorf("negative row: %v", err)
	}
}

// A route mounted without RequireUser answers ERROR_UNAUTHORIZED instead of running with a nil user (as in T-085).
func TestHandlersRefuseARequestWithoutAUser(t *testing.T) {
	h := &Handler{}
	for name, f := range map[string]http.HandlerFunc{
		"create": h.create, "list": h.list, "get": h.get, "update": h.update, "delete": h.delete,
		"members": h.members, "updateRole": h.updateRole, "removeMember": h.removeMember, "leave": h.leave,
	} {
		rec := httptest.NewRecorder()
		f(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "ERROR_UNAUTHORIZED") {
			t.Errorf("%s: want 401 ERROR_UNAUTHORIZED, got %d %s", name, rec.Code, rec.Body)
		}
	}
}

// The codes and statuses the matrix in docs/class-permissions.md promises.
func TestErrorCodes(t *testing.T) {
	if apperr.CodeOf(ErrNotFound) != apperr.NotFound || apperr.CodeOf(ErrForbidden) != apperr.Forbidden || apperr.CodeOf(ErrLastAdmin) != CodeLastAdmin {
		t.Fatal("error codes changed")
	}
	if CodeLastAdmin.HTTP != http.StatusForbidden || CodeEmailNotVerified.HTTP != http.StatusForbidden || CodeLimitReached.HTTP != http.StatusConflict {
		t.Fatal("HTTP statuses changed")
	}
}

func TestInviteToken(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		tok, hash, err := newInviteToken()
		if err != nil || len(tok) != tokenLen || seen[tok] {
			t.Fatalf("token %q, %v", tok, err)
		}
		seen[tok] = true
		if got, ok := inviteHash(tok); !ok || got != hash {
			t.Fatalf("inviteHash(%q) = %x, %v; want %x", tok, got, ok, hash)
		}
	}
	for name, s := range map[string]string{
		"empty": "", "short": strings.Repeat("a", tokenLen-1), "long": strings.Repeat("a", tokenLen+1), "padding": strings.Repeat("a", tokenLen-1) + "=",
		"space": strings.Repeat("a", tokenLen-1) + " ", "slash": strings.Repeat("a", tokenLen-1) + "/", "unicode": strings.Repeat("é", tokenLen/2),
	} {
		if _, ok := inviteHash(s); ok {
			t.Errorf("%s is accepted as a token", name)
		}
	}
}

// A class of 60 shares one school address: the per-IP budgets keep room for it (T-125 QA), the guessing limit stays small.
func TestPerIPBudgetsFitAClassOnOneNetwork(t *testing.T) {
	if lookupAllPerWindow < 600 || acceptPerIPWindow < 300 || lookupMissesPerWindow != 20 {
		t.Fatalf("lookups %d, accepts %d, misses %d", lookupAllPerWindow, acceptPerIPWindow, lookupMissesPerWindow)
	}
}
