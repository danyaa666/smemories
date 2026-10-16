// Package bookdata serves the owner's whole book in one response for the browser renderer (T-056, D-24): the yearbook, the
// owner's profile and the approved notes in book order. It stores nothing; it reads through the yearbook and notes services.
package bookdata

import (
	"context"

	"github.com/danyaa666/smemories/internal/notes"
	"github.com/danyaa666/smemories/internal/yearbook"
)

// Books is the part of the yearbook service the book data uses.
type Books interface {
	Get(ctx context.Context, ownerID uint64, publicID string) (*yearbook.Yearbook, error)
}

// Notes is the part of the notes service the book data uses.
type Notes interface {
	ApprovedNotes(ctx context.Context, ownerID uint64, verified bool, bookID string) ([]*notes.BookNote, int, error)
}

// BookInfo is the yearbook as the renderer needs it: no internal ids, no times.
type BookInfo struct {
	Title          string  `json:"title"`
	SchoolName     string  `json:"school_name"`
	ClassName      string  `json:"class_name"`
	GraduationYear *int    `json:"graduation_year"`
	Motto          string  `json:"motto"`
	Language       string  `json:"language"`
	PageSize       string  `json:"page_size"`
	TemplateID     *string `json:"template_id"`
	CoverMediaID   *string `json:"cover_media_id"`
}

// ProfileInfo is the owner's profile page; the birthday stays out.
type ProfileInfo struct {
	FullName     string  `json:"full_name"`
	Nickname     string  `json:"nickname"`
	Quote        string  `json:"quote"`
	Hobbies      string  `json:"hobbies"`
	FuturePlans  string  `json:"future_plans"`
	PhotoMediaID *string `json:"photo_media_id"`
}

// Book is the response of GET /api/book-data/get.
type Book struct {
	Yearbook      BookInfo          `json:"yearbook"`
	Profile       ProfileInfo       `json:"profile"`
	Notes         []*notes.BookNote `json:"notes"`
	NoteCount     int               `json:"note_count"`     // the number of notes returned
	ApprovedTotal int               `json:"approved_total"` // every approved note of the book
	Truncated     bool              `json:"truncated"`      // true when approved_total is more than the notes returned
}

// Service builds the book.
type Service struct {
	books Books
	notes Notes
}

// NewService wires the two sources.
func NewService(b Books, n Notes) *Service { return &Service{books: b, notes: n} }

// Get returns the owner's book. A malformed id is ERROR_PARAM and an unverified email is refused (the notes rule, T-013);
// a missing book and another user's are both ERROR_NOT_FOUND. The notes are read first because that call validates the id.
// A book with more approved notes than the cap comes back cut, with Truncated set and the real count in ApprovedTotal.
func (s *Service) Get(ctx context.Context, ownerID uint64, verified bool, bookID string) (*Book, error) {
	ns, total, err := s.notes.ApprovedNotes(ctx, ownerID, verified, bookID)
	if err != nil {
		return nil, err
	}
	y, err := s.books.Get(ctx, ownerID, bookID)
	if err != nil {
		return nil, err
	}
	p := &y.Profile
	return &Book{
		Yearbook: BookInfo{Title: y.Title, SchoolName: y.SchoolName, ClassName: y.ClassName, GraduationYear: y.GraduationYear, Motto: y.Motto,
			Language: y.Language, PageSize: y.PageSize, TemplateID: y.TemplateID, CoverMediaID: y.CoverMediaID},
		Profile: ProfileInfo{FullName: p.FullName, Nickname: p.Nickname, Quote: p.Quote, Hobbies: p.Hobbies, FuturePlans: p.FuturePlans,
			PhotoMediaID: p.PhotoMediaID},
		Notes: ns, NoteCount: len(ns), ApprovedTotal: total, Truncated: total > len(ns),
	}, nil
}
