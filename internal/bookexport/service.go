// Package bookexport builds the owner's book as one plain-text document with a public link for every photo (T-093, D-32).
// The text is made on the server so the link secret stays there and the print preview's note cap does not shorten an export.
// It stores nothing; it reads through the yearbook and notes services. Format and limits: docs/book-export.md.
package bookexport

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/medialink"
	"github.com/danyaa666/smemories/internal/notefields"
	"github.com/danyaa666/smemories/internal/notes"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/templates"
	"github.com/danyaa666/smemories/internal/yearbook"
)

// Books is the part of the yearbook service the export uses.
type Books interface {
	Get(ctx context.Context, ownerID uint64, publicID string) (*yearbook.Yearbook, error)
}

// Notes is the part of the notes service the export uses.
type Notes interface {
	ExportNotes(ctx context.Context, ownerID uint64, verified bool, bookID string) ([]*notes.BookNote, int, error)
}

const (
	exportsPerWindow = 30
	exportWindow     = 10 * time.Minute
	linkPath         = "/api/public-media/get-content?token="
)

// Export is the response of GET /api/book-export/get-text.
type Export struct {
	Filename      string `json:"filename"`
	Text          string `json:"text"`
	NoteCount     int    `json:"note_count"`     // notes in the text
	PhotoCount    int    `json:"photo_count"`    // photo links in the text
	ApprovedTotal int    `json:"approved_total"` // every approved note of the book
	Truncated     bool   `json:"truncated"`      // true when approved_total is more than note_count
}

// RetryAfterError is the 429: the handler turns After into the Retry-After header.
type RetryAfterError struct {
	Err   error
	After time.Duration
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error { return e.Err }

// Service builds the export.
type Service struct {
	books   Books
	notes   Notes
	signer  *medialink.Signer
	baseURL string // SMEM_PUBLIC_BASE_URL, no trailing slash
	limit   ratelimit.Limiter
	now     func() time.Time
}

// NewService wires the sources. limiters and now may be nil (no rate limit, time.Now).
func NewService(b Books, n Notes, signer *medialink.Signer, baseURL string, limiters *ratelimit.Factory, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	s := &Service{books: b, notes: n, signer: signer, baseURL: baseURL, now: now}
	if limiters != nil {
		s.limit = limiters.Open("book_export", exportsPerWindow, exportWindow) // fails open: it bounds effort, not guessing
	}
	return s
}

// GetText returns the owner's book as text in lang ("" = the book's language; otherwise en or vi, else ERROR_PARAM). A malformed
// id is ERROR_PARAM and an unverified email is refused (the notes rule, T-013); a missing book and another user's are both
// ERROR_NOT_FOUND. The notes are read first because that call validates the id.
func (s *Service) GetText(ctx context.Context, ownerID uint64, verified bool, bookID, lang string) (*Export, error) {
	if _, ok := texts[lang]; !ok && lang != "" {
		return nil, apperr.New(apperr.Param, "lang must be en or vi")
	}
	if s.limit != nil {
		if ok, wait, _ := s.limit.Take(ctx, strconv.FormatUint(ownerID, 10)); !ok {
			return nil, &RetryAfterError{Err: apperr.New(apperr.RateLimited, "too many requests, try again later"), After: wait}
		}
	}
	ns, total, err := s.notes.ExportNotes(ctx, ownerID, verified, bookID)
	if err != nil {
		return nil, err
	}
	y, err := s.books.Get(ctx, ownerID, bookID)
	if err != nil {
		return nil, err
	}
	if lang == "" {
		lang = y.Language
	}
	if _, ok := texts[lang]; !ok {
		lang = "en"
	}
	refs := notefields.Default()
	if y.TemplateID != nil {
		if r, ok := templates.NoteFields(*y.TemplateID); ok {
			refs = r
		}
	}
	now := s.now()
	link := func(mediaID *string) string {
		if mediaID == nil {
			return ""
		}
		return s.baseURL + linkPath + s.signer.Token(*mediaID, y.ID, y.LinkEpoch)
	}
	p := &y.Profile
	b := &book{
		title: y.Title, school: y.SchoolName, class: y.ClassName, motto: y.Motto, graduationYear: y.GraduationYear,
		fullName: p.FullName, nickname: p.Nickname, quote: p.Quote, hobbies: p.Hobbies, plans: p.FuturePlans,
		photoLink: link(p.PhotoMediaID), coverLink: link(y.CoverMediaID), fields: refs, total: total, now: now,
	}
	for _, n := range ns {
		answers := map[string]string{}
		if err := json.Unmarshal(n.Answers, &answers); err != nil {
			return nil, apperr.New(apperr.Internal, "note answers are unreadable")
		}
		out := note{answers: answers}
		for _, id := range n.PhotoIDs[:min(len(n.PhotoIDs), maxNotePhotos)] {
			out.photos = append(out.photos, link(&id))
		}
		b.notes = append(b.notes, out)
	}
	text, photos := render(lang, b)
	return &Export{Filename: filename(y.Title, now), Text: text, NoteCount: len(ns), PhotoCount: photos, ApprovedTotal: total,
		Truncated: total > len(ns)}, nil
}
