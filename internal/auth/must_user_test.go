package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// T-085: a route mounted without RequireUser answers ERROR_UNAUTHORIZED instead of running with a nil user.
func TestHandlersRefuseARequestWithoutAUser(t *testing.T) {
	h := &Handler{}
	for name, f := range map[string]http.HandlerFunc{
		"me":                 h.me,
		"verifyEmail":        h.verifyEmail,
		"resendVerification": h.resendVerification,
		"updateUsername":     h.updateUsername,
		"acceptTerms":        h.acceptTerms,
	} {
		rec := httptest.NewRecorder()
		f(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "ERROR_UNAUTHORIZED") {
			t.Errorf("%s: want 401 ERROR_UNAUTHORIZED, got %d %s", name, rec.Code, rec.Body)
		}
	}
}

func TestMustUserReturnsTheStoredUser(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(WithUser(r.Context(), &User{InternalID: 7}))
	if u, ok := MustUser(httptest.NewRecorder(), r); !ok || u.InternalID != 7 {
		t.Fatalf("got %v, %v", u, ok)
	}
}
