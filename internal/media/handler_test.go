package media

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func upload(t *testing.T, h *Handler, limit int64, build func(*multipart.Writer)) (*httptest.ResponseRecorder, []byte, bool) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	build(mw)
	_ = mw.Close()
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if limit > 0 {
		r.Body = http.MaxBytesReader(rec, r.Body, limit)
	}
	data, ok := h.readFile(rec, r)
	return rec, data, ok
}

func TestReadFile(t *testing.T) {
	h := &Handler{maxBytes: 100}
	file := func(name string, n int) func(*multipart.Writer) {
		return func(mw *multipart.Writer) {
			w, _ := mw.CreateFormFile(name, "../../etc/passwd")
			_, _ = w.Write(bytes.Repeat([]byte("a"), n))
		}
	}

	if rec, data, ok := upload(t, h, 0, file("file", 100)); !ok || len(data) != 100 {
		t.Fatalf("100 bytes at the cap: ok=%v len=%d %s", ok, len(data), rec.Body)
	}
	if rec, _, ok := upload(t, h, 0, file("file", 101)); ok || rec.Code != 413 || !strings.Contains(rec.Body.String(), "payload_too_large") {
		t.Fatalf("101 bytes: ok=%v %d %s", ok, rec.Code, rec.Body)
	}
	if rec, _, ok := upload(t, h, 150, file("file", 1000)); ok || rec.Code != 413 { // the request body cap trips first
		t.Fatalf("body cap: ok=%v %d %s", ok, rec.Code, rec.Body)
	}
	if rec, _, ok := upload(t, h, 0, file("other", 10)); ok || rec.Code != 400 || !strings.Contains(rec.Body.String(), "invalid_body") {
		t.Fatalf("wrong field name: ok=%v %d %s", ok, rec.Code, rec.Body)
	}
	if rec, _, ok := upload(t, h, 0, func(*multipart.Writer) {}); ok || rec.Code != 400 {
		t.Fatalf("empty body: ok=%v %d", ok, rec.Code)
	}
	// Other fields are skipped; the file may come after them.
	if _, data, ok := upload(t, h, 0, func(mw *multipart.Writer) {
		_ = mw.WriteField("caption", "x")
		file("file", 5)(mw)
	}); !ok || len(data) != 5 {
		t.Fatalf("file after another field: ok=%v len=%d", ok, len(data))
	}

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"file":"x"}`))
	r.Header.Set("Content-Type", "application/json")
	if _, ok := h.readFile(rec, r); ok || rec.Code != 400 {
		t.Fatalf("not multipart: ok=%v %d", ok, rec.Code)
	}
}

// deadlineWriter records the deadlines a handler asks for, like a real connection would apply them.
type deadlineWriter struct {
	http.ResponseWriter
	read, write time.Time
}

func (d *deadlineWriter) SetReadDeadline(t time.Time) error  { d.read = t; return nil }
func (d *deadlineWriter) SetWriteDeadline(t time.Time) error { d.write = t; return nil }

func TestExtendDeadlines(t *testing.T) {
	w := &deadlineWriter{ResponseWriter: httptest.NewRecorder()}
	extend(w, true)
	if time.Until(w.read) < time.Minute || time.Until(w.write) < time.Minute {
		t.Fatalf("deadlines not extended: read in %v, write in %v", time.Until(w.read), time.Until(w.write))
	}
	w = &deadlineWriter{ResponseWriter: httptest.NewRecorder()}
	extend(w, false)
	if !w.read.IsZero() || time.Until(w.write) < time.Minute {
		t.Fatal("download route should extend only the write deadline")
	}
	extend(httptest.NewRecorder(), true) // a writer without deadline support is fine
}

func TestUploadBusyWhenTooManyInFlight(t *testing.T) {
	h := &Handler{inflight: make(chan struct{}, 1)}
	h.inflight <- struct{}{} // one upload already running
	rec := httptest.NewRecorder()
	h.upload(rec, httptest.NewRequest("POST", "/", nil))
	if rec.Code != 503 || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), `"busy"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}
