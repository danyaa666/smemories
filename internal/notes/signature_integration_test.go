//go:build integration

package notes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/signature"
)

// sigPart is the multipart "signature" field.
func sigPart(s string) part { return text("signature", s) }

const (
	oddSig   = " { \"s\" : [[ 10 , 20 ,\n 500, 300 ], [ 5,5,  900 ,100]] , \"h\":400,\"v\":1, \"w\":1000 } "
	canonSig = `{"v":1,"w":1000,"h":400,"s":[[10,20,500,300],[5,5,900,100]]}`
)

// storedSig returns signature_json of the note (NULL = "<null>").
func (e *env) storedSig(noteID string) string {
	e.t.Helper()
	var s sql.NullString
	if err := e.db.QueryRow(`SELECT signature_json FROM note_tab WHERE public_id = ?`, noteID).Scan(&s); err != nil {
		e.t.Fatal(err)
	}
	if !s.Valid {
		return "<null>"
	}
	return s.String
}

func (e *env) enableSignatures() { e.ns.signatureOn = func(string) bool { return true } }

func sigOf(it map[string]any) any { return it["signature"] }

// AC2: a template without the switch refuses a signature (and says so on the form), and nothing is stored.
func TestSignatureNeedsTheTemplateSwitch(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	info := e.lookup(l.token).status(200, "").data()["signature"].(map[string]any)
	want := map[string]any{"enabled": false, "w": 1000.0, "h": 400.0, "max_strokes": 12.0, "max_points": 600.0}
	if fmt.Sprint(info) != fmt.Sprint(want) {
		t.Fatalf("signature info = %v, want %v", info, want)
	}
	e.post(l.token, answers("Linh", "hi"), sigPart(canonSig)).status(400, "ERROR_PARAM")
	// An oversize part is refused while the body is read, before the book (and its template) is looked up, so its code does not
	// depend on the template: documented in docs/note-fields.md and api/openapi.yaml (T-097).
	big := canonSig + strings.Repeat(" ", signature.MaxInput)
	e.post(l.token, answers("Linh", "hi"), sigPart(big)).status(400, "ERROR_INVALID_SIGNATURE")
	if n := e.count(`SELECT COUNT(*) FROM note_tab`); n != 0 {
		t.Fatalf("%d notes stored", n)
	}
	e.post(l.token, answers("Linh", "hi")).status(200, "") // without it the note is fine
}

// AC2: submit with and without a signature; the stored string is the canonical re-serialisation; the owner reads it back unchanged in
// the inbox in every status; a note without one reads null.
func TestSignatureRoundTrip(t *testing.T) {
	e := newEnv(t)
	e.enableSignatures()
	l := e.newOpenLink()
	if e.lookup(l.token).status(200, "").data()["signature"].(map[string]any)["enabled"] != true {
		t.Fatal("the form does not offer the pad")
	}
	signed := e.post(l.token, answers("Linh", "hi"), sigPart(oddSig)).status(200, "").data()["id"].(string)
	plain := e.post(l.token, answers("Mai", "hello")).status(200, "").data()["id"].(string)
	if got := e.storedSig(signed); got != canonSig {
		t.Fatalf("stored %q, want the canonical form", got)
	}
	if got := e.storedSig(plain); got != "<null>" {
		t.Fatalf("a note without a signature stored %q", got)
	}
	var want any
	_ = json.Unmarshal([]byte(canonSig), &want)
	for _, status := range []string{"pending", "approved", "hidden"} {
		if status != "pending" {
			e.setStatus(l.owner, l.book, []string{signed}, status).status(200, "")
		}
		items := e.noteList(l.owner, l.book, "").status(200, "").data()["items"].([]any)
		seen := 0
		for _, it := range items {
			m := it.(map[string]any)
			switch m["id"] {
			case signed:
				seen++
				if fmt.Sprint(sigOf(m)) != fmt.Sprint(want) {
					t.Errorf("%s: signature = %v, want %v", status, sigOf(m), want)
				}
			case plain:
				seen++
				if v, ok := m["signature"]; !ok || v != nil {
					t.Errorf("%s: a note without a signature must carry signature:null, got %v (present %v)", status, v, ok)
				}
			}
		}
		if seen != 2 {
			t.Fatalf("%s: saw %d of 2 notes", status, seen)
		}
	}
	if got := e.storedSig(signed); got != canonSig {
		t.Fatalf("the signature changed: %q", got)
	}
}

