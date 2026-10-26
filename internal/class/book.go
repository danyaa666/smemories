package class

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"

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

// MaxBookBytes is the most bytes class-book/get ever answers, envelope included (T-133). A maximal class (60 members x 40 notes of
// the longest text in four-byte characters) is about 50 MiB, which the server must neither build nor send to one request.
const MaxBookBytes = 8 << 20

const (
	// strippedMemberMax is the most a member costs once their notes are dropped: the profile at its longest (1250 characters, six
	// bytes each when every one is "<"), the name, the style and the keys. Every member after the one that does not fit is reserved
	// this much, so the cut itself can never push the body over MaxBookBytes. A unit test builds the worst member and checks it.
	strippedMemberMax = 12 << 10
	// bookFrameBytes covers the envelope, the keys and the list of truncated members (60 ids of 26 characters).
	bookFrameBytes = 1<<10 + MaxMembers*idJSONBytes
	idJSONBytes    = 26 + 3 // a ULID in quotes, and the comma
)

// ClassBook is the response of GET /api/class-book/get. The class pages (intro, teachers, ...) are in the stored order (T-127).
// Members are encoded one by one as they are read, so the whole body exists once, never twice, and never above MaxBookBytes
// (Encode writes it straight to the response). Truncated lists the members whose notes were left out to stay within the cap.
type ClassBook struct {
	Class      BookClass         `json:"class"`
	FrontPages []Page            `json:"front_pages"`
	Members    []json.RawMessage `json:"members"` // each one an encoded BookMember, in book order
	BackPages  []Page            `json:"back_pages"`
	Truncated  []string          `json:"truncated"` // member ids whose notes did not fit in MaxBookBytes; [] for a normal class
}

// Encode writes the book as JSON to w piece by piece: the same bytes as json.Marshal, without a second copy of the body.
func (b *ClassBook) Encode(w io.Writer) (err error) {
	str := func(s string) {
		if err == nil {
			_, err = io.WriteString(w, s)
		}
	}
	val := func(v any) {
		j, _ := json.Marshal(v) // plain structs and strings: cannot fail
		if err == nil {
			_, err = w.Write(j)
		}
	}
	str(`{"class":`)
	val(b.Class)
	str(`,"front_pages":`)
	val(b.FrontPages)
	str(`,"members":[`)
	for i, m := range b.Members {
		if i > 0 {
			str(",")
		}
		if err == nil {
			_, err = w.Write(m)
		}
	}
	str(`],"back_pages":`)
	val(b.BackPages)
	str(`,"truncated":`)
	val(b.Truncated)
	str("}")
	return err
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
	if err := s.throttleRead(ctx, userID, c.ID); err != nil {
		return nil, err
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
		FrontPages: []Page{}, BackPages: []Page{},
	}
	for _, p := range pages { // front first, then back, each in book order
		if p.Placement == PlacementFront {
			out.FrontPages = append(out.FrontPages, p)
		} else {
			out.BackPages = append(out.BackPages, p)
		}
	}
	front, _ := json.Marshal(out.FrontPages)
	back, _ := json.Marshal(out.BackPages)
	class, _ := json.Marshal(out.Class)
	out.Members, out.Truncated, err = s.encodeMembers(ctx, members, MaxBookBytes-bookFrameBytes-len(class)-len(front)-len(back))
	if err != nil {
		return nil, err
	}
	return out, nil
}

// encodeMembers reads the members in book order and encodes each one, keeping the encoded sizes under budget. The first member
// whose notes would pass the budget loses them, and so does every later one (earlier members stay complete); their ids are the
// second result. A cut member keeps its profile and approved_total and has truncated=true and no notes. Memory is the encoded
// members (at most the budget) plus one member's notes, never the whole class.
func (s *Service) encodeMembers(ctx context.Context, members []bookMember, budget int) ([]json.RawMessage, []string, error) {
	out, cut, used := make([]json.RawMessage, 0, len(members)), []string{}, 0
	over := false
	for i, m := range members {
		bm, err := s.bookMember(ctx, m)
		if err != nil {
			return nil, nil, err
		}
		var raw []byte
		if !over {
			raw, _ = json.Marshal(bm) // a plain struct plus raw JSON the notes service validated: cannot fail
			over = used+len(raw)+1+(len(members)-i-1)*strippedMemberMax > budget
		}
		if over {
			if len(bm.Notes) > 0 {
				cut = append(cut, bm.MemberID)
				bm.Notes, bm.NoteCount, bm.Truncated = []*notes.BookNote{}, 0, true
			}
			raw, _ = json.Marshal(bm)
		}
		used += len(raw) + 1
		out = append(out, raw)
	}
	return out, cut, nil
}

// throttleRead takes one read of class-book/get from the user's budget for this class: a *RetryAfterError when it is used up, the
// limiter's own error (503, fail closed) when the limiter store is down. It is taken after Authorize, so only members count.
func (s *Service) throttleRead(ctx context.Context, userID uint64, classID string) error {
	ok, wait, err := s.bookReads.Take(ctx, strconv.FormatUint(userID, 10)+":"+classID)
	if err != nil {
		return err
	}
	if !ok {
		return &RetryAfterError{Err: apperr.New(apperr.RateLimited, "too many requests, try again later"), After: wait}
	}
	return nil
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
