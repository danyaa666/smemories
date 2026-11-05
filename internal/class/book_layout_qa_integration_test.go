//go:build integration

package class

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/ulid"
)

// QA of T-167: the 8 MiB cap rule on a real, full class (60 members), with layouts of the largest size the validator accepts
// (about 125 KiB: 81 text items of 500 three-byte characters) inserted straight into book_layout_tab, and long approved notes inserted
// straight into note_tab. The unit tests prove the rule on synthetic members; this runs the real services and the real handler.

// bigLayout is a valid memory-book layout of just under booklayout.MaxBytes (128 KiB), spread over three pages (40 items per page at most).
func bigLayout(tag string) string {
	text, _ := json.Marshal(strings.Repeat("ế", 500))
	item := func(n int) string {
		return fmt.Sprintf(`{"id":"txt%03d","t":"text","text":%s,"x":0,"w":300,"h":200,"cy":500}`, n, text)
	}
	var pages []string
	n := 0
	for _, p := range []string{"cover", "profile", "back"} {
		var its []string
		for range 27 {
			its = append(its, item(n))
			n++
		}
		pages = append(pages, fmt.Sprintf(`%q:{"items":[%s]}`, p, strings.Join(its, ",")))
	}
	_ = tag
	return `{"v":1,"pages":{` + strings.Join(pages, ",") + `}}`
}

type qaClass struct {
	cls    string
	admin  person
	layout map[int]bool
}

