//go:build integration

package booklayout_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/booklayout"
	"github.com/danyaa666/smemories/internal/metrics"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/ulid"
)

// E09 T-179: abuse limits of the book-layout routes (docs/stage2-threat-model.md, docs/layout-storage-budget.md).

const (
	saveCap  = booklayout.MaxBytes + 4<<10 // the body cap of save; the other write routes take 8 KiB
	smallCap = 8 << 10
)

type route struct {
	name, method, path string
	body               func(book string) string
	cap                int  // request body cap in bytes; 0 for a GET
	limited            bool // takes the per-user limit (Burst per 10 s and the per-minute number)
}

func routes() []route {
	vb := func(book string) string { return versionBody(book, unknownID) }
	return []route{
		{"get", "GET", "", nil, 0, false},
		{"save", "POST", "/api/book-layout/save", func(b string) string { return save(b, 0, doc()) }, saveCap, true},
		{"reset", "POST", "/api/book-layout/reset", reset, smallCap, false},
		{"version-list", "POST", "/api/book-layout/version-list", reset, smallCap, true},
		{"version-get", "POST", "/api/book-layout/version-get", vb, smallCap, true},
		{"restore", "POST", "/api/book-layout/restore", func(b string) string { return restore(b, unknownID, 0) }, smallCap, true},
		{"template-list", "POST", "/api/book-layout/template-list", func(b string) string { return tplList(b, "") }, smallCap, true},
		{"template-create", "POST", "/api/book-layout/template-create", func(b string) string { return tplCreate(b, "cover", "x") }, smallCap, true},
		{"template-delete", "POST", "/api/book-layout/template-delete", func(string) string { return tplDelete(unknownID) }, smallCap, true},
		{"apply-template", "POST", "/api/book-layout/apply-template", func(b string) string { return applyBody(b, 0, "cover", "shipped", "cover-sunny") }, smallCap, true},
	}
}

func (r route) url(book string) string {
	if r.method == "GET" {
		return get(book)
	}
	return r.path
}

// padded is a JSON object of exactly n bytes whose last member is a string of blanks: valid up to the cap, one byte over it past the cap.
func padded(book string, n int) string {
	head := fmt.Sprintf(`{"yearbook_id":%q,"pad":"`, book)
	return head + strings.Repeat("a", n-len(head)-2) + `"}`
}

