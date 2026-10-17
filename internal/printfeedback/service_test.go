package printfeedback

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/ratelimit"
)

type fakeStore struct {
	mu   sync.Mutex
	hits map[string]int
}

func (f *fakeStore) increment(_ context.Context, family, verdict string, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits[family+"/"+verdict]++
	return nil
}

func newTest() (*Service, *fakeStore) {
	st := &fakeStore{hits: map[string]int{}}
	return &Service{store: st, ip: ratelimit.NewMemory(perIPHour, time.Hour, nil), now: time.Now}, st
}

func do(h http.Handler, ip, ct, body string, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/print-feedback/create", strings.NewReader(body))
	r.Header.Set("Content-Type", ct)
	r.RemoteAddr = ip + ":1234"
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func router(svc *Service) http.Handler {
	ip := func(r *http.Request) string { return strings.Split(r.RemoteAddr, ":")[0] }
	return httpx.NewRouter(slog.New(slog.DiscardHandler), NewHandler(svc, []string{"http://localhost:5173"}, ip).Routes)
}

func TestCreateCounts(t *testing.T) {
	svc, st := newTest()
	h := router(svc)
	for range 2 {
		if rec := do(h, "1.1.1.1", "application/json", `{"verdict":"yes","browser_family":"safari_ios"}`); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"data":{}`) {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	}
	do(h, "1.1.1.1", "application/json", `{"verdict":"no","browser_family":"other"}`)
	if st.hits["safari_ios/yes"] != 2 || st.hits["other/no"] != 1 || len(st.hits) != 2 {
		t.Fatalf("hits %v", st.hits)
	}
}

// A strict body: anything but the two enum fields is ERROR_PARAM and counts nothing.
func TestCreateRejects(t *testing.T) {
	svc, st := newTest()
	h := router(svc)
	for _, body := range []string{
		``, `[]`, `null`, `{}`, `not json`,
		`{"verdict":"maybe","browser_family":"firefox"}`, `{"verdict":"","browser_family":"firefox"}`, `{"verdict":"YES","browser_family":"firefox"}`,
		`{"verdict":"yes","browser_family":"Mozilla/5.0"}`, `{"verdict":"yes","browser_family":"chrome"}`, `{"verdict":"yes"}`, `{"browser_family":"firefox"}`,
		`{"verdict":"yes","browser_family":"firefox","user_agent":"x"}`, `{"verdict":"yes","browser_family":"firefox","count":99}`,
		`{"verdict":"yes","browser_family":"firefox"} {}`, `{"verdict":1,"browser_family":"firefox"}`,
		`{"verdict":"yes","browser_family":"` + strings.Repeat("a", 4000) + `"}`,
	} {
		rec := do(h, "1.1.1.1", "application/json", body)
		if rec.Code != 400 && rec.Code != 413 {
			t.Errorf("%.40q: %d %s", body, rec.Code, rec.Body.String())
		}
		if rec.Code == 400 && !strings.Contains(rec.Body.String(), `"ERROR_PARAM"`) {
			t.Errorf("%.40q: %s", body, rec.Body.String())
		}
	}
	if len(st.hits) != 0 {
		t.Fatalf("counted %v", st.hits)
	}
}

func TestCreateRateLimitPerIP(t *testing.T) {
	svc, st := newTest()
	h := router(svc)
	const body = `{"verdict":"yes","browser_family":"firefox"}`
	for range perIPHour {
		if rec := do(h, "2.2.2.2", "application/json", body); rec.Code != 200 {
			t.Fatalf("%d", rec.Code)
		}
	}
	rec := do(h, "2.2.2.2", "application/json", body)
	if rec.Code != 429 || !strings.Contains(rec.Body.String(), "ERROR_RATE_LIMITED") || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("%d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	if do(h, "3.3.3.3", "application/json", body).Code != 200 {
		t.Fatal("another IP must not share the limit")
	}
	if st.hits["firefox/yes"] != perIPHour+1 {
		t.Fatalf("hits %v", st.hits)
	}
}

// Like every state-changing POST: with the session cookie the Origin must be an allowed one; the body must be JSON.
func TestCreateGuard(t *testing.T) {
	svc, st := newTest()
	h := router(svc)
	const body = `{"verdict":"yes","browser_family":"firefox"}`
	if rec := do(h, "1.1.1.1", "application/json", body, "Cookie", "smem_session=x", "Origin", "https://evil.example"); rec.Code != 403 {
		t.Fatalf("foreign origin with a cookie: %d", rec.Code)
	}
	if rec := do(h, "1.1.1.1", "application/json", body, "Cookie", "smem_session=x", "Origin", "http://localhost:5173"); rec.Code != 200 {
		t.Fatalf("own origin with a cookie: %d", rec.Code)
	}
	if rec := do(h, "1.1.1.1", "text/plain", body); rec.Code != 415 {
		t.Fatalf("text/plain: %d", rec.Code)
	}
	if st.hits["firefox/yes"] != 1 {
		t.Fatalf("hits %v", st.hits)
	}
}

func TestEnumsAreDistinctAndNonEmpty(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range append(append([]string{}, Verdicts...), Families...) {
		if v == "" || len(v) > 24 || seen[v] {
			t.Errorf("bad or duplicate %q", v)
		}
		seen[v] = true
	}
}
