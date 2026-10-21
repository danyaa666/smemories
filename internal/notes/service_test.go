package notes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/ratelimit"
)

// fakeStore is an in-memory store that keeps the ownership and revoked rules of the real one.
type fakeStore struct {
	mu       sync.Mutex
	owners   map[string]uint64    // yearbook public id -> owner
	byHash   map[[32]byte]*Public // active links
	revoked  map[string]int64     // collection id -> revoked_at
	created  []*Collection
	notes    []*newNote
	count    int // what noteCount reports
	storeErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{owners: map[string]uint64{book1: 1}, byHash: map[[32]byte]*Public{}, revoked: map[string]int64{}}
}

func (f *fakeStore) create(_ context.Context, ownerID uint64, bookID string, c *Collection, hash [32]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if o, ok := f.owners[bookID]; !ok || o != ownerID {
		return ErrNotFound
	}
	f.created = append(f.created, c)
	f.byHash[hash] = &Public{CollectionID: uint64(len(f.created)), YearbookID: 10, DeadlineAt: c.DeadlineAt}
	return nil
}

func (f *fakeStore) list(_ context.Context, ownerID uint64, bookID string) ([]*Collection, error) {
	if o, ok := f.owners[bookID]; !ok || o != ownerID {
		return nil, ErrNotFound
	}
	return f.created, nil
}

func (f *fakeStore) revoke(_ context.Context, ownerID uint64, id string, now int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.created {
		if c.ID == id && f.owners[book1] == ownerID {
			if _, done := f.revoked[id]; !done {
				f.revoked[id] = now
			}
			return nil
		}
	}
	return ErrNotFound
}

func (f *fakeStore) lookup(_ context.Context, hash [32]byte) (*Public, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.storeErr != nil {
		return nil, f.storeErr
	}
	p, ok := f.byHash[hash]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (f *fakeStore) noteCount(context.Context, uint64) (int, error) { return f.count, nil }

func (f *fakeStore) insertNote(_ context.Context, n *newNote) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notes = append(f.notes, n)
	return nil
}

// fakePhotos fails the failAt-th upload (1-based; 0 = never) and remembers what was discarded.
type fakePhotos struct {
	failAt, calls int
	err           error
	discarded     []media.Media
}

func (f *fakePhotos) UploadContributorFrom(_ context.Context, _ uint64, load func() ([]byte, error)) (*media.Media, error) {
	f.calls++
	if f.calls == f.failAt {
		return nil, f.err
	}
	if _, err := load(); err != nil {
		return nil, err
	}
	return &media.Media{RowID: int64(f.calls)}, nil
}

func (f *fakePhotos) Discard(ms ...media.Media) error {
	f.discarded = append(f.discarded, ms...)
	return nil
}

type fixture struct {
	svc    *Service
	st     *fakeStore
	photos *fakePhotos
	now    time.Time
}

func newFixture() *fixture {
	f := &fixture{st: newFakeStore(), photos: &fakePhotos{}, now: time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)}
	clock := func() time.Time { return f.now }
	f.svc = &Service{store: f.st, media: f.photos, logger: slog.New(slog.DiscardHandler), now: clock,
		misses: ratelimit.NewMemory(missesPerWindow, publicWindow, clock), all: ratelimit.NewMemory(allPerWindow, publicWindow, clock),
		subIPHr: ratelimit.NewMemory(submitPerIPHour, time.Hour, clock), subIPDay: ratelimit.NewMemory(submitPerIPDay, 24*time.Hour, clock),
		subColl: ratelimit.NewMemory(submitPerCollectionHour, time.Hour, clock)}
	return f
}

// book1 is a canonical public id (the services refuse any other spelling before they query).
const book1 = "01J9Z3K6V8Q4M7N2P5R8T0W1X1"

func wantCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	if err == nil || apperr.CodeOf(err) != code {
		t.Fatalf("error %v (code %v), want code %s", err, apperr.CodeOf(err), code.Name)
	}
}