// qaFill makes a class of n students; student i has a layout of the given size when withLayout(i), and notes approved notes of
// noteRunes characters each. Students are numbered in book order.
func (e *env) qaFill(n, notes, noteRunes int, withLayout func(int) bool) qaClass {
	admin := e.user("Admin")
	cls, _ := e.seedClass([]person{admin})
	cid := e.count(`SELECT id FROM class_tab WHERE public_id = ?`, cls)
	now := e.now().UnixMilli()
	big := bigLayout("")
	e.t.Logf("bigLayout is %d bytes", len(big))
	q := qaClass{cls: cls, admin: admin, layout: map[int]bool{}}
	for i := range n {
		u := e.user("S" + strconv.Itoa(i))
		e.exec(`INSERT INTO class_member_tab (public_id, class_id, user_id, role, sort_order, joined_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
			ulid.New(e.now()), cid, u.row, RoleStudent, i, now, now, now)
		book := e.newBook(u)
		e.profile(book, "Student "+strconv.Itoa(i), "")
		e.exec(`UPDATE class_member_tab SET yearbook_id = ? WHERE class_id = ? AND user_id = ?`, e.bookRow(book), cid, u.row)
		e.exec(`UPDATE yearbook_tab SET template_id = 'memory-book' WHERE id = ?`, e.bookRow(book))
		if withLayout(i) {
			e.exec(`INSERT INTO book_layout_tab (yearbook_id, template_id, layout_json, version, created_at, updated_at) VALUES (?, 'memory-book', ?, 1, 1, 1)`, e.bookRow(book), big)
			q.layout[i] = true
		}
		for j := range notes {
			e.note(book, "approved", j+1, strings.Repeat("ế", noteRunes), "")
		}
	}
	return q
}

type qaMember struct {
	MemberID  string          `json:"member_id"`
	Name      string          `json:"name"`
	Linked    bool            `json:"linked"`
	Layout    json.RawMessage `json:"layout"`
	Notes     []any           `json:"notes"`
	Truncated bool            `json:"truncated"`
	Approved  int             `json:"approved_total"`
}

func (e *env) qaRead(q qaClass) (size int, ms []qaMember, cutNotes, cutLayouts []string) {
	e.t.Helper()
	r := e.classBook(q.admin, q.cls).status(200, "OK")
	var out struct {
		Data struct {
			Members          []qaMember `json:"members"`
			Truncated        []string   `json:"truncated"`
			TruncatedLayouts []string   `json:"truncated_layouts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
		e.t.Fatal(err)
	}
	if os.Getenv("QA_DUMP") != "" && len(out.Data.Members) > 0 {
		var raw struct {
			Data struct {
				Members []json.RawMessage `json:"members"`
			} `json:"data"`
		}
		_ = json.Unmarshal(r.Body.Bytes(), &raw)
		for _, k := range []int{0, 1, len(raw.Data.Members) - 1} {
			m := raw.Data.Members[k]
			if len(m) > 600 {
				m = append(m[:300:300], append([]byte(" ...[cut]... "), m[len(m)-300:]...)...)
			}
			e.t.Logf("member %d (%d bytes): %s", k, len(raw.Data.Members[k]), m)
		}
	}
	var students []qaMember // the admin is a member too (not linked, no page): leave them out so indexes are the students'
	for _, m := range out.Data.Members {
		if string(m.Layout) == "null" {
			m.Layout = nil
		}
		if m.Linked {
			students = append(students, m)
		}
	}
	if len(students) > 0 {
		e.t.Logf("first student: layout %d bytes, %d notes", len(students[0].Layout), len(students[0].Notes))
	}
	return r.Body.Len(), students, out.Data.Truncated, out.Data.TruncatedLayouts
}

func TestQA167FullClassOfMaximalLayoutsAndLongNotes(t *testing.T) {
	e := newEnv(t)
	// 60 students, each with a ~125 KiB layout and 40 approved notes of 700 characters (about 2.1 KB each, 84 KB a student).
	q := e.qaFill(60, 40, 700, func(int) bool { return true })
	size, ms, cutNotes, cutLayouts := e.qaRead(q)
	t.Logf("T167-QA full class: body %d bytes (cap %d), %d members, notes cut for %d, layouts dropped for %d", size, MaxBookBytes, len(ms), len(cutNotes), len(cutLayouts))
	if size > MaxBookBytes {
		t.Fatalf("body %d > MaxBookBytes %d", size, MaxBookBytes)
	}
	if len(ms) != 59 { // 59 students: the admin takes one of the MaxMembers (60) places
		t.Fatalf("members %d", len(ms))
	}
	if len(cutLayouts) == 0 {
		t.Fatalf("60 x 125 KiB layouts cannot fit 8 MiB, but nothing was dropped")
	}
	// the dropped layouts are exactly the members without a layout in the answer, and they are a suffix (earlier members keep theirs)
	first := -1
	have := 0
	for i, m := range ms {
		if m.Layout == nil {
			if first < 0 {
				first = i
			}
		} else {
			have++
			if first >= 0 {
				t.Fatalf("member %d has a layout after member %d lost theirs", i, first)
			}
		}
	}
	if have+len(cutLayouts) != 59 || first != have {
		t.Fatalf("layouts in %d, dropped %d, first without %d", have, len(cutLayouts), first)
	}
	for i, id := range cutLayouts { // ids are in book order and name the right members
		if ms[first+i].MemberID != id || ms[first+i].Name != "S"+strconv.Itoa(first+i) {
			t.Fatalf("truncated_layouts[%d] = %s is not member %d (%s)", i, id, first+i, ms[first+i].MemberID)
		}
	}
	// notes: a member is complete (all 40 notes) until the notes of one member are cut, and from then on every member's are cut
	// (the documented rule); a member whose notes are cut keeps profile and approved_total.
	cutFrom := -1
	for i, m := range ms {
		if m.Approved != 40 {
			t.Fatalf("member %d approved_total %d", i, m.Approved)
		}
		if m.Truncated {
			if cutFrom < 0 {
				cutFrom = i
			}
			if len(m.Notes) != 0 {
				t.Fatalf("member %d truncated but has %d notes", i, len(m.Notes))
			}
		} else if cutFrom >= 0 {
			t.Fatalf("member %d is complete after member %d lost theirs", i, cutFrom)
		} else if len(m.Notes) != 40 {
			t.Fatalf("member %d has %d notes, want 40", i, len(m.Notes))
		}
	}
	if len(cutNotes) != 59-cutFrom {
		t.Fatalf("truncated has %d ids, members from %d are cut", len(cutNotes), cutFrom)
	}
	t.Logf("T167-QA full class: complete members %d, layouts kept %d, layout dropped from member %d, notes cut from member %d", cutFrom, have, first, cutFrom)
}

// Notes of 10 x 700 characters (21 KB a student): the whole class (59 x 148 KB = 8.7 MB) does not fit. The documented order is
// layout first, then notes: a member keeps their layout and loses the notes while the layout still leaves the others their reserve.
func TestQA167AClassThatDoesNotFitStaysWithinTheCap(t *testing.T) {
	e := newEnv(t)
	q := e.qaFill(60, 10, 700, func(int) bool { return true })
	size, ms, cutNotes, cutLayouts := e.qaRead(q)
	t.Logf("T167-QA 10 notes: body %d, notes cut %d, layouts dropped %d", size, len(cutNotes), len(cutLayouts))
	if size > MaxBookBytes {
		t.Fatalf("body %d", size)
	}
	if len(cutNotes)+len(cutLayouts) == 0 {
		t.Fatal("the class cannot fit, expected something cut")
	}
	for i, m := range ms {
		if m.Truncated != (len(m.Notes) == 0) || m.Approved != 10 {
			t.Fatalf("member %d: truncated %v, %d notes, approved_total %d", i, m.Truncated, len(m.Notes), m.Approved)
		}
		if (m.Layout == nil) != slices.Contains(cutLayouts, m.MemberID) {
			t.Fatalf("member %d: layout nil %v but truncated_layouts says %v", i, m.Layout == nil, cutLayouts)
		}
	}
}

// A normal class: 30 members with layouts of the largest size (3.7 MiB) and a few notes: nothing is lost.
func TestQA167NormalClassLosesNothing(t *testing.T) {
	e := newEnv(t)
	q := e.qaFill(60, 5, 300, func(i int) bool { return i < 30 })
	size, ms, cutNotes, cutLayouts := e.qaRead(q)
	t.Logf("T167-QA normal class: body %d, notes cut %d, layouts dropped %d", size, len(cutNotes), len(cutLayouts))
	if len(cutNotes) != 0 || len(cutLayouts) != 0 {
		t.Fatalf("lost: notes %v layouts %v", cutNotes, cutLayouts)
	}
	for i, m := range ms {
		if (m.Layout != nil) != (i < 30) || len(m.Notes) != 5 {
			t.Fatalf("member %d: layout %v notes %d", i, m.Layout != nil, len(m.Notes))
		}
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
