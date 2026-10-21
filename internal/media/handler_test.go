package media

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
)

func nopLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

const bookID = "01J9Z3K6V8Q4M7N2P5R8T0W1XY"

func upload(t *testing.T, h *Handler, limit int64, build func(*multipart.Writer)) (rec *httptest.ResponseRecorder, id string, data []byte, ok bool) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	build(mw)
	_ = mw.Close()
	rec = httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/media/create", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if limit > 0 {
		r.Body = http.MaxBytesReader(rec, r.Body, limit)
	}
	id, data, ok = h.readForm(rec, r)
	return rec, id, data, ok
}

func TestReadForm(t *testing.T) {
	h := &Handler{maxBytes: 100, logger: nopLogger()}
	file := func(name string, n int) func(*multipart.Writer) {
		return func(mw *multipart.Writer) {
			w, _ := mw.CreateFormFile(name, "../../etc/passwd")
			_, _ = w.Write(bytes.Repeat([]byte("a"), n))
		}
	}
	withBook := func(parts ...func(*multipart.Writer)) func(*multipart.Writer) {
		return func(mw *multipart.Writer) {
			_ = mw.WriteField("yearbook_id", bookID)
			for _, p := range parts {
				p(mw)
			}
		}
	}

	if rec, id, data, ok := upload(t, h, 0, withBook(file("file", 100))); !ok || len(data) != 100 || id != bookID {
		t.Fatalf("100 bytes at the cap: ok=%v len=%d id=%q %s", ok, len(data), id, rec.Body)
	}
	// The order of the parts does not matter.
	if _, id, data, ok := upload(t, h, 0, func(mw *multipart.Writer) {
		file("file", 5)(mw)
		_ = mw.WriteField("yearbook_id", bookID)
	}); !ok || len(data) != 5 || id != bookID {
		t.Fatalf("file before yearbook_id: ok=%v len=%d id=%q", ok, len(data), id)
	}
	if rec, _, _, ok := upload(t, h, 0, withBook(file("file", 101))); ok || rec.Code != 413 || !strings.Contains(rec.Body.String(), "ERROR_TOO_LARGE") {
		t.Fatalf("101 bytes: ok=%v %d %s", ok, rec.Code, rec.Body)
	}
	if rec, _, _, ok := upload(t, h, 150, withBook(file("file", 1000))); ok || rec.Code != 413 { // the request body cap trips first
		t.Fatalf("body cap: ok=%v %d %s", ok, rec.Code, rec.Body)
	}
	for name, build := range map[string]func(*multipart.Writer){
		"wrong file field name": withBook(file("other", 10)),
		"no yearbook_id":        file("file", 10),
		"empty yearbook_id":     func(mw *multipart.Writer) { _ = mw.WriteField("yearbook_id", ""); file("file", 10)(mw) },
		"yearbook_id too long": func(mw *multipart.Writer) {
			_ = mw.WriteField("yearbook_id", strings.Repeat("A", 65))
			file("file", 10)(mw)
		},
		"lower-case yearbook_id": func(mw *multipart.Writer) {
			_ = mw.WriteField("yearbook_id", strings.ToLower(bookID))
			file("file", 10)(mw)
		},
		"yearbook_id with I":     func(mw *multipart.Writer) { _ = mw.WriteField("yearbook_id", "I"+bookID[1:]); file("file", 10)(mw) },
		"yearbook_id padded":     func(mw *multipart.Writer) { _ = mw.WriteField("yearbook_id", bookID+" "); file("file", 10)(mw) },
		"empty body":             func(*multipart.Writer) {},
		"only yearbook_id field": withBook(),
	} {
		if rec, _, _, ok := upload(t, h, 0, build); ok || rec.Code != 400 || !strings.Contains(rec.Body.String(), `"ERROR_PARAM"`) {
			t.Fatalf("%s: ok=%v %d %s", name, ok, rec.Code, rec.Body)
		}
	}

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/media/create", strings.NewReader(`{"file":"x"}`))
	r.Header.Set("Content-Type", "application/json")
	if _, _, ok := h.readForm(rec, r); ok || rec.Code != 400 || !strings.Contains(rec.Body.String(), `"ERROR_PARAM"`) {
		t.Fatalf("not multipart: ok=%v %d %s", ok, rec.Code, rec.Body)
	}
}

// deadlineWriter records the deadlines a handler asks for, like a real connection would apply them.
type deadlineWriter struct {
	http.ResponseWriter
	read, write time.Time
}

