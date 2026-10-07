package httpx

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func newTestRouter(t *testing.T, routes ...func(*http.ServeMux)) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	return NewRouter(slog.New(slog.NewJSONHandler(&logs, nil)), routes...), &logs
}

func do(h http.Handler, method, path string, body io.Reader, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) ErrorBody {
	t.Helper()
	var env struct{ Error ErrorBody }
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not an error envelope: %q (%v)", rec.Body.String(), err)
	}
	return env.Error
}

func TestHealthz(t *testing.T) {
	h, _ := newTestRouter(t)
	rec := do(h, "GET", "/healthz", nil)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestNotFoundEnvelope(t *testing.T) {
	h, _ := newTestRouter(t)
	rec := do(h, "GET", "/nope", nil)
	e := decodeError(t, rec)
	if rec.Code != 404 || e.Code != "not_found" || e.RequestID == "" || e.RequestID != rec.Header().Get("X-Request-Id") {
		t.Fatalf("got %d %+v", rec.Code, e)
	}
}

func TestMethodNotAllowedEnvelope(t *testing.T) {
	h, _ := newTestRouter(t)
	rec := do(h, "POST", "/healthz", nil)
	e := decodeError(t, rec)
	if rec.Code != 405 || e.Code != "method_not_allowed" {
		t.Fatalf("got %d %+v", rec.Code, e)
	}
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, "GET") {
		t.Fatalf("Allow = %q, want it to list GET", allow)
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	h, _ := newTestRouter(t, func(m *http.ServeMux) {
		m.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("x") })
	})
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"X-Frame-Options":        "DENY",
		"Cache-Control":          "no-store",
	}
	for _, p := range []string{"/healthz", "/nope", "/boom"} {
		rec := do(h, "GET", p, nil)
		for k, v := range want {
			if got := rec.Header().Get(k); got != v {
				t.Errorf("%s: %s = %q, want %q", p, k, got, v)
			}
		}
	}
	if rec := do(h, "POST", "/healthz", nil); rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("405 response is missing security headers")
	}
}

func TestRequestID(t *testing.T) {
	h, _ := newTestRouter(t)
	valid := regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

	if got := do(h, "GET", "/healthz", nil, "X-Request-Id", "client-id-1234").Header().Get("X-Request-Id"); got != "client-id-1234" {
		t.Errorf("valid client id not reused: %q", got)
	}
	for _, bad := range []string{"short", strings.Repeat("a", 65), "has space 12345", "ünïcode-12345", "ok-id-12\tx"} {
		got := do(h, "GET", "/healthz", nil, "X-Request-Id", bad).Header().Get("X-Request-Id")
		if got == bad || !valid.MatchString(got) {
			t.Errorf("bad id %q: got %q, want a freshly generated valid id", bad, got)
		}
	}
	a := do(h, "GET", "/healthz", nil).Header().Get("X-Request-Id")
	b := do(h, "GET", "/healthz", nil).Header().Get("X-Request-Id")
	if a == b || !valid.MatchString(a) {
		t.Errorf("generated ids should be valid and unique: %q %q", a, b)
	}
}

func TestAccessLogLine(t *testing.T) {
	h, logs := newTestRouter(t)
	do(h, "GET", "/healthz?token=secret", nil, "X-Request-Id", "req-12345678", "Authorization", "Bearer s3cret", "Cookie", "sid=s3cret")
	line := logs.String()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &m); err != nil {
		t.Fatalf("log line is not one JSON object: %q", line)
	}
	if m["request_id"] != "req-12345678" || m["method"] != "GET" || m["path"] != "/healthz" || m["status"] != float64(200) {
		t.Errorf("unexpected fields: %v", m)
	}
	if _, ok := m["duration_ms"]; !ok {
		t.Error("duration_ms missing")
	}
	if strings.Contains(line, "s3cret") || strings.Contains(line, "token") {
		t.Errorf("log leaks secrets: %q", line)
	}
}

func TestPanicRecovery(t *testing.T) {
	h, logs := newTestRouter(t, func(m *http.ServeMux) {
		m.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("kaboom") })
	})
	rec := do(h, "GET", "/boom", nil)
	if e := decodeError(t, rec); rec.Code != 500 || e.Code != "internal_error" || e.RequestID == "" {
		t.Fatalf("got %d %+v", rec.Code, e)
	}
	if !strings.Contains(logs.String(), "kaboom") || !strings.Contains(logs.String(), `"stack"`) {
		t.Errorf("panic and stack not logged: %q", logs.String())
	}
	if !strings.Contains(logs.String(), `"status":500`) {
		t.Errorf("access log should record the 500: %q", logs.String())
	}
	if rec := do(h, "GET", "/healthz", nil); rec.Code != 200 {
		t.Fatalf("server stopped serving after a panic: %d", rec.Code)
	}
}

func echoLen(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		WriteBodyError(w, r, err)
		return
	}
	WriteJSON(w, 200, map[string]int{"n": len(b)})
}

func TestBodyLimit(t *testing.T) {
	h, _ := newTestRouter(t, func(m *http.ServeMux) {
		m.HandleFunc("POST /echo", echoLen)
		m.Handle("POST /upload", WithBodyLimit(4<<20, http.HandlerFunc(echoLen)))
	})
	big := strings.Repeat("a", DefaultMaxBody+1)

	if rec := do(h, "POST", "/echo", strings.NewReader(strings.Repeat("a", DefaultMaxBody))); rec.Code != 200 {
		t.Errorf("body exactly at the cap: got %d", rec.Code)
	}
	rec := do(h, "POST", "/echo", strings.NewReader(big))
	if e := decodeError(t, rec); rec.Code != 413 || e.Code != "payload_too_large" {
		t.Errorf("over the cap: got %d %+v", rec.Code, e)
	}
	if rec := do(h, "POST", "/upload", strings.NewReader(big)); rec.Code != 200 {
		t.Errorf("per-route override: got %d", rec.Code)
	}
	if rec := do(h, "POST", "/upload", strings.NewReader(strings.Repeat("a", 4<<20+1))); rec.Code != 413 {
		t.Errorf("per-route cap not enforced: got %d", rec.Code)
	}
}

func TestPathValuesStillWork(t *testing.T) {
	h, _ := newTestRouter(t, func(m *http.ServeMux) {
		m.HandleFunc("GET /v1/x/{id}", func(w http.ResponseWriter, r *http.Request) { WriteJSON(w, 200, r.PathValue("id")) })
	})
	if rec := do(h, "GET", "/v1/x/abc", nil); strings.TrimSpace(rec.Body.String()) != `"abc"` {
		t.Fatalf("path value lost: %q", rec.Body.String())
	}
}