func TestCreateCollectionRules(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	str := func(s string) *string { return &s }
	ms := func(d time.Duration) *int64 { v := f.now.Add(d).UnixMilli(); return &v }

	_, err := f.svc.CreateCollection(ctx, 1, false, &CreateInput{YearbookID: book1})
	wantCode(t, err, CodeEmailNotVerified)
	_, err = f.svc.CreateCollection(ctx, 1, true, &CreateInput{})
	wantCode(t, err, apperr.Param)
	_, err = f.svc.CreateCollection(ctx, 2, true, &CreateInput{YearbookID: book1}) // someone else's book
	wantCode(t, err, apperr.NotFound)
	_, err = f.svc.CreateCollection(ctx, 1, true, &CreateInput{YearbookID: "01J9Z3K6V8Q4M7N2P5R8T0W1X2"})
	wantCode(t, err, apperr.NotFound)
	for name, in := range map[string]*CreateInput{
		"label too long": {YearbookID: book1, Label: str(string(make([]rune, maxLabel+1)))},
		"control char":   {YearbookID: book1, Label: str("bell\a")},
	} {
		if _, err := f.svc.CreateCollection(ctx, 1, true, in); apperr.CodeOf(err) != CodeInvalidLabel {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, d := range map[string]*int64{"past": ms(-time.Hour), "now": ms(0), "over a year": ms(366*24*time.Hour + time.Hour), "zero": new(int64)} {
		if _, err := f.svc.CreateCollection(ctx, 1, true, &CreateInput{YearbookID: book1, DeadlineAt: d}); apperr.CodeOf(err) != CodeInvalidDeadline {
			t.Errorf("deadline %s: %v", name, err)
		}
	}
	if len(f.st.created) != 0 {
		t.Fatalf("%d links created by rejected requests", len(f.st.created))
	}
	c, err := f.svc.CreateCollection(ctx, 1, true, &CreateInput{YearbookID: book1, Label: str("  Class  "), DeadlineAt: ms(time.Hour)})
	if err != nil || len(c.Token) != tokenLen || c.Label != "Class" || c.CreatedAt != f.now.UnixMilli() {
		t.Fatalf("created %+v: %v", c, err)
	}
}

func TestResolveRules(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	deadline := f.now.Add(time.Hour).UnixMilli()
	c, err := f.svc.CreateCollection(ctx, 1, true, &CreateInput{YearbookID: book1, DeadlineAt: &deadline})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.Resolve(ctx, "ip", c.Token); err != nil {
		t.Fatalf("open link: %v", err)
	}
	// unknown, malformed and revoked links are one and the same error
	revoked, _ := f.svc.CreateCollection(ctx, 1, true, &CreateInput{YearbookID: book1})
	hash, _ := tokenHash(revoked.Token)
	delete(f.st.byHash, hash) // what the real store does for revoked_at IS NOT NULL
	for _, tok := range []string{"", "short", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", revoked.Token, c.Token + "x"} {
		if _, err := f.svc.Resolve(ctx, "ip", tok); err != ErrNotFound {
			t.Errorf("token %q: %v, want ErrNotFound", tok, err)
		}
	}
	// the deadline: a millisecond before it is open, at it the link is closed
	f.now = f.now.Add(time.Hour - time.Millisecond)
	if _, err := f.svc.Resolve(ctx, "ip2", c.Token); err != nil {
		t.Fatalf("just before the deadline: %v", err)
	}
	f.now = f.now.Add(time.Millisecond)
	if _, err := f.svc.Resolve(ctx, "ip2", c.Token); err != ErrClosed {
		t.Fatalf("at the deadline: %v, want ErrClosed", err)
	}
	// a store failure is not a miss: it gives the attempt back and surfaces as it is
	f.st.storeErr = errors.New("db down")
	if _, err := f.svc.Resolve(ctx, "ip3", c.Token); apperr.CodeOf(err) != apperr.Internal {
		t.Fatalf("store failure: %v", err)
	}
}

// Only misses count against the tight limit; hits count against the generous one; both answer with a wait time.
func TestResolveLimits(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	c, _ := f.svc.CreateCollection(ctx, 1, true, &CreateInput{YearbookID: book1})
	for range missesPerWindow {
		_, _ = f.svc.Resolve(ctx, "ip", "x")
	}
	_, err := f.svc.Resolve(ctx, "ip", c.Token)
	var ra *RetryAfterError
	if !errors.As(err, &ra) || ra.After <= 0 || apperr.CodeOf(err) != apperr.RateLimited {
		t.Fatalf("61st miss: %v", err)
	}
	for range allPerWindow { // hits never touch the miss counter
		if _, err := f.svc.Resolve(ctx, "other", c.Token); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.svc.Resolve(ctx, "other", c.Token); apperr.CodeOf(err) != apperr.RateLimited {
		t.Fatalf("hit 601: %v", err)
	}
}

func TestRevokeAndListOwnership(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	c, _ := f.svc.CreateCollection(ctx, 1, true, &CreateInput{YearbookID: book1})
	wantCode(t, f.svc.RevokeCollection(ctx, 2, c.ID), apperr.NotFound) // someone else's link
	wantCode(t, f.svc.RevokeCollection(ctx, 1, ""), apperr.Param)
	if err := f.svc.RevokeCollection(ctx, 1, c.ID); err != nil {
		t.Fatal(err)
	}
	first := f.st.revoked[c.ID]
	f.now = f.now.Add(time.Hour)
	if err := f.svc.RevokeCollection(ctx, 1, c.ID); err != nil || f.st.revoked[c.ID] != first {
		t.Fatalf("revoking again must succeed and keep the first time: %v", err)
	}
	_, err := f.svc.ListCollections(ctx, 2, book1)
	wantCode(t, err, apperr.NotFound)
	_, err = f.svc.ListCollections(ctx, 1, "")
	wantCode(t, err, apperr.Param)
}

func TestSubmitNoteRules(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	p := &Public{CollectionID: 7, YearbookID: 10}
	good := []byte(`{"name":"An","message":"hi"}`)
	submit := func(in *Submission) (string, error) {
		if in.IP == "" {
			in.IP = "ip"
		}
		return f.svc.SubmitNote(ctx, p, in)
	}

	if id, err := submit(&Submission{Answers: good}); err != nil || len(id) != 26 || len(f.st.notes) != 1 || f.st.notes[0].CreatedAt != f.now.UnixMilli() {
		t.Fatalf("id %q err %v notes %d", id, err, len(f.st.notes))
	}
	// the text is checked before any photo is touched, and a rejected one counts nothing against the rate limits
	_, err := submit(&Submission{Answers: []byte(`{"name":"An"}`), Photos: []func() ([]byte, error){func() ([]byte, error) { return nil, nil }}})
	wantCode(t, err, CodeMissingAnswer)
	_, err = submit(&Submission{Answers: []byte(`{"name":"An","message":"m","x<":"y"}`)})
	wantCode(t, err, apperr.Param)
	_, err = submit(&Submission{Answers: []byte(`["a"]`)})
	wantCode(t, err, apperr.Param)
	if f.photos.calls != 0 || len(f.st.notes) != 1 {
		t.Fatalf("a rejected text reached the photos (%d) or the store (%d)", f.photos.calls, len(f.st.notes))
	}
	// the abuse check
	_, err = submit(&Submission{Answers: good, Verify: func(context.Context) error { return errors.New("bot") }})
	wantCode(t, err, CodeVerificationFailed)
	// a full collection answers before the text is read
	f.st.count = maxNotes
	if _, err := submit(&Submission{Answers: []byte("garbage")}); err != ErrFull {
		t.Fatalf("full: %v", err)
	}
	f.st.count = 0
}

// A failing photo rejects the submission, discards the photos stored before it and stores no note; a busy media service gives
// the rate-limit hits back.
func TestSubmitNotePhotoFailures(t *testing.T) {
	f := newFixture()
	p := &Public{CollectionID: 7, YearbookID: 10}
	load := func() ([]byte, error) { return []byte("x"), nil }
	in := func() *Submission {
		return &Submission{IP: "ip", Answers: []byte(`{"name":"An","message":"hi"}`), Photos: []func() ([]byte, error){load, load, load}}
	}

	f.photos.failAt, f.photos.err = 3, media.ErrInvalidImage
	_, err := f.svc.SubmitNote(context.Background(), p, in())
	wantCode(t, err, CodeInvalidImage)
	if len(f.photos.discarded) != 2 || len(f.st.notes) != 0 {
		t.Fatalf("discarded %d photos, stored %d notes", len(f.photos.discarded), len(f.st.notes))
	}

	f.photos.calls, f.photos.discarded, f.photos.failAt, f.photos.err = 0, nil, 1, media.ErrBusy
	for range submitPerCollectionHour + 5 { // busy answers are free: the 60 per hour per link never run out
		_, err = f.svc.SubmitNote(context.Background(), p, in())
		var ra *RetryAfterError
		if !errors.As(err, &ra) || apperr.CodeOf(err) != CodeBusy || ra.After <= 0 {
			t.Fatalf("busy: %v", err)
		}
		f.photos.calls = 0
	}

	f.photos.failAt, f.photos.err = 1, media.ErrQuota
	_, err = f.svc.SubmitNote(context.Background(), p, in())
	wantCode(t, err, CodeQuotaExceeded)
	f.photos.calls, f.photos.err = 0, media.ErrUnsupported
	_, err = f.svc.SubmitNote(context.Background(), p, in())
	wantCode(t, err, CodeUnsupportedMediaType)
}

// takeSubmit counts per IP (hour and day) and per collection (hour); a refusal consumes nothing.
func TestTakeSubmit(t *testing.T) {
	f := newFixture()
	s := f.svc
	n := 0
	take := func(ip string) (bool, time.Duration) {
		n++
		return s.takeSubmit(context.Background(), ip, fmt.Sprint("collection", n))
	}

	for range 3 { // three hours of 100: the daily 300 is used up
		for range submitPerIPHour {
			if ok, _ := take("ip"); !ok {
				t.Fatal("refused under the limits")
			}
		}
		if ok, wait := take("ip"); ok || wait <= 0 {
			t.Fatalf("hourly limit not enforced: %v %v", ok, wait)
		}
		f.now = f.now.Add(time.Hour + time.Second)
	}
	if ok, wait := take("ip"); ok || wait < 20*time.Hour {
		t.Fatalf("daily limit not enforced: %v %v", ok, wait)
	}
	if ok, _ := take("another"); !ok {
		t.Fatal("the limit is per IP")
	}
	// one collection: 60 an hour, whoever sends
	for range submitPerCollectionHour {
		if ok, _ := s.takeSubmit(context.Background(), "a"+fmt.Sprint(n), "same"); !ok {
			t.Fatal("refused under the collection limit")
		}
		n++
	}
	if ok, _ := s.takeSubmit(context.Background(), "fresh", "same"); ok {
		t.Fatal("collection limit not enforced")
	}
	if ok, _ := s.takeSubmit(context.Background(), "fresh", "elsewhere"); !ok { // and the refused request used none of fresh's allowance
		t.Fatal("a refusal consumed the IP allowance")
	}
}

// T-092: the public photo link query repeats the approved status (media cannot import notes); this keeps the two together.
func TestStatusApprovedIsWhatPublicPhotoLinksRequire(t *testing.T) {
	if StatusApproved != media.ShareableNoteStatus {
		t.Fatalf("notes.StatusApproved %q != media.ShareableNoteStatus %q", StatusApproved, media.ShareableNoteStatus)
	}
}

// Audit M3: an id that is not a canonical ULID is ERROR_PARAM before the store is asked (the columns match any spelling).
func TestCollectionsRefuseNonCanonicalIDs(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	c, err := f.svc.CreateCollection(ctx, 1, true, &CreateInput{YearbookID: book1})
	if err != nil {
		t.Fatal(err)
	}
	created := len(f.st.created)
	for _, bad := range []string{strings.ToLower(book1), book1[:25], book1 + "A", " " + book1[1:], book1[:25] + "I"} {
		_, err := f.svc.CreateCollection(ctx, 1, true, &CreateInput{YearbookID: bad})
		wantCode(t, err, apperr.Param)
		_, err = f.svc.ListCollections(ctx, 1, bad)
		wantCode(t, err, apperr.Param)
		wantCode(t, f.svc.RevokeCollection(ctx, 1, bad), apperr.Param)
		wantCode(t, f.svc.RevokeCollection(ctx, 1, strings.ToLower(c.ID)), apperr.Param)
	}
	if len(f.st.created) != created || len(f.st.revoked) != 0 {
		t.Fatal("the store was touched by a refused id")
	}
}