func (d *deadlineWriter) SetReadDeadline(t time.Time) error  { d.read = t; return nil }
func (d *deadlineWriter) SetWriteDeadline(t time.Time) error { d.write = t; return nil }

// The route timeout must fit the connection's write deadline: the server's own WriteTimeout (30 s) would otherwise cut the
// answer of a route that is allowed to run longer.
func TestSlowRouteWriteDeadlineFitsTimeout(t *testing.T) {
	for name, c := range map[string]struct {
		timeout, read time.Duration
	}{
		"create":      {uploadRouteTimeout, uploadReadTimeout},
		"get-content": {contentRouteTimeout, 0},
	} {
		w := &deadlineWriter{ResponseWriter: httptest.NewRecorder()}
		var ctxDeadline time.Time
		slowRoute(c.timeout, c.read, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			ctxDeadline, _ = r.Context().Deadline()
		})).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if c.timeout <= httpx.NewServer("", nil).WriteTimeout {
			t.Errorf("%s: the test only means something for a route longer than the server's WriteTimeout", name)
		}
		if ctxDeadline.IsZero() || w.write.Before(ctxDeadline) {
			t.Errorf("%s: write deadline %v is before the request deadline %v", name, w.write, ctxDeadline)
		}
		if got := time.Until(w.write); got < c.timeout || got > c.timeout+writeSlack {
			t.Errorf("%s: write deadline in %v, want between %v and %v", name, got, c.timeout, c.timeout+writeSlack)
		}
		if (c.read == 0) != w.read.IsZero() || (c.read > 0 && time.Until(w.read) < c.read-time.Second) {
			t.Errorf("%s: read deadline %v for read=%v", name, w.read, c.read)
		}
	}
	slowRoute(time.Minute, time.Minute, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil)) // a writer without deadline support is fine
}

// A real server whose WriteTimeout is shorter than the handler: the response only arrives because slowRoute moves the write
// deadline (and fails without it).
func TestSlowRouteSurvivesServerWriteTimeout(t *testing.T) {
	slow := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, "done")
	})
	get := func(h http.Handler) (string, error) {
		srv := httptest.NewUnstartedServer(h)
		srv.Config.WriteTimeout = 100 * time.Millisecond
		srv.Start()
		defer srv.Close()
		res, err := http.Get(srv.URL)
		if err != nil {
			return "", err
		}
		defer func() { _ = res.Body.Close() }()
		b, err := io.ReadAll(res.Body)
		return string(b), err
	}
	if got, err := get(slow); err == nil {
		t.Fatalf("control: the server's WriteTimeout should cut the slow handler off, got %q", got)
	}
	if got, err := get(slowRoute(5*time.Second, 0, slow)); err != nil || got != "done" {
		t.Fatalf("with slowRoute: %q, %v", got, err)
	}
}

// newCapHandler is a Handler with the upload caps of NewHandler and a shared pool of pool slots, and no service.
func newCapHandler(pool int) *Handler {
	return &Handler{inflight: make(chan struct{}, pool), logger: nopLogger(), perUser: map[uint64]int{}, perClient: map[string]int{}, clientCap: maxUploadsPerClient}
}

