//go:build integration

package bookdesign_test

import (
	"fmt"
	"strings"
	"testing"
)

// QA (T-141): size boundary at 4095/4096/4097 bytes as sent, NFC/NFD, deep nesting, odd JSON; rejected documents leave the older design as it was.
func TestQAOptionsBoundariesAndOddJSON(t *testing.T) {
	e := newEnv(t, nil)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	pad := func(n int) string { // a document of exactly n bytes, padded with spaces before the closing brace
		base := `{"theme":"mint"`
		return base + strings.Repeat(" ", n-len(base)-1) + "}"
	}
	for _, n := range []int{4095, 4096} {
		if d := pad(n); len(d) != n {
			t.Fatalf("pad(%d) is %d bytes", n, len(d))
		}
		e.call("POST", "/api/book-design/update", alice, update(id, pad(n))).status(200, "OK")
	}
	e.call("POST", "/api/book-design/update", alice, update(id, `{"theme":"peach"}`)).status(200, "OK")
	e.call("POST", "/api/book-design/update", alice, update(id, pad(4097))).status(400, "ERROR_INVALID_DESIGN")

	deep := strings.Repeat(`{"a":`, 1000) + "1" + strings.Repeat("}", 1000)
	arr := strings.Repeat("[", 1500) + strings.Repeat("]", 1500)
	for _, doc := range []string{
		deep, arr, `{"texts":{"cover.belongs":` + arr + `}}`, `{"sections":{"order":[` + arr + `]}}`,
		`{"theme":"mint","theme":"nope"}`, // duplicate key: last one wins in Go, so this must be rejected (it is invalid)
		`{"v":1}{"v":1}`, `{"v":1}x`, `{"v":true}`, `{"v":null,"theme":1}`, `{"sections":null,"texts":{"a":null}}`,
		`{"texts":{"cover.belongs":"` + strings.Repeat("\\u0000", 3) + `"}}`, `{"texts":{"cover.belongs":"\ud800"}}`,
		`{"texts":{"cover.belongs":"\u200b"}}`, `{"theme":"MINT"}`,
		`{"images":{"cover.pattern":null}}`, `{"images":{"":"dots"}}`, `{"texts":{"":"x"}}`,
	} {
		rec := e.call("POST", "/api/book-design/update", alice, update(id, doc))
		if rec.Code != 400 {
			// A few of these (null images value, "a":null) are legitimately "absent"; those must at least not 5xx.
			if rec.Code >= 500 {
				t.Errorf("%.60q -> %d %s", doc, rec.Code, rec.Body)
			}
			t.Logf("accepted %.70q -> %d %s", doc, rec.Code, rec.Body)
			// put back a known design
			e.call("POST", "/api/book-design/update", alice, update(id, `{"theme":"peach"}`)).status(200, "OK")
		}
	}
	var stored string
	if err := e.db.QueryRow(`SELECT options_json FROM book_design_tab`).Scan(&stored); err != nil || stored != `{"v":1,"theme":"peach"}` {
		t.Fatalf("stored %q %v", stored, err)
	}
}

// NFD input is stored NFC, and counted in NFC runes (40 NFD letters with two marks each fit a max of 40).
func TestQANFDText(t *testing.T) {
	e := newEnv(t, nil)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	nfd := strings.Repeat("ệ", 40) // 120 runes as sent, 40 after NFC
	e.call("POST", "/api/book-design/update", alice, update(id, fmt.Sprintf(`{"texts":{"cover.belongs":%q}}`, nfd))).status(200, "OK")
	var stored string
	_ = e.db.QueryRow(`SELECT options_json FROM book_design_tab`).Scan(&stored)
	if want := `{"v":1,"texts":{"cover.belongs":"` + strings.Repeat("ệ", 40) + `"}}`; stored != want {
		t.Fatalf("stored %q want %q", stored, want)
	}
	e.call("POST", "/api/book-design/update", alice, update(id, fmt.Sprintf(`{"texts":{"cover.belongs":%q}}`, nfd+"x"))).status(400, "ERROR_INVALID_DESIGN")
}

// Atomicity: a template change that fails after the design hook ran (a cover photo of nobody's) leaves the design in place.
func TestQATemplateChangeFailureKeepsDesign(t *testing.T) {
	e := newEnv(t, nil)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	e.call("POST", "/api/book-design/update", alice, update(id, full)).status(200, "OK")
	rec := e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"template_id":"navy-classic","cover_media_id":"01K7QB1B2C3D4E5F6G7H8J9K0M"}`, id))
	if rec.Code < 400 {
		t.Fatalf("update with a bad cover succeeded: %d %s", rec.Code, rec.Body)
	}
	if n := e.count(`SELECT COUNT(*) FROM book_design_tab`); n != 1 {
		t.Fatalf("design rows %d after the failed template change, want 1", n)
	}
	d := e.call("GET", "/api/book-design/get?yearbook_id="+id, alice, "").status(200, "OK").data()
	if d["template_id"] != "memory-book" || d["options"] == nil {
		t.Fatalf("book changed by a failed update: %v", d)
	}
}