// Req 4: for every route, an anonymous call is 401 before a byte of the body is parsed, a body one byte over the route's cap is 413
// ERROR_TOO_LARGE before it is decoded, and the routes that take the per-user limit answer 429 with Retry-After once the burst is spent,
// for that user only.
func TestRoutesRefuseAnonymousOversizeAndTooMany(t *testing.T) {
	now := func() time.Time { return t0 }
	e := newEnv(t, ratelimit.NewMemoryFactory(now), 1000) // the burst (20 per 10 s) is what is under test
	owner := e.register("owner@example.com")
	book := e.book(owner, "memory-book")
	hostile := func(r route) string {
		if r.cap == 0 {
			return ""
		}
		return strings.Repeat("{", 2<<20) // 2 MiB that is not JSON: if anything parsed it first, it would be a 400
	}
	for _, r := range routes() {
		// anonymous: 401 whatever the body is
		e.call(r.method, r.url(book), user{}, hostile(r)).status(401, "ERROR_UNAUTHORIZED")
		if r.cap > 0 {
			// the cap: exactly at it the body is read (and refused by the route's own rules, never as too large); one byte over it is 413
			over := e.call(r.method, r.url(book), owner, padded(book, r.cap+1))
			over.status(413, "ERROR_TOO_LARGE")
			if got := e.call(r.method, r.url(book), owner, padded(book, r.cap)); got.Code == 413 {
				t.Errorf("%s: a body of exactly %d bytes is refused as too large", r.name, r.cap)
			}
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM book_layout_tab`); n != 0 {
		t.Errorf("%d layout rows after refused calls", n)
	}

	// the per-user limit: a user per route, so the burst of one route does not hide another's
	for i, r := range routes() {
		who := e.register(fmt.Sprintf("limit%d@example.com", i))
		own := e.book(who, "memory-book")
		call := func() resp { return e.call(r.method, r.url(own), who, r.body0(own)) }
		for n := range booklayout.Burst + 5 {
			got := call()
			switch {
			case !r.limited && got.Code == 429:
				t.Fatalf("%s: call %d is rate limited, the route is documented as not limited", r.name, n+1)
			case r.limited && n < booklayout.Burst && got.Code == 429:
				t.Fatalf("%s: call %d is rate limited before the burst of %d", r.name, n+1, booklayout.Burst)
			case r.limited && n >= booklayout.Burst:
				got.status(429, "ERROR_RATE_LIMITED")
				if ra := got.Header().Get("Retry-After"); ra == "" || ra == "0" {
					t.Errorf("%s: Retry-After %q", r.name, ra)
				}
			}
		}
		if r.limited {
			// per user: someone else is not slowed down by it
			other := e.register(fmt.Sprintf("other%d@example.com", i))
			ob := e.book(other, "memory-book")
			if got := e.call(r.method, r.url(ob), other, r.body0(ob)); got.Code == 429 {
				t.Errorf("%s: the limit is shared between users", r.name)
			}
		}
	}
}

// body0 is the body of a valid first call.
func (r route) body0(book string) string {
	if r.body == nil {
		return ""
	}
	return r.body(book)
}

// Req 2: hostile bodies through the real route fail fast with the right code and bounded allocation, and store nothing.
func TestHostileBodiesFailFast(t *testing.T) {
	const (
		maxTime  = 200 * time.Millisecond
		maxAlloc = 2 << 20 // a refusal before decoding: the recorder's copy of the body and the request, nothing of the document (measured: 0.3 MB)
		// 43 000 empty items is the most a 128 KiB body holds: it is decoded (about 6 MB), looked at twice (the media ids, the validator) and
		// refused for having too many items (measured: 12 MB). The ceiling is 24 MiB, about 190 times the body.
		maxDecodeAlloc = 24 << 20
	)
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	tiny := func(n int) string {
		return `{"v":1,"pages":{"cover":{"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, n), ",") + `]}}}`
	}
	cases := []struct {
		name, body string
		code       int
		status     string
		problem    string // first problem code of an ERROR_INVALID_LAYOUT
		ceiling    uint64
	}{
		{"2 MB body", save(id, 0, `"`+strings.Repeat("a", 2<<20)+`"`), 413, "ERROR_TOO_LARGE", "", maxAlloc},
		{"100 000 levels of arrays", save(id, 0, strings.Repeat("[", 100000)), 400, "ERROR_INVALID_LAYOUT", booklayout.CodeTooDeep, maxAlloc},
		{"60 000 levels, closed", save(id, 0, strings.Repeat("[", 60000)+strings.Repeat("]", 60000)), 400, "ERROR_INVALID_LAYOUT", booklayout.CodeTooDeep, maxAlloc},
		{"17 levels inside a page", save(id, 0, `{"v":1,"pages":{"cover":`+strings.Repeat(`{"a":`, 15)+`1`+strings.Repeat("}", 15)+`}}`), 400, "ERROR_INVALID_LAYOUT", booklayout.CodeTooDeep, maxAlloc},
		{"200 000 tiny items", save(id, 0, tiny(200000)), 413, "ERROR_TOO_LARGE", "", maxAlloc},
		{"43 000 tiny items", save(id, 0, tiny(43000)), 400, "ERROR_INVALID_LAYOUT", booklayout.CodeTooMany, maxDecodeAlloc},
	}
	for _, c := range cases {
		var rec resp
		var a, b runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&a)
		start := time.Now()
		rec = e.call("POST", "/api/book-layout/save", alice, c.body)
		took := time.Since(start)
		runtime.ReadMemStats(&b)
		alloc := b.TotalAlloc - a.TotalAlloc
		rec.status(c.code, c.status)
		if c.problem != "" {
			var probs struct {
				Data struct{ Problems []booklayout.Problem }
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &probs)
			if len(probs.Data.Problems) == 0 || probs.Data.Problems[0].Code != c.problem {
				t.Errorf("%s: problems %v, want %s", c.name, probs.Data.Problems, c.problem)
			}
		}
		if took > maxTime || alloc > c.ceiling {
			t.Errorf("%s: took %v and allocated %d bytes (ceilings %v and %d)", c.name, took, alloc, maxTime, c.ceiling)
		}
		t.Logf("%-26s %d %-22s %8v %9d bytes allocated", c.name, rec.Code, c.status, took.Round(time.Microsecond), alloc)
	}
	if n := e.count(`SELECT COUNT(*) FROM book_layout_tab`) + e.count(`SELECT COUNT(*) FROM book_layout_version_tab`); n != 0 {
		t.Errorf("%d rows after hostile bodies", n)
	}
	// the counters saw them: two of the refusals were sizes, four were documents
	if e.counted("too_large") != 2 || e.counted("invalid") != 4 {
		t.Errorf("counters too_large=%d invalid=%d, want 2 and 4", e.counted("too_large"), e.counted("invalid"))
	}
}

// lockedBuffer is a log sink that parallel requests can write to.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// Req 5: nothing a student wrote (text items, template names, image ids) reaches the application log or the Prometheus output, and a
// problem list holds paths and codes only. Create, apply (shipped and own, also failing), save, restore and a rejected save run with
// sentinel strings; both outputs are searched, case-insensitively, afterwards.
func TestNoLayoutContentInLogsMetricsOrProblemLists(t *testing.T) {
	logs := &lockedBuffer{}
	log := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mt := metrics.New()
	e := newEnvObserved(t, nil, 0, log, mt)
	alice := e.register("alice@example.com")
	book := e.book(alice, "memory-book")
	img := e.image(book, "design") // an image id that is the owner's: it is in the document, so it must not be in the logs
	const (
		sText   = "SentinelTextZq7"
		sName   = "SentinelNameZq7"
		sKey    = "Sentinel Key Zq7" // a member name with a blank is not echoed, only short plain names are
		sBadTxt = "SentinelBadZq7"
	)
	sentinels := []string{sText, sName, sKey, sBadTxt, img, unknownID, "Ệ" + sText}
	text := func(id, s string) string {
		return `{"id":"` + id + `","t":"text","text":"` + s + `","x":100,"w":400,"h":120,"cy":850,"r":0}`
	}
	good := func(extra string) string {
		return `{"v":1,"pages":{"cover":{"items":[` + text("a1b2c3", "Ệ"+sText) + `,{"id":"i1b2c3","t":"image","media":"` + img + `","x":0,"w":300,"h":300,"cy":300,"r":0}` + extra + `]}}}`
	}
	var problems []string // bodies of the refusals: only these must be free of every sentinel
	refused := func(r resp, code int, status string) {
		t.Helper()
		r.status(code, status)
		problems = append(problems, r.Body.String())
	}

	// save, then a second save an hour later (a snapshot), then create, apply and restore
	e.call("POST", "/api/book-layout/save", alice, save(book, 0, good(""))).status(200, "OK")
	e.now = e.now.Add(time.Hour)
	e.call("POST", "/api/book-layout/save", alice, save(book, 1, good(","+text("d4e5f6", sName)))).status(200, "OK")
	tpl := e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", "Ệ"+sName)).status(200, "OK").data()["id"].(string)
	vs := e.versions(alice, book)
	e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, 2, "cover", "shipped", "cover-sunny")).status(200, "OK")
	e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, 3, "cover", "own", tpl)).status(200, "OK")
	e.call("POST", "/api/book-layout/restore", alice, restore(book, vs[len(vs)-1].ID, 4)).status(200, "OK")
	e.call("POST", "/api/book-layout/version-get", alice, versionBody(book, vs[0].ID)).status(200, "OK")
	e.call("GET", get(book), alice, "").status(200, "OK")

	// failing calls: a stale apply (the 409 carries the owner's own document, so it is not searched), a template of the wrong kind,
	// an unknown template, a restore on a stale version
	e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, 1, "cover", "own", tpl)).status(409, "ERROR_VERSION_CONFLICT")
	refused(e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, 5, "back", "own", tpl)), 400, "ERROR_PARAM")
	refused(e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, 5, "cover", "own", unknownID)), 404, "ERROR_NOT_FOUND")
	e.call("POST", "/api/book-layout/restore", alice, restore(book, vs[0].ID, 1)).status(409, "ERROR_VERSION_CONFLICT")
	refused(e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", sName+"\u202e")), 400, "ERROR_PARAM")
	refused(e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "poster", sName)), 400, "ERROR_PARAM")

	// rejected documents: control character in a text, too long a text, an unknown member named like a sentinel, someone else's image
	cur := 5
	for name, bad := range map[string]string{
		"bad text":     `{"v":1,"pages":{"cover":{"items":[` + text("a1b2c3", sBadTxt+`\u0000`) + `]}}}`,
		"bidi":         `{"v":1,"pages":{"cover":{"items":[` + text("a1b2c3", sBadTxt+`\u202e`) + `]}}}`,
		"too long":     `{"v":1,"pages":{"cover":{"items":[` + text("a1b2c3", strings.Repeat(sBadTxt, 50)) + `]}}}`,
		"unknown key":  `{"v":1,"pages":{"cover":{"items":[{"id":"a1b2c3","t":"text","text":"` + sText + `","` + sKey + `":1,"x":1,"w":400,"h":120,"cy":8}]}}}`,
		"unknown page": `{"v":1,"pages":{"` + sKey + `":{}}}`,
		"foreign img":  `{"v":1,"pages":{"cover":{"items":[{"id":"a1b2c3","t":"image","media":"` + unknownID + `","x":0,"w":300,"h":300,"cy":300}]}}}`,
	} {
		r := e.call("POST", "/api/book-layout/save", alice, save(book, cur, bad))
		if r.Code != 400 {
			t.Fatalf("%s: %d %.300s", name, r.Code, r.Body)
		}
		refused(r, 400, "ERROR_INVALID_LAYOUT")
		var pl struct {
			Data struct{ Problems []booklayout.Problem }
		}
		_ = json.Unmarshal(r.Body.Bytes(), &pl)
		if len(pl.Data.Problems) == 0 {
			t.Errorf("%s: no problems listed", name)
		}
		for _, p := range pl.Data.Problems {
			if strings.ContainsAny(p.Code, " \"") || len(p.Code) > 20 {
				t.Errorf("%s: problem code %q", name, p.Code)
			}
		}
	}

	metricsText := httptest.NewRecorder()
	mt.Handler().ServeHTTP(metricsText, httptest.NewRequest("GET", "/metrics", nil))
	out := map[string]string{"log": logs.String(), "metrics": metricsText.Body.String(), "problem lists": strings.Join(problems, "\n")}
	// the capture is real: the lines this test is about are there
	for _, want := range []string{"layout restored", "page template applied", `"route":"POST /api/book-layout/save"`} {
		if !strings.Contains(out["log"], want) {
			t.Errorf("the log has no %q: the capture proves nothing", want)
		}
	}
	for _, want := range []string{"smem_layout_saves_total", "smem_layout_restores_total", "smem_page_template_applies_total"} {
		if !strings.Contains(out["metrics"], want) {
			t.Errorf("the metrics have no %s", want)
		}
	}
	for where, body := range out {
		low := strings.ToLower(body)
		for _, s := range sentinels {
			if strings.Contains(low, strings.ToLower(s)) {
				t.Errorf("%s contains %q", where, s)
			}
		}
	}
	if !ulid.Valid(img) {
		t.Fatal("the image id is not an id")
	}
}

