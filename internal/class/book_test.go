package class

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/bookdata"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/notes"
	"github.com/danyaa666/smemories/internal/yearbook"
)

type fakeBooks struct {
	y   *yearbook.Yearbook
	err error
}

func (f fakeBooks) Get(context.Context, uint64, string) (*yearbook.Yearbook, error) {
	return f.y, f.err
}

type fakeNotes struct {
	ns    []*notes.BookNote
	total int
	owner *uint64
}

func (f fakeNotes) ClassBookNotes(_ context.Context, owner uint64, _ string) ([]*notes.BookNote, int, error) {
	if f.owner != nil {
		*f.owner = owner
	}
	return f.ns, f.total, nil
}

func TestBookMember(t *testing.T) {
	photo := "01K7Q8W3ZR5N2M4X6V8B0C1D2E"
	named := &yearbook.Yearbook{PageSize: "A5", Language: "vi", Profile: yearbook.Profile{FullName: "Linh", Quote: "q"}}
	one := []*notes.BookNote{{ID: "n1", PhotoIDs: []string{}}}
	member := bookMember{id: "m1", name: "Linh", book: "b1", userID: 7}
	for name, tc := range map[string]struct {
		m                       bookMember
		y                       *yearbook.Yearbook
		err                     error
		ns                      []*notes.BookNote
		total                   int
		linked, truncated       bool
		wantNotes, wantApproved int
	}{
		"not linked":             {m: bookMember{id: "m1", name: "Linh"}},
		"book deleted meanwhile": {m: member, err: yearbook.ErrNotFound},
		"empty page":             {m: member, y: &yearbook.Yearbook{}},
		"photo only":             {m: member, y: &yearbook.Yearbook{Profile: yearbook.Profile{PhotoMediaID: &photo}}, linked: true},
		"profile, no notes":      {m: member, y: named, linked: true},
		"notes only":             {m: member, y: &yearbook.Yearbook{}, ns: one, total: 1, linked: true, wantNotes: 1, wantApproved: 1},
		"cut at the cap":         {m: member, y: named, ns: one, total: 41, linked: true, truncated: true, wantNotes: 1, wantApproved: 41},
	} {
		t.Run(name, func(t *testing.T) {
			s := &Service{books: fakeBooks{tc.y, tc.err}, notes: fakeNotes{ns: tc.ns, total: tc.total}}
			got, err := s.bookMember(context.Background(), tc.m)
			if err != nil {
				t.Fatal(err)
			}
			if got.Linked != tc.linked || got.Truncated != tc.truncated || got.NoteCount != tc.wantNotes || got.ApprovedTotal != tc.wantApproved {
				t.Fatalf("got %+v", got)
			}
			if got.Notes == nil || (got.Linked != (got.Profile != nil)) || (got.Linked != (got.Yearbook != nil)) {
				t.Fatalf("notes must be [] and profile and yearbook null exactly when not linked: %+v", got)
			}
		})
	}
}

// The notes of a page are read as the page's owner (the member row's user), never as the class admin who asked.
func TestBookMemberReadsAsTheOwner(t *testing.T) {
	var owner uint64
	s := &Service{books: fakeBooks{y: &yearbook.Yearbook{Profile: yearbook.Profile{FullName: "Linh"}}}, notes: fakeNotes{owner: &owner}}
	if _, err := s.bookMember(context.Background(), bookMember{id: "m", book: "b", userID: 42}); err != nil || owner != 42 {
		t.Fatalf("notes read as %d, %v", owner, err)
	}
}

// The wire shape of an unlinked member: arrays are [] and objects null, so a client never branches on a missing key.
func TestBookMemberJSON(t *testing.T) {
	s := &Service{books: fakeBooks{}, notes: fakeNotes{}}
	bm, _ := s.bookMember(context.Background(), bookMember{id: "m", name: "N"})
	b, _ := json.Marshal(bm)
	want := `{"member_id":"m","name":"N","linked":false,"yearbook":null,"profile":null,"notes":[],"note_count":0,"approved_total":0,"truncated":false}`
	if string(b) != want {
		t.Fatalf("%s\nwant %s", b, want)
	}
}

// media repeats the admin role of this package (it cannot import it): the two must stay equal.
func TestMediaAdminRoleMatches(t *testing.T) {
	if media.ClassAdminRole != RoleAdmin {
		t.Fatalf("media.ClassAdminRole = %q, class.RoleAdmin = %q", media.ClassAdminRole, RoleAdmin)
	}
	if media.ShareableNoteStatus != noteApproved || !strings.EqualFold(noteApproved, notes.StatusApproved) {
		t.Fatal("the approved status drifted")
	}
}

// The size of a full class book (60 members x 40 approved notes of about 230 characters with two photos each, no signature) is
// measured here without a database: the shape is the whole story. The integration test TestClassBookFullClass measures the real answer.
func TestClassBookSizeOfAFullClass(t *testing.T) {
	msg := strings.Repeat("Lời nhắn thân thương. ", 10)
	id := "01K7Q8W3ZR5N2M4X6V8B0C1D2E"
	photo := id
	b := ClassBook{Class: BookClass{ID: id, Name: "Lớp 12A1", Language: "vi"}, FrontPages: []any{}, BackPages: []any{}}
	for range MaxMembers {
		m := BookMember{MemberID: id, Name: "Nguyễn Thị Hồng", Linked: true, Yearbook: &BookStyle{PageSize: "A4", Language: "vi"},
			Profile: &bookdata.ProfileInfo{FullName: "Nguyễn Thị Hồng", Quote: msg, PhotoMediaID: &photo}, NoteCount: notes.MaxClassBookNotes, ApprovedTotal: notes.MaxClassBookNotes}
		for range notes.MaxClassBookNotes {
			ans, _ := json.Marshal(map[string]string{"name": "Linh", "relationship": "classmate", "message": msg})
			m.Notes = append(m.Notes, &notes.BookNote{ID: id, Answers: ans, PhotoIDs: []string{id, id}})
		}
		b.Members = append(b.Members, m)
	}
	out, _ := json.Marshal(b)
	t.Logf("full class book: %d bytes (%.2f MiB)", len(out), float64(len(out))/(1<<20))
	if len(out) > 4<<20 {
		t.Fatalf("%d bytes is over the 4 MiB budget", len(out))
	}
}
