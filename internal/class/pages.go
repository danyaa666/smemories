package class

import (
	"context"
	"slices"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/textx"
	"github.com/danyaa666/smemories/internal/ulid"
)

// The class pages and the book order (T-127): intro, teachers, events and other pages the admin writes for the whole class, and
// the order of the members and of those pages in the book. Admin only, for reads and writes alike. A page body is plain text:
// it is stored and returned as typed (after the usual text normalisation) and is never interpreted as HTML on the server; the web
// shows it as text.

const (
	// MaxPages is the most pages one class can hold (DESIGN.md, limits); counted under the class row lock.
	MaxPages     = 30
	maxPageTitle = 120
	maxPageBody  = 4000 // characters

	PlacementFront = "front" // before the students' pages
	PlacementBack  = "back"  // after them
)

var (
	pageKinds  = []string{"intro", "teachers", "events", "superlatives", "custom"}
	placements = []string{PlacementFront, PlacementBack}
)

// ErrPageNotFound is a page id that is not in this class (an id of another class included).
var ErrPageNotFound = apperr.New(apperr.NotFound, "page not found")

// ErrPagesFull is the 31st page of a class.
var ErrPagesFull = apperr.New(CodeLimitReached, "at most 30 pages per class")

// Page is a class page. Pages are listed (and printed) per placement in their stored order; the order itself is not a field.
type Page struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Placement string `json:"placement"`
	CreatedAt int64  `json:"created_at"` // Unix ms
	UpdatedAt int64  `json:"updated_at"` // Unix ms
}

// PageInput is the body of class-page/create and (next to class_id and id) class-page/update. update replaces every field.
type PageInput struct {
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Placement string `json:"placement"`
}

// clean returns the validated copy of in. The body may be empty and keeps its newlines.
func (in *PageInput) clean() (*PageInput, error) {
	var out PageInput
	var ok bool
	if !slices.Contains(pageKinds, in.Kind) {
		return nil, invalid("kind")
	}
	out.Kind = in.Kind
	if out.Title, ok = textx.Clean(in.Title, 1, maxPageTitle); !ok {
		return nil, invalid("title")
	}
	if out.Body, ok = textx.CleanMultiline(in.Body, 0, maxPageBody); !ok {
		return nil, invalid("body")
	}
	if !slices.Contains(placements, in.Placement) {
		return nil, invalid("placement")
	}
	out.Placement = in.Placement
	return &out, nil
}

// OrderInput is the body of class-book/update-order: the new order of every member and, per placement, of every page.
type OrderInput struct {
	MemberIDs    []string `json:"member_ids"`
	FrontPageIDs []string `json:"front_page_ids"`
	BackPageIDs  []string `json:"back_page_ids"`
}

// checkOrderIDs is ERROR_INVALID_<FIELD> for a list that is too long, has a malformed id or repeats one. Whether the ids are the
// right ones is decided under the class lock (Store.updateOrder).
func checkOrderIDs(ids []string, limit int, field string) error {
	if len(ids) > limit {
		return invalid(field)
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup || !ulid.Valid(id) {
			return invalid(field)
		}
		seen[id] = struct{}{}
	}
	return nil
}

// Pages lists the class pages (admin): the front pages, then the back pages, each in book order.
func (s *Service) Pages(ctx context.Context, userID uint64, classID string) ([]Page, error) {
	c, err := s.Authorize(ctx, userID, classID, RoleAdmin)
	if err != nil {
		return nil, err
	}
	return s.store.pages(ctx, c.internalID)
}

// CreatePage adds a page at the end of its placement (admin). At most MaxPages per class, counted under the class lock.
func (s *Service) CreatePage(ctx context.Context, userID uint64, classID string, in *PageInput) (*Page, error) {
	if _, err := s.Authorize(ctx, userID, classID, RoleAdmin); err != nil {
		return nil, err
	}
	clean, err := in.clean()
	if err != nil {
		return nil, err
	}
	if err := s.throttle(ctx, userID); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	p := &Page{ID: ulid.New(now), Kind: clean.Kind, Title: clean.Title, Body: clean.Body, Placement: clean.Placement, CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli()}
	if err := s.store.createPage(ctx, userID, classID, p); err != nil {
		return nil, err
	}
	return p, nil
}

// UpdatePage replaces the fields of a page (admin). A page that changes placement goes to the end of the new one.
func (s *Service) UpdatePage(ctx context.Context, userID uint64, classID, pageID string, in *PageInput) (*Page, error) {
	if _, err := s.Authorize(ctx, userID, classID, RoleAdmin); err != nil {
		return nil, err
	}
	clean, err := in.clean()
	if err != nil {
		return nil, err
	}
	if err := s.throttle(ctx, userID); err != nil {
		return nil, err
	}
	return s.store.updatePage(ctx, userID, classID, pageID, clean, s.now().UnixMilli())
}

// DeletePage removes a page (admin).
func (s *Service) DeletePage(ctx context.Context, userID uint64, classID, pageID string) error {
	if _, err := s.Authorize(ctx, userID, classID, RoleAdmin); err != nil {
		return err
	}
	if err := s.throttle(ctx, userID); err != nil {
		return err
	}
	return s.store.deletePage(ctx, userID, classID, pageID)
}

// UpdateOrder sets the book order (admin). Each list must be a permutation of the current set: every member, every front page and
// every back page exactly once (else 400 ERROR_INVALID_<FIELD> and nothing changes). It is applied in one transaction under the
// class lock, so two admins cannot interleave: the book ends in one of the two orders.
func (s *Service) UpdateOrder(ctx context.Context, userID uint64, classID string, in *OrderInput) error {
	if _, err := s.Authorize(ctx, userID, classID, RoleAdmin); err != nil {
		return err
	}
	for _, l := range []struct {
		ids   []string
		limit int
		field string
	}{{in.MemberIDs, MaxMembers, "member_ids"}, {in.FrontPageIDs, MaxPages, "front_page_ids"}, {in.BackPageIDs, MaxPages, "back_page_ids"}} {
		if err := checkOrderIDs(l.ids, l.limit, l.field); err != nil {
			return err
		}
	}
	if err := s.throttle(ctx, userID); err != nil {
		return err
	}
	return s.store.updateOrder(ctx, userID, classID, in, s.now().UnixMilli())
}
