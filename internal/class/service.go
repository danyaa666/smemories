// Package class is the class space of the class yearbook (E07, D-36): classes, their members and the two roles, admin and
// student. A class is a thin layer over the personal yearbooks; it never gives anyone rights over another person's yearbook.
// The permission matrix is docs/class-permissions.md.
package class

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/ulid"
)

const (
	// DefaultListLimit and MaxListLimit are the page sizes of class/get-list.
	DefaultListLimit = 20
	MaxListLimit     = 50
	// MaxMembers is the most members a class can hold (DESIGN.md, limits); the roster fits in one page of this size.
	MaxMembers = 60

	maxAdminClasses = 10 // classes one user can be admin of

	writesPerWindow = 120 // class writes per user and window
	writeWindow     = time.Hour

	bookReadsPerWindow = 30 // class-book/get per user and class and window (T-133); an admin laying out a book needs a handful
	bookReadWindow     = time.Hour
)

// Service holds every rule of the class domain: validation, limits, the role checks and the last-admin rule. The handler
// only decodes and maps; the store only runs SQL.
type Service struct {
	store     *Store
	tokens    Tokens
	writes    ratelimit.Limiter
	bookReads ratelimit.Limiter // class-book/get, per user and class (closed)
	now       func() time.Time

	lookupAll, lookupMisses ratelimit.Limiter // class-invite/lookup, per client IP (open: they bound load, not guessing)
	acceptIP                ratelimit.Limiter // class-invite/accept, per client IP (closed, like every write)
	baseURL                 string            // origin of the invite links, see SetPublicBaseURL
	books                   Books             // the class book's sources, see SetBookSources
	notes                   ApprovedNotes
}

// NewService builds the service. limiters is required: class writes are rate-limited per user and, if the limiter store is
// down, refused (fail closed, 503). now may be nil (time.Now).
func NewService(store *Store, tokens Tokens, limiters *ratelimit.Factory, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, tokens: tokens, writes: limiters.Closed("class_write", writesPerWindow, writeWindow), now: now,
		bookReads: limiters.Closed("class_book_read", bookReadsPerWindow, bookReadWindow),
		lookupAll: limiters.Open("class_invite_lookup", lookupAllPerWindow, lookupWindow), lookupMisses: limiters.Open("class_invite_lookup_miss", lookupMissesPerWindow, lookupWindow),
		acceptIP: limiters.Closed("class_invite_accept_ip", acceptPerIPWindow, acceptWindow)}
}

// throttle takes one write from the user's budget: a *RetryAfterError when it is used up, the limiter's own error (503) when
// the limiter store is down.
func (s *Service) throttle(ctx context.Context, userID uint64) error {
	ok, wait, err := s.writes.Take(ctx, strconv.FormatUint(userID, 10))
	if err != nil {
		return err
	}
	if !ok {
		return &RetryAfterError{Err: apperr.New(apperr.RateLimited, "too many requests, try again later"), After: wait}
	}
	return nil
}

// Authorize is the one access check every class endpoint starts with. It returns the class with the caller's membership when
// the caller is a member whose role is at least min. A class that does not exist and a class the caller is not in are both
// ErrNotFound (404, no existence leak); a member with a role below min is ErrForbidden (403). One SQL join on class_member_tab.
// Endpoints that change membership repeat the check under a lock inside their transaction (lockMembership).
func (s *Service) Authorize(ctx context.Context, userID uint64, classID, min string) (*Class, error) {
	c, err := s.store.access(ctx, userID, classID)
	if err != nil {
		return nil, err
	}
	if !atLeast(c.MyRole, min) {
		return nil, ErrForbidden
	}
	return c, nil
}

// Create makes a class for a user whose email is verified; the creator becomes its admin. At most maxAdminClasses as admin.
func (s *Service) Create(ctx context.Context, userID uint64, emailVerified bool, in *ClassInput) (*Class, error) {
	if !emailVerified {
		return nil, ErrEmailNotVerified
	}
	clean, err := in.clean()
	if err != nil {
		return nil, err
	}
	if err := s.throttle(ctx, userID); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	c := &Class{ID: ulid.New(now), Name: clean.Name, SchoolName: clean.SchoolName, GraduationYear: clean.GraduationYear, Language: clean.Language}
	if err := s.store.create(ctx, userID, c, ulid.New(now), now.UnixMilli()); err != nil {
		return nil, err
	}
	return c, nil
}

