package bookdata

import (
	"context"
	"testing"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/notes"
	"github.com/danyaa666/smemories/internal/yearbook"
)

type fakeBooks struct {
	y     *yearbook.Yearbook
	err   error
	calls int
}

func (f *fakeBooks) Get(context.Context, uint64, string) (*yearbook.Yearbook, error) {
	f.calls++
	return f.y, f.err
}

type fakeNotes struct {
	ns  []*notes.BookNote
	err error
}

func (f fakeNotes) ApprovedNotes(context.Context, uint64, bool, string) ([]*notes.BookNote, error) {
	return f.ns, f.err
}

// The notes call validates the id, so its error wins and the book is not read; a missing book is its own error.
func TestGetErrorsAndShape(t *testing.T) {
	bad := apperr.New(apperr.Param, "yearbook_id must be a yearbook id")
	b := &fakeBooks{}
	if _, err := NewService(b, fakeNotes{err: bad}).Get(context.Background(), 1, true, "x"); err != bad || b.calls != 0 {
		t.Fatalf("err %v, book reads %d", err, b.calls)
	}
	gone := apperr.New(apperr.NotFound, "yearbook not found")
	if _, err := NewService(&fakeBooks{err: gone}, fakeNotes{}).Get(context.Background(), 1, true, "x"); err != gone {
		t.Fatalf("err %v", err)
	}
	year := 2026
	y := &yearbook.Yearbook{Title: "T", GraduationYear: &year, Profile: yearbook.Profile{FullName: "An", Birthday: new(string)}}
	got, err := NewService(&fakeBooks{y: y}, fakeNotes{ns: []*notes.BookNote{{ID: "N1"}, {ID: "N2"}}}).Get(context.Background(), 1, true, "x")
	if err != nil || got.NoteCount != 2 || got.Yearbook.Title != "T" || *got.Yearbook.GraduationYear != 2026 || got.Profile.FullName != "An" {
		t.Fatalf("got %+v, %v", got, err)
	}
}