// Req 6: a template name is text: control, bidi and separator characters are refused, a lone surrogate too, the composed and decomposed
// forms of a Vietnamese name are the same stored name and the limit counts stored characters.
func TestTemplateNamesAreText(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	book := e.book(alice, "memory-book")
	e.call("POST", "/api/book-layout/save", alice, save(book, 0, doc(textAt("a1b2c3")))).status(200, "OK")
	create := func(rawName string) resp { // rawName is a JSON string body, so "\ud800" reaches the server as written
		return e.call("POST", "/api/book-layout/template-create", alice, fmt.Sprintf(`{"yearbook_id":%q,"kind":"cover","name":%s}`, book, rawName))
	}
	for name, bad := range map[string]string{
		"NUL": `"a\u0000b"`, "escape": `"a\u001bb"`, "tab": `"a\tb"`, "newline": `"a\nb"`, "bidi override": `"a\u202eb"`, "bidi isolate": `"\u2066a\u2069"`,
		"zero width": `"a\u200bb"`, "line separator": `"a\u2028b"`, "lone high surrogate": `"a\ud800b"`, "lone low surrogate": `"a\udc00b"`, "BOM": `"\ufeffa"`,
	} {
		if r := create(bad); r.Code != 400 {
			t.Errorf("%s: %d %.200s", name, r.Code, r.Body)
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM book_template_tab`); n != 0 {
		t.Fatalf("%d templates stored from refused names", n)
	}
	// composed and decomposed (macOS Telex) become one stored name
	composed, decomposed := "Bìa Việt", "Bìa Việt"
	if composed == decomposed {
		t.Fatal("the two forms are the same bytes")
	}
	a := create(`"`+composed+`"`).status(200, "OK").data()
	b := create(`"`+decomposed+`"`).status(200, "OK").data()
	if a["name"] != composed || b["name"] != composed {
		t.Errorf("stored names %q and %q, want %q", a["name"], b["name"], composed)
	}
	var stored []string
	rows, err := e.db.Query(`SELECT name FROM book_template_tab`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		stored = append(stored, n)
	}
	if len(stored) != 2 || stored[0] != composed || stored[1] != composed {
		t.Errorf("stored in the table: %q", stored)
	}
	// 60 letters fit in either form, 61 do not
	e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", strings.Repeat("ệ", 60))).status(200, "OK")
	e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", strings.Repeat("ệ", 61))).status(400, "ERROR_PARAM")
}
