package auth

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func guarded() http.Handler {
	return Guard([]string{"https://app.example.com", "http://localhost:5173"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
}

func guardReq(method, body, contentType string, hdr map[string]string, cookie bool) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, "/v1/x", rd)
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	if cookie {
		r.AddCookie(&http.Cookie{Name: CookieName, Value: "t"})
	}
	rec := httptest.NewRecorder()
	guarded().ServeHTTP(rec, r)
	return rec
}

func TestGuardOrigin(t *testing.T) {
	for _, c := range []struct {
		name   string
		hdr    map[string]string
		cookie bool
		want   int
	}{
		{"allowed origin", map[string]string{"Origin": "https://app.example.com"}, true, 204},
		{"allowed origin, mixed case", map[string]string{"Origin": "HTTPS://App.Example.com"}, true, 204},
		{"allowed dev origin", map[string]string{"Origin": "http://localhost:5173"}, true, 204},
		{"allowed referer when no origin", map[string]string{"Referer": "https://app.example.com/books/1?x=y"}, true, 204},
		{"no origin and no referer", nil, true, 403},
		{"other origin", map[string]string{"Origin": "https://evil.example.com"}, true, 403},
		{"suffix trick", map[string]string{"Origin": "https://app.example.com.evil.com"}, true, 403},
		{"userinfo trick", map[string]string{"Origin": "https://app.example.com@evil.com"}, true, 403},
		{"wrong scheme", map[string]string{"Origin": "http://app.example.com"}, true, 403},
		{"wrong port", map[string]string{"Origin": "http://localhost:5174"}, true, 403},
		{"opaque null origin", map[string]string{"Origin": "null"}, true, 403},
		{"bad origin wins over good referer", map[string]string{"Origin": "https://evil.example.com", "Referer": "https://app.example.com/"}, true, 403},
		{"no cookie: origin not checked", map[string]string{"Origin": "https://evil.example.com"}, false, 204},
		{"no cookie, no origin", nil, false, 204},
	} {
		if rec := guardReq("POST", "", "", c.hdr, c.cookie); rec.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, rec.Code, c.want)
		} else if c.want == 403 && !strings.Contains(rec.Body.String(), `"csrf_origin_mismatch"`) {
			t.Errorf("%s: body %q", c.name, rec.Body.String())
		}
	}
}

func TestGuardMethods(t *testing.T) {
	evil := map[string]string{"Origin": "https://evil.example.com"}
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if rec := guardReq(m, "", "", evil, true); rec.Code != 403 {
			t.Errorf("%s with bad origin: %d, want 403", m, rec.Code)
		}
	}
	for _, m := range []string{"GET", "HEAD", "OPTIONS"} {
		if rec := guardReq(m, "", "", evil, true); rec.Code != 204 {
			t.Errorf("%s is not state-changing, got %d", m, rec.Code)
		}
	}
}

func TestGuardJSONOnly(t *testing.T) {
	ok := map[string]string{"Origin": "https://app.example.com"}
	for ct, want := range map[string]int{
		"application/json":                  204,
		"application/json; charset=utf-8":   204,
		"Application/JSON":                  204,
		"":                                  415,
		"text/plain":                        415,
		"application/x-www-form-urlencoded": 415,
		"multipart/form-data; boundary=x":   415,
		"application/jsonx":                 415,
		"text/html; application/json":       415,
	} {
		if rec := guardReq("POST", `{"a":1}`, ct, ok, false); rec.Code != want {
			t.Errorf("Content-Type %q: got %d, want %d", ct, rec.Code, want)
		} else if want == 415 && !strings.Contains(rec.Body.String(), `"unsupported_media_type"`) {
			t.Errorf("Content-Type %q: body %q", ct, rec.Body.String())
		}
	}
	if rec := guardReq("POST", "", "", ok, false); rec.Code != 204 {
		t.Errorf("empty body needs no content type, got %d", rec.Code)
	}
	if rec := guardReq("GET", `{"a":1}`, "text/plain", nil, false); rec.Code != 204 {
		t.Errorf("GET is not checked, got %d", rec.Code)
	}
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
	plain := &Handler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	trusting := &Handler{cfg: HandlerConfig{TrustProxy: true}}
	for _, c := range []struct {
		h    *Handler
		req  *http.Request
		want string
	}{
		{plain, r("203.0.113.9:5555", "1.1.1.1"), "203.0.113.9"},
		{plain, r("[2001:db8::1]:5555"), "2001:db8::1"},
		{trusting, r("10.0.0.1:1", "1.1.1.1, 198.51.100.7"), "198.51.100.7"},
		{trusting, r("10.0.0.1:1", "9.9.9.9", "1.1.1.1, 198.51.100.8"), "198.51.100.8"},
		{trusting, r("10.0.0.1:1"), "10.0.0.1"},
		{trusting, r("10.0.0.1:1", "not-an-ip"), "10.0.0.1"},
	} {
		if got := c.h.clientIP(c.req); got != c.want {
			t.Errorf("clientIP = %q, want %q", got, c.want)
		}
	}
}