// List returns one page of the caller's classes (newest membership first) and the token of the next page ("" on the last).
func (s *Service) List(ctx context.Context, userID uint64, limit int, nextID string) ([]Class, string, error) {
	var after uint64
	if nextID != "" {
		var err error
		if after, err = openPos(s.tokens, classListScope(userID), nextID); err != nil {
			return nil, "", err
		}
	}
	out, next, err := s.store.list(ctx, userID, after, limit)
	if err != nil {
		return nil, "", err
	}
	return out, sealPos(s.tokens, classListScope(userID), next), nil
}

// Get returns the class with the caller's role and their own page status. It never carries the roster, whatever the role.
func (s *Service) Get(ctx context.Context, userID uint64, classID string) (*Class, error) {
	c, err := s.Authorize(ctx, userID, classID, RoleStudent)
	if err != nil {
		return nil, err
	}
	c.Page, err = s.store.pageStatus(ctx, userID, c.internalID)
	return c, err
}

// Update replaces the editable fields of the class (admin).
func (s *Service) Update(ctx context.Context, userID uint64, classID string, in *ClassInput) (*Class, error) {
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
	if err := s.store.update(ctx, userID, classID, clean, s.now().UnixMilli()); err != nil {
		return nil, err
	}
	return s.store.access(ctx, userID, classID)
}

// Delete removes the class and its memberships (admin). Members' yearbooks are not touched.
func (s *Service) Delete(ctx context.Context, userID uint64, classID string) error {
	if _, err := s.Authorize(ctx, userID, classID, RoleAdmin); err != nil {
		return err
	}
	if err := s.throttle(ctx, userID); err != nil {
		return err
	}
	return s.store.remove(ctx, userID, classID)
}

// Members returns one page of the roster (admin only), oldest member first, and the token of the next page.
func (s *Service) Members(ctx context.Context, userID uint64, classID string, limit int, nextID string) ([]Member, string, error) {
	c, err := s.Authorize(ctx, userID, classID, RoleAdmin)
	if err != nil {
		return nil, "", err
	}
	scope := memberListScope(userID, c.ID)
	var after uint64
	if nextID != "" {
		if after, err = openPos(s.tokens, scope, nextID); err != nil {
			return nil, "", err
		}
	}
	out, next, err := s.store.members(ctx, c.internalID, after, limit)
	if err != nil {
		return nil, "", err
	}
	return out, sealPos(s.tokens, scope, next), nil
}

// UpdateRole promotes or demotes a member (admin). The last admin cannot be demoted.
func (s *Service) UpdateRole(ctx context.Context, userID uint64, classID, memberID, role string) error {
	if _, err := s.Authorize(ctx, userID, classID, RoleAdmin); err != nil {
		return err
	}
	if !slices.Contains(roles, role) {
		return invalid("role")
	}
	if err := s.throttle(ctx, userID); err != nil {
		return err
	}
	return s.store.setRole(ctx, userID, classID, memberID, role, s.now().UnixMilli())
}

// RemoveMember takes a member out of the class (admin). The last admin cannot be removed.
func (s *Service) RemoveMember(ctx context.Context, userID uint64, classID, memberID string) error {
	if _, err := s.Authorize(ctx, userID, classID, RoleAdmin); err != nil {
		return err
	}
	if err := s.throttle(ctx, userID); err != nil {
		return err
	}
	return s.store.removeMember(ctx, userID, classID, memberID)
}

// Leave takes the caller out of the class (any member). The last admin cannot leave.
func (s *Service) Leave(ctx context.Context, userID uint64, classID string) error {
	if _, err := s.Authorize(ctx, userID, classID, RoleStudent); err != nil {
		return err
	}
	if err := s.throttle(ctx, userID); err != nil {
		return err
	}
	return s.store.leave(ctx, userID, classID)
}

// DeleteByYearbook implements yearbook.ChildPurger: when a yearbook is deleted, the member row that pointed at it forgets it
// (the member keeps the membership). Registered with the yearbook service in cmd/smemories-api.
func (s *Service) DeleteByYearbook(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	return s.store.clearBook(ctx, tx, yearbookID, s.now().UnixMilli())
}
