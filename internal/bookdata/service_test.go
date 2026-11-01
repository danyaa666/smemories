package bookdata

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/bookdesign"
	"github.com/danyaa666/smemories/internal/booklayout"
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

// fakeDesigns answers the same design for every book and records the template it was asked about.
type fakeDesigns struct {
	d   *bookdesign.Resolved
	err error
	tpl *string
}

func (f *fakeDesigns) Resolved(_ context.Context, _ uint64, _ string, tpl *string) (*bookdesign.Resolved, error) {
	f.tpl = tpl
	return f.d, f.err
}

// fakeLayouts answers the same layout for every book.
type fakeLayouts struct {
	l   *booklayout.Resolved
	err error
}

func (f fakeLayouts) Resolved(context.Context, uint64, string, *string) (*booklayout.Resolved, error) {
	return f.l, f.err
}

type fakeNotes struct {
	ns    []*notes.BookNote
	total int
	err   error
}

func (f fakeNotes) ApprovedNotes(context.Context, uint64, bool, string) ([]*notes.BookNote, int, error) {
	return f.ns, f.total, f.err
}

// The notes call validates the id, so its error wins and the book is not read; a missing book is its own error.
func TestGetErrorsAndShape(t *testing.T) {
	bad := apperr.New(apperr.Param, "yearbook_id must be a yearbook id")
	b := &fakeBooks{}
	if _, err := NewService(b, fakeNotes{err: bad}, &fakeDesigns{}, fakeLayouts{}).Get(context.Background(), 1, true, "x"); err != bad || b.calls != 0 {
		t.Fatalf("err %v, book reads %d", err, b.calls)
	}
	gone := apperr.New(apperr.NotFound, "yearbook not found")
	if _, err := NewService(&fakeBooks{err: gone}, fakeNotes{}, &fakeDesigns{}, fakeLayouts{}).Get(context.Background(), 1, true, "x"); err != gone {
		t.Fatalf("err %v", err)
	}
	year := 2026
	y := &yearbook.Yearbook{Title: "T", GraduationYear: &year, Profile: yearbook.Profile{FullName: "An", Birthday: new(string)}}
	tpl := "memory-book"
	y.TemplateID = &tpl
	fd := &fakeDesigns{d: &bookdesign.Resolved{Theme: "peach"}}
	got, err := NewService(&fakeBooks{y: y}, fakeNotes{ns: []*notes.BookNote{{ID: "N1"}, {ID: "N2"}}, total: 5}, fd, fakeLayouts{l: &booklayout.Resolved{Layout: []byte(`{"v":1,"pages":{}}`), Version: 7}}).Get(context.Background(), 1, true, "x")
	if err != nil || got.NoteCount != 2 || got.ApprovedTotal != 5 || !got.Truncated || got.Yearbook.Title != "T" || *got.Yearbook.GraduationYear != 2026 || got.Profile.FullName != "An" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got.Design == nil || got.Design.Theme != "peach" || fd.tpl == nil || *fd.tpl != "memory-book" {
		t.Errorf("design %+v for template %v", got.Design, fd.tpl)
	}
	if string(got.Layout) != `{"v":1,"pages":{}}` || got.LayoutVersion != 7 {
		t.Errorf("layout %s version %d", got.Layout, got.LayoutVersion)
	}
	none, err := NewService(&fakeBooks{y: y}, fakeNotes{}, fd, fakeLayouts{}).Get(context.Background(), 1, true, "x")
	if err != nil || none.Layout != nil || none.LayoutVersion != 0 {
		t.Errorf("no layout: %s %d %v", none.Layout, none.LayoutVersion, err)
	}
	if out, _ := json.Marshal(none); !strings.Contains(string(out), `"layout":null,"layout_version":0`) {
		t.Errorf("no layout is not null and 0: %s", out)
	}
	if _, err := NewService(&fakeBooks{y: y}, fakeNotes{}, fd, fakeLayouts{err: apperr.New(apperr.Internal, "x")}).Get(context.Background(), 1, true, "x"); err == nil {
		t.Error("a layout error must fail the call")
	}
	boom := apperr.New(apperr.Internal, "design store down")
	if _, err := NewService(&fakeBooks{y: y}, fakeNotes{}, &fakeDesigns{err: boom}, fakeLayouts{}).Get(context.Background(), 1, true, "x"); err != boom {
		t.Errorf("a design error must fail the call, got %v", err)
	}
}
