package yearbook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/auth"
)

// Audit M3 (T-100): every endpoint refuses an id that is not a canonical ULID with ERROR_PARAM before the store or the object
// store is touched. The public_id column compares case-insensitively, so a lower-case id would otherwise find a real book.
func TestHandlersRefuseNonCanonicalIDs(t *testing.T) {
	const id = "01J9Z3K6V8Q4M7N2P5R8T0W1XY"
	purged := 0
	f := &fakeStore{books: map[uint64]map[string]Yearbook{1: {id: {ID: id}, strings.ToLower(id): {ID: id}}}}
	h := &Handler{svc: newTestService(f, purgerFunc(func(string) error { purged++; return nil }))}
	user := auth.WithUser(context.Background(), &auth.User{InternalID: 1})

	bad := []string{strings.ToLower(id), id[:13] + strings.ToLower(id[13:]), id[:25], id + "A", " " + id[1:], "I" + id[1:], id[:25] + "/", "../" + id[3:]}
	for _, b := range bad {
		for name, c := range map[string]struct {
			f    http.HandlerFunc
			body string
		}{
			"get":           {h.get, ""},
			"update":        {h.update, `{"id":"` + b + `","title":"x"}`},
			"updateProfile": {h.updateProfile, `{"yearbook_id":"` + b + `","full_name":"x"}`},
			"delete":        {h.delete, `{"id":"` + b + `"}`},
			"revoke":        {h.revokePhotoLinks, `{"id":"` + b + `"}`},
		} {
			var r *http.Request
			if name == "get" {
				r = httptest.NewRequest(http.MethodGet, "/api/yearbook/get?id="+strings.ReplaceAll(b, " ", "%20"), nil)
			} else {
				r = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body))
			}
			rec := httptest.NewRecorder()
			c.f(rec, r.WithContext(user))
			if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"ERROR_PARAM"`) {
				t.Errorf("%s with id %q: %d %s", name, b, rec.Code, rec.Body)
			}
		}
	}
	// A photo id of the cover or the profile photo is checked the same way (ERROR_INVALID_MEDIA, the code of an unknown photo).
	for name, c := range map[string]struct {
		f    http.HandlerFunc
		body string
	}{
		"cover":   {h.update, `{"id":"` + id + `","cover_media_id":"` + strings.ToLower(id) + `"}`},
		"profile": {h.updateProfile, `{"yearbook_id":"` + id + `","full_name":"x","photo_media_id":"` + strings.ToLower(id) + `"}`},
	} {
		rec := httptest.NewRecorder()
		c.f(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body)).WithContext(user))
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"ERROR_INVALID_MEDIA"`) {
			t.Errorf("%s photo id: %d %s", name, rec.Code, rec.Body)
		}
	}
	if f.calls != 0 || purged != 0 {
		t.Fatalf("a refused id reached the store (%d calls) or the object store (%d purges)", f.calls, purged)
	}

	// The canonical spelling still works.
	rec := httptest.NewRecorder()
	h.get(rec, httptest.NewRequest(http.MethodGet, "/api/yearbook/get?id="+id, nil).WithContext(user))
	if rec.Code != 200 {
		t.Fatalf("canonical id: %d %s", rec.Code, rec.Body)
	}
}
