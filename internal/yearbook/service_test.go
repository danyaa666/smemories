package yearbook

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/httpx"
)

// fakeStore records what the service asks of the store; books maps owner -> public id -> book.
type fakeStore struct {
	books     map[uint64]map[string]Yearbook
	created   []Yearbook
	createErr error
	gotLimit  int
	gotAfter  *cursor
	nextOut   *cursor
	modifiedN int64 // the nowMs of the last modify
	removed   []string
	order     *[]string
}

func (f *fakeStore) note(s string) {
	if f.order != nil {
		*f.order = append(*f.order, s)
	}
}

func (f *fakeStore) create(_ context.Context, _ uint64, y *Yearbook) error {
	f.created = append(f.created, *y)
	return f.createErr
}

func (f *fakeStore) get(_ context.Context, owner uint64, id string) (*Yearbook, error) {
	f.note("get")
	if y, ok := f.books[owner][id]; ok {
		return &y, nil // y is a copy, so the stored book is untouched
	}
	return nil, ErrNotFound // also another owner's book: the fake, like the SQL, is scoped by owner
}

func (f *fakeStore) list(_ context.Context, _ uint64, c *cursor, limit int) ([]Yearbook, *cursor, error) {
	f.gotLimit, f.gotAfter = limit, c
	return nil, f.nextOut, nil
}

func (f *fakeStore) modify(_ context.Context, owner uint64, id string, nowMs int64, change func(*Yearbook) error) (*Yearbook, error) {
	y, err := f.get(context.Background(), owner, id)
	if err != nil {
		return nil, err
	}
	if err := change(y); err != nil {
		return nil, err
	}
	f.modifiedN, y.UpdatedAt = nowMs, nowMs
	return y, nil
}

func (f *fakeStore) remove(_ context.Context, _ uint64, y *Yearbook, _ []ChildPurger) error {
	f.note("remove")
	f.removed = append(f.removed, y.ID)
	return nil
}

type fakePurger struct {
	err   error
	order *[]string
}

func (p fakePurger) PurgeYearbook(context.Context, string) error {
	*p.order = append(*p.order, "purge")
	return p.err
}

var testNow = time.Date(2026, 10, 7, 12, 0, 0, 123_000_000, time.UTC)

func newTestService(f *fakeStore, p Purger) *Service {
	return &Service{store: f, tokens: httpx.NewCursors([]byte("0123456789abcdef0123456789abcdef")), purger: p, now: func() time.Time { return testNow }}
}

func ptr[T any](v T) *T { return &v }

func TestServiceCreateDefaultsAndRequiredFields(t *testing.T) {
	f := &fakeStore{}
	s := newTestService(f, nil)
	y, err := s.Create(context.Background(), 7, "Đặng Thị Hồng", &BookInput{Title: ptr("Lớp 12A"), Language: ptr("vi")})
	if err != nil {
		t.Fatal(err)
	}
	if y.PageSize != "A5" || y.Title != "Lớp 12A" || y.Language != "vi" || y.GraduationYear != nil {
		t.Errorf("defaults: %+v", y)
	}
	if y.CreatedAt != testNow.UnixMilli() || y.UpdatedAt != y.CreatedAt {
		t.Errorf("times %d/%d, want %d", y.CreatedAt, y.UpdatedAt, testNow.UnixMilli())
	}
	if len(y.ID) != publicIDLen || len(y.Profile.ID) != publicIDLen || y.ID == y.Profile.ID {
		t.Errorf("ids %q %q", y.ID, y.Profile.ID)
	}
	if p := y.Profile; !p.IsOwner || p.FullName != "Đặng Thị Hồng" {
		t.Errorf("owner profile: %+v", p)
	}
	if len(f.created) != 1 {
		t.Fatalf("store.create called %d times", len(f.created))
	}

	for _, tc := range []struct {
		name string
		in   BookInput
		want string
	}{
		{"no title", BookInput{Language: ptr("en")}, "ERROR_INVALID_TITLE"},
		{"no language", BookInput{Title: ptr("T")}, "ERROR_INVALID_LANGUAGE"},
		{"bad language", BookInput{Title: ptr("T"), Language: ptr("fr")}, "ERROR_INVALID_LANGUAGE"},
		{"a new book has no media", BookInput{Title: ptr("T"), Language: ptr("en"), CoverMediaID: optional[string]{Set: true, Val: ptr("01J9Z3K6V8Q4M7N2P5R8T0W1XY")}}, "ERROR_INVALID_MEDIA"},
	} {
		f.created = nil
		if _, err := s.Create(context.Background(), 7, "x", &tc.in); code(err) != tc.want {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.want)
		}
		if len(f.created) != 0 {
			t.Errorf("%s: a rejected book reached the store", tc.name)
		}
	}
}

func TestServiceCreateLimitReachedPassesThrough(t *testing.T) {
	s := newTestService(&fakeStore{createErr: ErrLimitReached}, nil)
	_, err := s.Create(context.Background(), 7, "x", &BookInput{Title: ptr("T"), Language: ptr("en")})
	if got := apperr.CodeOf(err); got != CodeLimitReached || got.HTTP != 409 {
		t.Errorf("code %+v", got)
	}
}

