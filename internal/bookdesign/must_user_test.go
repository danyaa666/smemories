package bookdesign

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A route mounted without RequireUser answers ERROR_UNAUTHORIZED instead of running with a nil user (T-085).
func TestHandlersRefuseARequestWithoutAUser(t *testing.T) {
	h := &Handler{}
	for name, f := range map[string]http.HandlerFunc{"get": h.get, "update": h.update, "reset": h.reset} {
		rec := httptest.NewRecorder()
		f(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "ERROR_UNAUTHORIZED") {
			t.Errorf("%s: want 401 ERROR_UNAUTHORIZED, got %d %s", name, rec.Code, rec.Body)
		}
	}
}
