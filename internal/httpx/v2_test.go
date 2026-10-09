package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
)

type v2Body struct {
	Status       string          `json:"status"`
	Data         json.RawMessage `json:"data"`
	ErrorMessage string          `json:"error_message"`
	RequestID    string          `json:"request_id"`
}

func decodeV2(t *testing.T, rec *httptest.ResponseRecorder) v2Body {
	t.Helper()
	var b v2Body
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil || b.Status == "" {
		t.Fatalf("body is not a v2 envelope: %q (%v)", rec.Body.String(), err)
	}
	return b
}

func TestOK(t *testing.T) {
	for _, c := range []struct {
		name string
		data any
		want string
	}{
		{"nil", nil, `{"status":"OK","data":{}}`},
		{"object", map[string]int{"n": 1}, `{"status":"OK","data":{"n":1}}`},
		{"page", Page[string]{Items: []string{"a"}, NextID: "x"}, `{"status":"OK","data":{"items":["a"],"next_id":"x"}}`},
	} {
		rec := httptest.NewRecorder()
		OK(rec, httptest.NewRequest("GET", "/api/x", nil), c.data)
		if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != c.want {
			t.Errorf("%s: %d %q, want %s", c.name, rec.Code, rec.Body.String(), c.want)
		}
	}
}

func TestFailMapsEveryCode(t *testing.T) {
	biz := apperr.NewCode("ERROR_INVALID_TITLE", 400)
	for _, c := range []struct {
		err    error
		status int
		name   string
	}{
		{apperr.New(apperr.Internal, "x"), 500, "ERROR_INTERNAL"},
		{apperr.New(apperr.Unauthorized, "x"), 401, "ERROR_UNAUTHORIZED"},
		{apperr.New(apperr.Forbidden, "x"), 403, "ERROR_FORBIDDEN"},
		{apperr.New(apperr.Param, "x"), 400, "ERROR_PARAM"},
		{apperr.New(apperr.NotFound, "x"), 404, "ERROR_NOT_FOUND"},
		{apperr.New(apperr.Conflict, "x"), 409, "ERROR_CONFLICT"},
		{apperr.New(apperr.RateLimited, "x"), 429, "ERROR_RATE_LIMITED"},
		{apperr.New(apperr.TooLarge, "x"), 413, "ERROR_TOO_LARGE"},
		{apperr.New(apperr.Unavailable, "x"), 503, "ERROR_UNAVAILABLE"},
		{apperr.New(apperr.BadGateway, "x"), 502, "ERROR_BAD_GATEWAY"},
		{apperr.New(biz, "title must be 1-120 characters"), 400, "ERROR_INVALID_TITLE"},
		{errors.New("plain"), 500, "ERROR_INTERNAL"},
		{context.Canceled, 500, "ERROR_INTERNAL"},
	} {
		rec := httptest.NewRecorder()
		Fail(rec, httptest.NewRequest("GET", "/api/x", nil), c.err)
		b := decodeV2(t, rec)
		if rec.Code != c.status || b.Status != c.name {
			t.Errorf("%v: got %d %s, want %d %s", c.err, rec.Code, b.Status, c.status, c.name)
		}
	}
}