// AC2: invalid signatures are ERROR_INVALID_SIGNATURE, the message names the rule and never the content, nothing is stored, no photo is
// processed (the signature is validated before the photos), and the limits are not consumed by a request that fails validation.
func TestSignatureRejected(t *testing.T) {
	e := newEnv(t)
	e.enableSignatures()
	l := e.newOpenLink()
	bad := map[string]string{
		"speck":     `{"v":1,"w":1000,"h":400,"s":[[10,10,20,20]]}`,
		"float":     `{"v":1,"w":1000,"h":400,"s":[[10,10,520.5,20]]}`,
		"extra key": `{"v":1,"w":1000,"h":400,"s":[[10,10,520,20]],"x":"SECRET-WORD"}`,
		"range":     `{"v":1,"w":1000,"h":400,"s":[[10,10,5200,20]]}`,
		"markup":    `<svg onload=alert(1)>`,
		"empty":     ``,
		"control":   "{\"v\":1,\x00}",
		"unicode":   `{"v":1,"w":1000,"h":400,"s":[[10,10,520,20]],"n":"Ơ̷̢"}`,
		"too large": `{"v":1,"w":1000,"h":400,"s":[[10,10,520,20]]}` + strings.Repeat(" ", signature.MaxInput),
		"1 MB":      strings.Repeat("A", 1<<20),
	}
	for name, v := range bad {
		r := e.post(l.token, answers("Linh", "hi"), photoPart(jpegOf(t, 1)), sigPart(v)).status(400, "ERROR_INVALID_SIGNATURE")
		if msg, _ := r.json()["error_message"].(string); strings.Contains(msg, "SECRET-WORD") || strings.Contains(msg, "5200") || strings.Contains(msg, "alert") {
			t.Errorf("%s: the message echoes the content: %q", name, msg)
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM note_tab`); n != 0 {
		t.Fatalf("%d notes stored", n)
	}
	if e.st.puts != 0 {
		t.Fatalf("%d objects written: a photo was processed before the signature was validated", e.st.puts)
	}
	e.post(l.token, answers("Linh", "hi"), sigPart(canonSig), sigPart(canonSig)).status(400, "ERROR_PARAM") // duplicate field
	// Text problems still win over the signature's, in the existing order of checks.
	e.post(l.token, text("answers", `{"name":""}`), sigPart(`junk`)).status(400, "ERROR_MISSING_ANSWER")
	// The same link takes a valid one afterwards.
	e.post(l.token, answers("Linh", "hi"), photoPart(jpegOf(t, 2)), sigPart(canonSig)).status(200, "")
}

// AC2: the signature goes with the note when it is deleted (batch-delete) and when the book is deleted.
func TestSignatureLifecycle(t *testing.T) {
	e := newEnv(t)
	e.enableSignatures()
	l := e.newOpenLink()
	a := e.post(l.token, answers("A", "x"), sigPart(canonSig)).status(200, "").data()["id"].(string)
	b := e.post(l.token, answers("B", "y"), sigPart(canonSig)).status(200, "").data()["id"].(string)
	e.setStatus(l.owner, l.book, []string{a, b}, "approved").status(200, "")
	e.reorder(l.owner, l.book, []string{b, a}).status(200, "")
	if e.storedSig(a) != canonSig || e.storedSig(b) != canonSig {
		t.Fatal("moderation changed a signature")
	}
	e.deleteNotes(l.owner, l.book, []string{a}).status(200, "")
	if n := e.count(`SELECT COUNT(*) FROM note_tab WHERE signature_json IS NOT NULL`); n != 1 {
		t.Fatalf("%d signatures after deleting one note, want 1", n)
	}
	e.deleteBook(l.owner, l.book).status(200, "")
	if n := e.count(`SELECT COUNT(*) FROM note_tab WHERE signature_json IS NOT NULL`); n != 0 {
		t.Fatalf("%d signatures left after the book was deleted", n)
	}
}

// The API never returns anything but integers: a damaged stored value (a string where a number belongs, markup) reads as no signature.
func TestSignatureReadIsReparsed(t *testing.T) {
	e := newEnv(t)
	e.enableSignatures()
	l := e.newOpenLink()
	id := e.post(l.token, answers("A", "x"), sigPart(canonSig)).status(200, "").data()["id"].(string)
	for _, damaged := range []string{`{"v":1,"w":1000,"h":400,"s":[["<script>alert(1)</script>",0,500,0]]}`, `<svg/onload=alert(1)>`, `{"v":1,"w":1000,"h":400,"s":[[0,0,5000,0]]}`} {
		if _, err := e.db.ExecContext(context.Background(), `UPDATE note_tab SET signature_json = ? WHERE public_id = ?`, damaged, id); err != nil {
			t.Fatal(err)
		}
		r := e.noteList(l.owner, l.book, "").status(200, "")
		if strings.Contains(r.Body.String(), "script") || strings.Contains(r.Body.String(), "svg") || sigOf(r.data()["items"].([]any)[0].(map[string]any)) != nil {
			t.Fatalf("a damaged value leaked: %s", r.Body.String())
		}
	}
}
