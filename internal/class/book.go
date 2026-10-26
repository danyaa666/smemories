package class

import (
	"context"
	"errors"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/bookdata"
	"github.com/danyaa666/smemories/internal/notes"
	"github.com/danyaa666/smemories/internal/yearbook"
)

// The class book (T-027): what the class admin reads to lay out and print the book. It is assembled on read from the members'
// linked yearbooks and holds approved content only. Members are in class_member_tab.sort_order, then in joining order; each
// member's notes are in that student's own order (note sort_order, then creation), exactly the order of their own book-data/get,
// cut at notes.MaxClassBookNotes. The class admin never gets a right over a yearbook: every yearbook and note read below is made
// as the book's owner (the member row's user), and the admin's photo access is the narrow media read path (media.Store.readable).

// Books is the part of the yearbook service the class book uses.
type Books interface {
	Get(ctx context.Context, ownerID uint64, publicID string) (*yearbook.Yearbook, error)
}

// ApprovedNotes is the part of the notes service the class book uses: the same approved-notes query as book-data, with the class cap.
type ApprovedNotes interface {
	ClassBookNotes(ctx context.Context, ownerID uint64, bookID string) ([]*notes.BookNote, int, error)
}

// SetBookSources wires the yearbook and notes services the class book reads through (once, at start-up).
func (s *Service) SetBookSources(b Books, n ApprovedNotes) { s.books, s.notes = b, n }

// BookClass is the class as the book's cover needs it.
type BookClass struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	SchoolName     string `json:"school_name"`
	GraduationYear *int   `json:"graduation_year"`
	Language       string `json:"language"`
}

// BookStyle is how a member's page is drawn: the template, page size and language of their yearbook.
type BookStyle struct {
	TemplateID *string `json:"template_id"`
	PageSize   string  `json:"page_size"`
	Language   string  `json:"language"`
}

// BookMember is one student's pages. A member who has not linked a yearbook, or whose page is empty (no profile text, no photo and
// no approved note), is Linked=false with no content. The profile is the book-data shape (no birthday); no ids of notes' authors,
// links or collections ever appear.
type BookMember struct {
	MemberID      string                `json:"member_id"`
	Name          string                `json:"name"` // the display name, as in the roster
	Linked        bool                  `json:"linked"`
	Yearbook      *BookStyle            `json:"yearbook"`
	Profile       *bookdata.ProfileInfo `json:"profile"`
	Notes         []*notes.BookNote     `json:"notes"`          // approved only, at most notes.MaxClassBookNotes
	NoteCount     int                   `json:"note_count"`     // the number of notes returned
	ApprovedTotal int                   `json:"approved_total"` // every approved note of the page
	Truncated     bool                  `json:"truncated"`      // approved_total is more than the notes returned
}

// ClassBook is the response of GET /api/class-book/get. The class pages (intro, teachers, ...) are in the stored order (T-127).
type ClassBook struct {
	Class      BookClass    `json:"class"`
	FrontPages []Page       `json:"front_pages"`
	Members    []BookMember `json:"members"`
	BackPages  []Page       `json:"back_pages"`
}

// bookMember is a roster row of the class book: the member, and the public id of the yearbook they own and linked ("" if none).
type bookMember struct {
	id, name, book string
	userID         uint64
}

// bookMembers lists the members in book order with the display name and the linked yearbook, in one query. A link to a yearbook
// the member does not own (the API cannot make one) counts as not linked, as in the roster.
func (s *Store) bookMembers(ctx context.Context, classID uint64) ([]bookMember, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.public_id, m.user_id, u.display_name, y.public_id FROM class_member_tab m JOIN user_tab u ON u.id = m.user_id
		 LEFT JOIN yearbook_tab y ON y.id = m.yearbook_id AND y.owner_id = m.user_id
		 WHERE m.class_id = ? ORDER BY m.sort_order, m.id LIMIT ?`, classID, MaxMembers)
	if err != nil {
		return nil, dbErr(err, "list book members")
	}
	defer func() { _ = rows.Close() }()
	var out []bookMember
	for rows.Next() {
		var m bookMember
		var book *string
		if err := rows.Scan(&m.id, &m.userID, &m.name, &book); err != nil {
			return nil, dbErr(err, "scan book member")
		}
		if book != nil {
			m.book = *book
		}
		out = append(out, m)
	}
	return out, dbErr(rows.Err(), "list book members")
}

// Book is class-book/get (admin only; a student is 403, a non-member 404). One query for the members, then per linked member the
// yearbook (its book and profile) and the approved notes, so the number of queries grows with the members and never with the notes.
func (s *Service) Book(ctx context.Context, userID uint64, classID string) (*ClassBook, error) {
	c, err := s.Authorize(ctx, userID, classID, RoleAdmin)
	if err != nil {
		return nil, err
	}
	if s.books == nil || s.notes == nil {
		return nil, apperr.New(apperr.Internal, "class book is not wired")
	}
	members, err := s.store.bookMembers(ctx, c.internalID)
	if err != nil {
		return nil, err
	}
	pages, err := s.store.pages(ctx, c.internalID)
	if err != nil {
		return nil, err
	}
	out := &ClassBook{
		Class:      BookClass{ID: c.ID, Name: c.Name, SchoolName: c.SchoolName, GraduationYear: c.GraduationYear, Language: c.Language},
		FrontPages: []Page{}, Members: make([]BookMember, 0, len(members)), BackPages: []Page{},
	}
	for _, p := range pages { // front first, then back, each in book order
		if p.Placement == PlacementFront {
			out.FrontPages = append(out.FrontPages, p)
		} else {
			out.BackPages = append(out.BackPages, p)
		}
	}
	for _, m := range members {
		bm, err := s.bookMember(ctx, m)
		if err != nil {
			return nil, err
		}
		out.Members = append(out.Members, bm)
	}
	return out, nil
}

func (s *Service) bookMember(ctx context.Context, m bookMember) (BookMember, error) {
	bm := BookMember{MemberID: m.id, Name: m.name, Notes: []*notes.BookNote{}}
	if m.book == "" {
		return bm, nil
	}
	ns, total, err := s.notes.ClassBookNotes(ctx, m.userID, m.book)
	if err != nil {
		return bm, err
	}
	y, err := s.books.Get(ctx, m.userID, m.book)
	if errors.Is(err, yearbook.ErrNotFound) { // deleted since the member list was read: no page
		return bm, nil
	}
	if err != nil {
		return bm, err
	}
	// A photo id that points at nothing (profile_complete only checks the column) arrives as nil: the page renders without it.
	p := bookdata.ProfileInfo{FullName: y.Profile.FullName, Nickname: y.Profile.Nickname, Quote: y.Profile.Quote, Hobbies: y.Profile.Hobbies,
		FuturePlans: y.Profile.FuturePlans, PhotoMediaID: y.Profile.PhotoMediaID}
	if total == 0 && p.FullName == "" && p.Nickname == "" && p.Quote == "" && p.Hobbies == "" && p.FuturePlans == "" && p.PhotoMediaID == nil {
		return bm, nil
	}
	if ns == nil {
		ns = []*notes.BookNote{} // JSON [], never null
	}
	bm.Linked, bm.Profile, bm.Notes = true, &p, ns
	bm.Yearbook = &BookStyle{TemplateID: y.TemplateID, PageSize: y.PageSize, Language: y.Language}
	bm.NoteCount, bm.ApprovedTotal, bm.Truncated = len(ns), total, total > len(ns)
	return bm, nil
}