func TestFailLoggingAndMessages(t *testing.T) {
	var logs bytes.Buffer
	h := NewRouter(slog.New(slog.NewJSONHandler(&logs, nil)), func(m *http.ServeMux) {
		m.HandleFunc("GET /api/internal", func(w http.ResponseWriter, r *http.Request) {
			Fail(w, r, apperr.Wrap(apperr.Internal, errors.New("dial tcp 10.0.0.5: secret-cause"), "insert yearbook"))
		})
		m.HandleFunc("GET /api/down", func(w http.ResponseWriter, r *http.Request) {
			Fail(w, r, apperr.Wrap(apperr.Unavailable, errors.New("redis gone"), "load session"))
		})
		m.HandleFunc("GET /api/missing", func(w http.ResponseWriter, r *http.Request) {
			Fail(w, r, apperr.New(apperr.NotFound, "yearbook not found"))
		})
		m.HandleFunc("GET /api/silent", func(w http.ResponseWriter, r *http.Request) {
			Fail(w, r, apperr.WithCode(apperr.Forbidden, errors.New("db detail")))
		})
	})

	for _, p := range []struct{ path, cause string }{{"/api/internal", "secret-cause"}, {"/api/down", "redis gone"}} {
		logs.Reset()
		rec := do(h, "GET", p.path, nil)
		b := decodeV2(t, rec)
		if b.ErrorMessage != "internal server error" || strings.Contains(rec.Body.String(), p.cause) {
			t.Errorf("%s leaks the cause: %q", p.path, rec.Body.String())
		}
		if b.RequestID == "" || b.RequestID != rec.Header().Get("X-Request-Id") {
			t.Errorf("%s request_id %q", p.path, b.RequestID)
		}
		var errLog map[string]any
		for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
			var m map[string]any
			if json.Unmarshal([]byte(line), &m) == nil && m["level"] == "ERROR" {
				errLog = m
			}
		}
		if errLog == nil || errLog["request_id"] != b.RequestID || !strings.Contains(errLog["error"].(string), p.cause) {
			t.Errorf("%s: want an ERROR log with request_id and the wrapped error, got %v", p.path, errLog)
		}
	}

	logs.Reset()
	rec := do(h, "GET", "/api/missing", nil)
	if b := decodeV2(t, rec); b.ErrorMessage != "yearbook not found" {
		t.Errorf("4xx message = %q", b.ErrorMessage)
	}
	rec = do(h, "GET", "/api/silent", nil)
	if b := decodeV2(t, rec); b.ErrorMessage != "forbidden" || strings.Contains(rec.Body.String(), "db detail") {
		t.Errorf("WithCode 4xx must not echo the cause: %q", rec.Body.String())
	}
	if strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Errorf("4xx must not log at ERROR: %s", logs.String())
	}
}

func TestParseLimit(t *testing.T) {
	for _, c := range []struct {
		query   string
		want    int
		wantErr bool
	}{
		{"", 20, false},
		{"limit=1", 1, false},
		{"limit=50", 50, false},
		{"limit=0", 0, true},
		{"limit=51", 0, true},
		{"limit=-3", 0, true},
		{"limit=abc", 0, true},
		{"limit=", 20, false},
	} {
		got, err := ParseLimit(httptest.NewRequest("GET", "/api/x?"+c.query, nil), 20, 50)
		if (err != nil) != c.wantErr || got != c.want || (err != nil && !apperr.Is(err, apperr.Param)) {
			t.Errorf("%q: got %d, %v", c.query, got, err)
		}
	}
}

func TestV2CodeMapping(t *testing.T) {
	for old, want := range map[string]string{
		"internal_error": "ERROR_INTERNAL", "invalid_body": "ERROR_PARAM", "unknown_field": "ERROR_PARAM",
		"not_found": "ERROR_NOT_FOUND", "method_not_allowed": "ERROR_METHOD_NOT_ALLOWED",
		"payload_too_large": "ERROR_TOO_LARGE", "unauthenticated": "ERROR_UNAUTHORIZED",
		"invalid_title": "ERROR_INVALID_TITLE", "rate_limited": "ERROR_RATE_LIMITED",
	} {
		if got := v2Code(old); got != want {
			t.Errorf("v2Code(%q) = %q, want %q", old, got, want)
		}
	}
}

