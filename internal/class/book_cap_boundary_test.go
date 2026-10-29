package class

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/danyaa666/smemories/internal/notes"
	"github.com/danyaa666/smemories/internal/yearbook"
)

// notesByOwner answers a different note list per member (QA T-133), so one class can mix members with and without notes.
type notesByOwner map[uint64][]*notes.BookNote

func (f notesByOwner) ClassBookNotes(_ context.Context, owner uint64, _ string) ([]*notes.BookNote, int, error) {
	return f[owner], len(f[owner]), nil
}

// QA T-133: the cut sits exactly at the budget. Budget == the bytes of the members: nothing is cut; one byte less: the last member
// (and only it) loses its notes. A member with a profile and no notes after the cut is never listed in truncated.
func TestClassBookCapBoundary(t *testing.T) {
	ns := make([]*notes.BookNote, notes.MaxClassBookNotes)
	for i := range ns {
		ns[i] = maximalNote(i)
	}
	y := &yearbook.Yearbook{PageSize: "A4", Language: "vi", Profile: yearbook.Profile{FullName: "A"}}
	s := &Service{books: fakeBooks{y: y}, notes: notesByOwner{1: ns, 2: ns}}
	ms := []bookMember{
		{id: "01K7Q8W3ZR5N2M4X6V8B0C1001", name: "A", book: "b", userID: 1},
		{id: "01K7Q8W3ZR5N2M4X6V8B0C1002", name: "B", book: "b", userID: 2},
	}
	big, cut, err := s.encodeMembers(context.Background(), ms, 1<<40)
	if err != nil || len(cut) != 0 {
		t.Fatalf("%v %v", err, cut)
	}
	exact := len(big[0]) + 1 + len(big[1]) + 1
	if len(big[1]) < strippedMemberMax {
		t.Fatal("fixture too small to exercise the reserve")
	}

	got, cut, err := s.encodeMembers(context.Background(), ms, exact)
	if err != nil || len(cut) != 0 || len(got[1]) != len(big[1]) {
		t.Fatalf("budget == size: cut %v err %v", cut, err)
	}
	got, cut, err = s.encodeMembers(context.Background(), ms, exact-1)
	if err != nil || len(cut) != 1 || cut[0] != ms[1].id || len(got[0]) != len(big[0]) {
		t.Fatalf("budget == size-1: cut %v err %v", cut, err)
	}
	var m BookMember
	if json.Unmarshal(got[1], &m) != nil || len(m.Notes) != 0 || !m.Truncated || m.ApprovedTotal != notes.MaxClassBookNotes || m.NoteCount != 0 {
		t.Fatalf("cut member: %+v", m)
	}

	// member 1 does not fit at all: it and the later member are cut; a third member with a profile and zero notes is not listed
	s.notes = notesByOwner{1: ns, 2: ns, 3: nil}
	ms = append(ms, bookMember{id: "01K7Q8W3ZR5N2M4X6V8B0C1003", name: "C", book: "b", userID: 3})
	_, cut, err = s.encodeMembers(context.Background(), ms, len(big[0])/2)
	if err != nil || len(cut) != 2 || cut[0] != ms[0].id || cut[1] != ms[1].id {
		t.Fatalf("zero-note member listed or cut wrong: %v %v", cut, err)
	}
}