func TestServiceOwnershipScoping(t *testing.T) {
	const id = "01J9Z3K6V8Q4M7N2P5R8T0W1XY"
	f := &fakeStore{books: map[uint64]map[string]Yearbook{1: {id: {ID: id, Title: "Alice"}}}}
	s := newTestService(f, nil)
	ctx := context.Background()
	if _, err := s.Get(ctx, 1, id); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"get":    func() error { _, err := s.Get(ctx, 2, id); return err }(),
		"update": func() error { _, err := s.Update(ctx, 2, id, &BookInput{Title: ptr("Mine now")}); return err }(),
		"profile": func() error {
			_, err := s.UpdateProfile(ctx, 2, id, &ProfileInput{FullName: "Mine now"})
			return err
		}(),
		"delete": s.Delete(ctx, 2, id),
	} {
		if !apperr.Is(err, apperr.NotFound) {
			t.Errorf("%s of another owner's book: %v, want not found", name, err)
		}
	}
	if len(f.removed) != 0 {
		t.Error("another owner's book was removed")
	}
}

func TestServiceUpdateIsPartialAndStampsTheClock(t *testing.T) {
	const id = "01J9Z3K6V8Q4M7N2P5R8T0W1XY"
	f := &fakeStore{books: map[uint64]map[string]Yearbook{1: {id: {ID: id, Title: "Old", Motto: "keep", GraduationYear: ptr(2020)}}}}
	s := newTestService(f, nil)
	y, err := s.Update(context.Background(), 1, id, &BookInput{Title: ptr("New"), GraduationYear: optional[int]{Set: true}})
	if err != nil {
		t.Fatal(err)
	}
	if y.Title != "New" || y.Motto != "keep" || y.GraduationYear != nil || y.UpdatedAt != testNow.UnixMilli() || f.modifiedN != testNow.UnixMilli() {
		t.Errorf("update: %+v", y)
	}
	if _, err := s.Update(context.Background(), 1, id, &BookInput{Title: ptr(strings.Repeat("a", maxTitle+1))}); code(err) != "ERROR_INVALID_TITLE" {
		t.Errorf("over-long title: %v", err)
	}
	p, err := s.UpdateProfile(context.Background(), 1, id, &ProfileInput{FullName: "Hồng", PhotoMediaID: ptr("01J9Z3K6V8Q4M7N2P5R8T0W1XZ")})
	if err != nil || p.Profile.FullName != "Hồng" || p.Profile.PhotoMediaID == nil {
		t.Errorf("profile: %+v %v", p, err)
	}
}

func TestServiceListPageTokens(t *testing.T) {
	f := &fakeStore{nextOut: &cursor{testNow.UnixMilli(), "01J9Z3K6V8Q4M7N2P5R8T0W1XY"}}
	s := newTestService(f, nil)
	ctx := context.Background()
	books, next, err := s.List(ctx, 1, 20, "")
	if err != nil || books == nil || len(books) != 0 || next == "" || f.gotLimit != 20 || f.gotAfter != nil {
		t.Fatalf("first page: %v %q %v limit=%d after=%v", books, next, err, f.gotLimit, f.gotAfter)
	}
	f.nextOut = nil
	_, next, err = s.List(ctx, 1, 5, next)
	if err != nil || next != "" || f.gotAfter == nil || f.gotAfter.updatedAt != testNow.UnixMilli() || f.gotAfter.publicID != "01J9Z3K6V8Q4M7N2P5R8T0W1XY" {
		t.Fatalf("second page: %q %v after=%+v", next, err, f.gotAfter)
	}

	f.nextOut = &cursor{1, "01J9Z3K6V8Q4M7N2P5R8T0W1XY"}
	_, token, _ := s.List(ctx, 1, 20, "")
	flipped := []byte(token)
	flipped[len(flipped)/2] ^= 1
	for name, bad := range map[string]string{"garbage": "garbage", "bang": "!!", "tampered": string(flipped), "truncated": token[:len(token)-4], "another user's": mustToken(t, s, 2)} {
		if _, _, err := s.List(ctx, 1, 20, bad); !apperr.Is(err, apperr.Param) {
			t.Errorf("%s token: %v, want ERROR_PARAM", name, err)
		}
	}
	// A token that opens but holds nonsense is refused as well (a bug or a key that signs something else).
	for _, pos := range []string{"", "x.y", "12.short", "99999999999999999.01J9Z3K6V8Q4M7N2P5R8T0W1XY"} {
		if _, err := openCursor(s.tokens, 1, s.tokens.Seal(cursorScope(1), pos)); !apperr.Is(err, apperr.Param) {
			t.Errorf("position %q: %v", pos, err)
		}
	}
}

func mustToken(t *testing.T, s *Service, owner uint64) string {
	t.Helper()
	return sealCursor(s.tokens, owner, &cursor{1, "01J9Z3K6V8Q4M7N2P5R8T0W1XY"})
}

func TestServiceDeletePurgesStorageBeforeTheTransaction(t *testing.T) {
	const id = "01J9Z3K6V8Q4M7N2P5R8T0W1XY"
	var order []string
	f := &fakeStore{books: map[uint64]map[string]Yearbook{1: {id: {ID: id}}}, order: &order}
	s := newTestService(f, fakePurger{order: &order})
	if err := s.Delete(context.Background(), 1, id); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(order, ","); got != "get,purge,remove" {
		t.Errorf("order %s, want get,purge,remove", got)
	}

	order, f.removed = nil, nil
	s = newTestService(f, fakePurger{err: errors.New("s3 down"), order: &order})
	err := s.Delete(context.Background(), 1, id)
	if got := apperr.CodeOf(err); got != CodeStorage || got.HTTP != 502 {
		t.Errorf("code %+v, want ERROR_STORAGE_ERROR 502", got)
	}
	if len(f.removed) != 0 {
		t.Error("the book was removed although the object store failed")
	}
}
