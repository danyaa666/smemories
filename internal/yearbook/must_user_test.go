package yearbook

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
		"create":        h.create,
		"list":          h.list,
		"get":           h.get,
		"update":        h.update,
		"updateProfile": h.updateProfile,
		"delete":        h.delete,
		"revoke":        h.revokePhotoLinks,
	} {
		rec := httptest.NewRecorder()
		f(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "ERROR_UNAUTHORIZED") {
			t.Errorf("%s: want 401 ERROR_UNAUTHORIZED, got %d %s", name, rec.Code, rec.Body)
		}
	}
}