// TestPathSwitch: the same router answers /api/... with the v2 body and everything else with the old one.
func TestPathSwitch(t *testing.T) {
	read := func(w http.ResponseWriter, r *http.Request) {
		var v struct{}
		if DecodeJSON(w, r, &v, true) {
			OK(w, r, nil)
		}
	}
	h, logs := newTestRouter(t, func(m *http.ServeMux) {
		m.HandleFunc("GET /api/only-get", func(w http.ResponseWriter, r *http.Request) { OK(w, r, nil) })
		m.HandleFunc("POST /api/read", read)
		m.HandleFunc("POST /v1/read", read)
		m.HandleFunc("GET /api/boom", func(http.ResponseWriter, *http.Request) { panic("kaboom") })
		m.HandleFunc("GET /v1/boom", func(http.ResponseWriter, *http.Request) { panic("kaboom") })
		m.Handle("GET /api/private", func() http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				WriteError(w, r, 401, "unauthenticated", "sign in required")
			})
		}())
	})
	big := strings.NewReader(`{"a":"` + strings.Repeat("x", 2<<20) + `"}`)

	rec := do(h, "GET", "/api/nope", nil)
	if b := decodeV2(t, rec); rec.Code != 404 || b.Status != "ERROR_NOT_FOUND" || b.RequestID == "" {
		t.Errorf("/api/nope: %d %+v", rec.Code, b)
	}
	rec = do(h, "PUT", "/api/only-get", nil)
	if b := decodeV2(t, rec); rec.Code != 405 || b.Status != "ERROR_METHOD_NOT_ALLOWED" || !strings.Contains(rec.Header().Get("Allow"), "GET") {
		t.Errorf("405: %d %+v Allow=%q", rec.Code, b, rec.Header().Get("Allow"))
	}
	rec = do(h, "POST", "/api/read", big, "Content-Type", "application/json")
	if b := decodeV2(t, rec); rec.Code != 413 || b.Status != "ERROR_TOO_LARGE" {
		t.Errorf("/api 413: %d %+v", rec.Code, b)
	}
	rec = do(h, "POST", "/v1/read", strings.NewReader(`{"a":"`+strings.Repeat("x", 2<<20)+`"}`), "Content-Type", "application/json")
	if e := decodeError(t, rec); rec.Code != 413 || e.Code != "payload_too_large" {
		t.Errorf("/v1 413: %d %+v", rec.Code, e)
	}
	rec = do(h, "POST", "/api/read", strings.NewReader(`{"a":1}`))
	if b := decodeV2(t, rec); rec.Code != 400 || b.Status != "ERROR_PARAM" {
		t.Errorf("unknown field under /api: %d %+v", rec.Code, b)
	}
	rec = do(h, "GET", "/api/private", nil)
	if b := decodeV2(t, rec); rec.Code != 401 || b.Status != "ERROR_UNAUTHORIZED" {
		t.Errorf("401: %d %+v", rec.Code, b)
	}
	rec = do(h, "GET", "/api/boom", nil)
	if b := decodeV2(t, rec); rec.Code != 500 || b.Status != "ERROR_INTERNAL" || strings.Contains(rec.Body.String(), "goroutine") || strings.Contains(rec.Body.String(), "kaboom") {
		t.Errorf("/api panic: %d %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs.String(), "kaboom") {
		t.Error("the panic must still be logged")
	}
	rec = do(h, "GET", "/v1/boom", nil)
	if e := decodeError(t, rec); rec.Code != 500 || e.Code != "internal_error" {
		t.Errorf("/v1 panic: %d %+v", rec.Code, e)
	}
	rec = do(h, "GET", "/v1/nope", nil)
	if e := decodeError(t, rec); rec.Code != 404 || e.Code != "not_found" {
		t.Errorf("/v1 404: %d %+v", rec.Code, e)
	}
	rec = do(h, "GET", "/apix/nope", nil)
	if e := decodeError(t, rec); rec.Code != 404 || e.Code != "not_found" {
		t.Errorf("/apix is not under /api/: %d %+v", rec.Code, e)
	}
}