func TestCreateBusyWhenTooManyInFlight(t *testing.T) {
	h := newCapHandler(1)
	h.inflight <- struct{}{} // one upload already running
	rec := httptest.NewRecorder()
	h.create(rec, httptest.NewRequest("POST", "/api/media/create", nil).WithContext(auth.WithUser(context.Background(), &auth.User{InternalID: 1, EmailVerified: true})))
	if rec.Code != 503 || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), `"ERROR_BUSY"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

// T-102: the third upload of one user is 429 with Retry-After 5 before anything else (the shared pool is not touched), another
// user is not counted, and the count is given back on every path.
func TestCreatePerUserCap(t *testing.T) {
	h := newCapHandler(8)
	call := func(id uint64) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.create(rec, httptest.NewRequest("POST", "/api/media/create", nil).WithContext(auth.WithUser(context.Background(), &auth.User{InternalID: id, EmailVerified: true})))
		return rec
	}
	for range maxUploadsPerUser {
		if !take(&h.userMu, h.perUser, 1, maxUploadsPerUser) {
			t.Fatal("two uploads per user must be allowed")
		}
	}
	rec := call(1)
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "5" || !strings.Contains(rec.Body.String(), `"ERROR_RATE_LIMITED"`) || len(h.inflight) != 0 {
		t.Fatalf("third: %d %q %s inflight=%d", rec.Code, rec.Header().Get("Retry-After"), rec.Body, len(h.inflight))
	}
	if rec := call(2); rec.Code == 429 { // a different user (the body is empty, so it fails on the form, not on the cap)
		t.Fatalf("other user: %d", rec.Code)
	}
	for range maxUploadsPerUser {
		release(&h.userMu, h.perUser, 1)
	}
	if len(h.perUser) != 0 {
		t.Fatalf("counts left: %v", h.perUser)
	}
}

// T-110: an unverified account is 403 ERROR_EMAIL_NOT_VERIFIED before any slot is taken or a body byte is read; the fifth upload
// from one client key is 429 with Retry-After 5 whoever the user is, and the counts are given back.
func TestCreateNeedsVerifiedEmailAndHasPerClientCap(t *testing.T) {
	h := newCapHandler(8)
	call := func(id uint64, verified bool) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/media/create", nil)
		req.RemoteAddr = "192.0.2.7:1234"
		h.create(rec, req.WithContext(auth.WithUser(context.Background(), &auth.User{InternalID: id, EmailVerified: verified})))
		return rec
	}
	h.inflight <- struct{}{}
	h.inflight <- struct{}{}
	rec := call(1, false)
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), `"ERROR_EMAIL_NOT_VERIFIED"`) || len(h.perUser) != 0 || len(h.perClient) != 0 || len(h.inflight) != 2 {
		t.Fatalf("unverified: %d %s counts=%v %v inflight=%d", rec.Code, rec.Body, h.perUser, h.perClient, len(h.inflight))
	}
	for range maxUploadsPerClient { // four uploads of four different users are in progress behind the same address
		if !take(&h.userMu, h.perClient, "192.0.2.7", maxUploadsPerClient) {
			t.Fatal("four uploads per client must be allowed")
		}
	}
	rec = call(9, true)
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "5" || len(h.perUser) != 0 {
		t.Fatalf("fifth from one client: %d %q %s user counts=%v", rec.Code, rec.Header().Get("Retry-After"), rec.Body, h.perUser)
	}
	release(&h.userMu, h.perClient, "192.0.2.7")
	if rec = call(9, true); rec.Code == 429 || len(h.perUser) != 0 || h.perClient["192.0.2.7"] != maxUploadsPerClient-1 { // the empty body fails on the form, and gives the counts back
		t.Fatalf("after a release: %d %s counts=%v %v", rec.Code, rec.Body, h.perUser, h.perClient)
	}
}

// The pace the owner route shares with the public route lets a 10 MiB photo through over a 2 Mbit/s link.
func TestPhoneUploadPaceAllowsSlowLink(t *testing.T) {
	p := httpx.PhoneUploadPace
	const rate = 256 << 10 // 2 Mbit/s in bytes per second
	if d := time.Duration(10<<20) * time.Second / rate; d >= p.Total || rate <= p.MinRate {
		t.Fatalf("10 MiB takes %v (total %v), rate %d vs minimum %d", d, p.Total, rate, p.MinRate)
	}
	if p.Total != uploadReadTimeout {
		t.Fatalf("pace total %v differs from the route's read time %v", p.Total, uploadReadTimeout)
	}
}

// Audit M3 (T-100): list, content and delete answer ERROR_PARAM for an id that is not a canonical ULID before the service is
// called. The handler has no service here, so reaching it would panic.
func TestIDEndpointsRefuseNonCanonicalIDs(t *testing.T) {
	h := &Handler{logger: nopLogger()}
	user := auth.WithUser(context.Background(), &auth.User{InternalID: 1})
	for _, bad := range []string{strings.ToLower(bookID), bookID[:25], bookID + "A", "O" + bookID[1:], "..%2F" + bookID[5:], "%20" + bookID[1:]} {
		for name, rec := range map[string]*httptest.ResponseRecorder{
			"list":    do(h.list, httptest.NewRequest("GET", "/api/media/get-list?yearbook_id="+bad, nil).WithContext(user)),
			"content": do(h.content, httptest.NewRequest("GET", "/api/media/get-content?id="+bad, nil).WithContext(user)),
			"delete":  do(h.delete, httptest.NewRequest("POST", "/api/media/delete", strings.NewReader(`{"id":"`+bad+`"}`)).WithContext(user)),
		} {
			if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"ERROR_PARAM"`) {
				t.Errorf("%s with %q: %d %s", name, bad, rec.Code, rec.Body)
			}
		}
	}
}

func do(f http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	f(rec, r)
	return rec
}