// TestTimeout covers the default deadline, the per-route override and cancellation.
func TestTimeout(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	t.Run("a handler blocked on the store sees a cancelled ctx", func(t *testing.T) {
		var got error
		h := NewRouterWith(logger, RouterConfig{RequestTimeout: 20 * time.Millisecond}, func(m *http.ServeMux) {
			m.HandleFunc("GET /api/slow", func(w http.ResponseWriter, r *http.Request) {
				<-r.Context().Done() // the store stub waits for the deadline
				got = r.Context().Err()
				Fail(w, r, got)
			})
		})
		rec := do(h, "GET", "/api/slow", nil)
		if !errors.Is(got, context.DeadlineExceeded) || rec.Code != 500 || decodeV2(t, rec).Status != "ERROR_INTERNAL" {
			t.Errorf("err=%v code=%d body=%q", got, rec.Code, rec.Body.String())
		}
	})

	t.Run("a handler that finishes in time is untouched", func(t *testing.T) {
		h := Timeout(time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := r.Context().Deadline(); !ok {
				t.Error("no deadline")
			}
			w.WriteHeader(204)
		}))
		if rec := do(h, "GET", "/", nil); rec.Code != 204 || rec.Body.Len() != 0 {
			t.Errorf("middleware wrote something: %d %q", rec.Code, rec.Body.String())
		}
	})

	t.Run("the default applies when the config is zero", func(t *testing.T) {
		var d time.Duration
		h := NewRouter(logger, func(m *http.ServeMux) {
			m.HandleFunc("GET /x", func(w http.ResponseWriter, r *http.Request) {
				dl, _ := r.Context().Deadline()
				d = time.Until(dl)
			})
		})
		do(h, "GET", "/x", nil)
		if d < 29*time.Second || d > DefaultRequestTimeout {
			t.Errorf("deadline in %v, want about %v", d, DefaultRequestTimeout)
		}
	})

	type key struct{}
	t.Run("WithTimeout replaces the deadline and keeps the context values", func(t *testing.T) {
		var errAfter error
		var val any
		inner := WithTimeout(time.Second, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(60 * time.Millisecond) // longer than the 10ms default
			errAfter, val = r.Context().Err(), r.Context().Value(key{})
		}))
		h := Timeout(10 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			inner.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), key{}, "kept")))
		}))
		do(h, "GET", "/", nil)
		if errAfter != nil || val != "kept" {
			t.Errorf("err=%v val=%v", errAfter, val)
		}
	})

	t.Run("WithTimeout still stops on client disconnect and on its own deadline", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		h := Timeout(time.Hour)(WithTimeout(time.Hour, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cancel() // the client went away
			select {
			case <-r.Context().Done():
			case <-time.After(time.Second):
				t.Error("disconnect did not cancel the route context")
			}
		})))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil).WithContext(ctx))

		short := WithTimeout(10*time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
			if !errors.Is(r.Context().Err(), context.DeadlineExceeded) {
				t.Errorf("err = %v", r.Context().Err())
			}
		}))
		short.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil)) // no Timeout in front: still works
	})
}

func TestClientIP(t *testing.T) {
	r := func(remote string, xff ...string) *http.Request {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = remote
		for _, v := range xff {
			req.Header.Add("X-Forwarded-For", v)
		}
		return req
	}
	for _, c := range []struct {
		name  string
		req   *http.Request
		trust bool
		want  string
	}{
		{"remote host", r("203.0.113.9:5555", "1.1.1.1"), false, "203.0.113.9"},
		{"ipv6", r("[2001:db8::1]:5555"), false, "2001:db8::1"},
		{"no port", r("203.0.113.9"), false, "203.0.113.9"},
		{"last hop", r("10.0.0.1:1", "1.1.1.1, 198.51.100.7"), true, "198.51.100.7"},
		{"last header", r("10.0.0.1:1", "9.9.9.9", "1.1.1.1, 198.51.100.8"), true, "198.51.100.8"},
		{"no header", r("10.0.0.1:1"), true, "10.0.0.1"},
		{"not an ip", r("10.0.0.1:1", "not-an-ip"), true, "10.0.0.1"},
	} {
		if got := ResolveClientIP(c.req, c.trust); got != c.want {
			t.Errorf("%s: ResolveClientIP = %q, want %q", c.name, got, c.want)
		}
		var viaMW string
		WithClientIP(c.trust)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { viaMW = ClientIP(r) })).
			ServeHTTP(httptest.NewRecorder(), c.req)
		if viaMW != c.want {
			t.Errorf("%s: ClientIP via middleware = %q, want %q", c.name, viaMW, c.want)
		}
	}
	if got := ClientIP(r("203.0.113.9:1", "1.1.1.1")); got != "203.0.113.9" {
		t.Errorf("ClientIP without the middleware must not trust headers, got %q", got)
	}
}

func TestRouterClientIPTrust(t *testing.T) {
	var got string
	routes := func(m *http.ServeMux) {
		m.HandleFunc("GET /x", func(_ http.ResponseWriter, r *http.Request) { got = ClientIP(r) })
	}
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	for _, trust := range []bool{false, true} {
		do(NewRouterWith(logger, RouterConfig{TrustProxy: trust}, routes), "GET", "/x", nil, "X-Forwarded-For", "198.51.100.7")
		want := "192.0.2.1" // httptest's RemoteAddr
		if trust {
			want = "198.51.100.7"
		}
		if got != want {
			t.Errorf("trust=%v: %q, want %q", trust, got, want)
		}
	}
}
