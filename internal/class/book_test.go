package class

import (
	"bytes"
	"context"
	"encoding/json"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/bookdesign"
	"github.com/danyaa666/smemories/internal/booklayout"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/notes"
	"github.com/danyaa666/smemories/internal/templates"
	"github.com/danyaa666/smemories/internal/yearbook"
)

type fakeBooks struct {
	y   *yearbook.Yearbook
	err error
}

func (f fakeBooks) Get(context.Context, uint64, string) (*yearbook.Yearbook, error) {
	return f.y, f.err
}

type fakeNotes struct {
	ns    []*notes.BookNote
	total int
	owner *uint64
}

func (f fakeNotes) ClassBookNotes(_ context.Context, owner uint64, _ string) ([]*notes.BookNote, int, error) {
	if f.owner != nil {
		*f.owner = owner
	}
	return f.ns, f.total, nil
}

func TestBookMember(t *testing.T) {
	photo := "01K7Q8W3ZR5N2M4X6V8B0C1D2E"
	named := &yearbook.Yearbook{PageSize: "A5", Language: "vi", Profile: yearbook.Profile{FullName: "Linh", Quote: "q"}}
	one := []*notes.BookNote{{ID: "n1", PhotoIDs: []string{}}}
	member := bookMember{id: "m1", name: "Linh", book: "b1", userID: 7}
	for name, tc := range map[string]struct {
		m                       bookMember
		y                       *yearbook.Yearbook
		err                     error
		ns                      []*notes.BookNote
		total                   int
		linked, truncated       bool
		wantNotes, wantApproved int
	}{
		"not linked":             {m: bookMember{id: "m1", name: "Linh"}},
		"book deleted meanwhile": {m: member, err: yearbook.ErrNotFound},
		"empty page":             {m: member, y: &yearbook.Yearbook{}},
		"photo only":             {m: member, y: &yearbook.Yearbook{Profile: yearbook.Profile{PhotoMediaID: &photo}}, linked: true},
		"profile, no notes":      {m: member, y: named, linked: true},
		"notes only":             {m: member, y: &yearbook.Yearbook{}, ns: one, total: 1, linked: true, wantNotes: 1, wantApproved: 1},
		"cut at the cap":         {m: member, y: named, ns: one, total: 41, linked: true, truncated: true, wantNotes: 1, wantApproved: 41},
	} {
		t.Run(name, func(t *testing.T) {
			s := &Service{books: fakeBooks{tc.y, tc.err}, notes: fakeNotes{ns: tc.ns, total: tc.total}}
			got, err := s.bookMember(context.Background(), tc.m)
			if err != nil {
				t.Fatal(err)
			}
			if got.Linked != tc.linked || got.Truncated != tc.truncated || got.NoteCount != tc.wantNotes || got.ApprovedTotal != tc.wantApproved {
				t.Fatalf("got %+v", got)
			}
			if got.Notes == nil || (got.Linked != (got.Profile != nil)) || (got.Linked != (got.Yearbook != nil)) {
				t.Fatalf("notes must be [] and profile and yearbook null exactly when not linked: %+v", got)
			}
		})
	}
}

// The notes of a page are read as the page's owner (the member row's user), never as the class admin who asked.
func TestBookMemberReadsAsTheOwner(t *testing.T) {
	var owner uint64
	s := &Service{books: fakeBooks{y: &yearbook.Yearbook{Profile: yearbook.Profile{FullName: "Linh"}}}, notes: fakeNotes{owner: &owner}}
	if _, err := s.bookMember(context.Background(), bookMember{id: "m", book: "b", userID: 42}); err != nil || owner != 42 {
		t.Fatalf("notes read as %d, %v", owner, err)
	}
}

// The wire shape of an unlinked member: arrays are [] and objects null, so a client never branches on a missing key.
func TestBookMemberJSON(t *testing.T) {
	s := &Service{books: fakeBooks{}, notes: fakeNotes{}}
	bm, _ := s.bookMember(context.Background(), bookMember{id: "m", name: "N"})
	b, _ := json.Marshal(bm)
	want := `{"member_id":"m","name":"N","linked":false,"yearbook":null,"profile":null,"design":null,"layout":null,"notes":[],"note_count":0,"approved_total":0,"truncated":false}`
	if string(b) != want {
		t.Fatalf("%s\nwant %s", b, want)
	}
}

// media repeats the admin role of this package (it cannot import it): the two must stay equal.
func TestMediaAdminRoleMatches(t *testing.T) {
	if media.ClassAdminRole != RoleAdmin {
		t.Fatalf("media.ClassAdminRole = %q, class.RoleAdmin = %q", media.ClassAdminRole, RoleAdmin)
	}
	if media.ShareableNoteStatus != noteApproved || !strings.EqualFold(noteApproved, notes.StatusApproved) {
		t.Fatal("the approved status drifted")
	}
}

// ---- T-133: the size cap and the streaming ------------------------------------------------------------------------------

// maximalNote is a note of the longest text in four-byte characters, with two photos: about 20 KB, the shape QA used to estimate 50 MiB.
func maximalNote(i int) *notes.BookNote {
	ans, _ := json.Marshal(map[string]string{"name": "Linh", "message": strings.Repeat("\U0001F600", 5000)})
	id := "01K7Q8W3ZR5N2M4X6V8B0C1D2E"
	return &notes.BookNote{ID: id[:20] + strconv.Itoa(100000+i), Answers: ans, PhotoIDs: []string{id, id}}
}

type countWriter struct{ n int }

func (c *countWriter) Write(p []byte) (int, error) { c.n += len(p); return len(p), nil }

// maximalClass is the class of the QA estimate: 60 members, each with 40 maximal approved notes and a profile at its longest.
func maximalClass() (*Service, []bookMember) {
	long := strings.Repeat("<", 1250)
	y := &yearbook.Yearbook{PageSize: "A4", Language: "vi", Profile: yearbook.Profile{FullName: long[:100], Nickname: long[:50], Quote: long[:500], Hobbies: long[:300], FuturePlans: long[:300]}}
	ns := make([]*notes.BookNote, notes.MaxClassBookNotes)
	for i := range ns {
		ns[i] = maximalNote(i)
	}
	ms := make([]bookMember, MaxMembers)
	for i := range ms {
		ms[i] = bookMember{id: "01K7Q8W3ZR5N2M4X6V8B0C" + strconv.Itoa(100+i), name: long[:100], book: "b", userID: uint64(i + 1)}
	}
	return &Service{books: fakeBooks{y: y}, notes: fakeNotes{ns: ns, total: len(ns)}}, ms
}

func worstPages() []Page {
	worst := Page{ID: "01K7Q8W3ZR5N2M4X6V8B0C1D2E", Kind: "custom", Title: strings.Repeat("<", maxPageTitle), Body: strings.Repeat("<", maxPageBody), Placement: PlacementFront}
	pages := make([]Page, MaxPages)
	for i := range pages {
		pages[i] = worst
	}
	return pages
}

// encodeClass is Service.Book without the database: the same budget, the same members, the same Encode.
func encodeClass(t *testing.T, s *Service, ms []bookMember) (*ClassBook, int) {
	t.Helper()
	b := &ClassBook{Class: BookClass{ID: "01K7Q8W3ZR5N2M4X6V8B0C1D2E", Name: strings.Repeat("<", 120), Language: "vi"}, FrontPages: worstPages(), BackPages: []Page{}}
	class, _ := json.Marshal(b.Class)
	front, _ := json.Marshal(b.FrontPages)
	var err error
	b.Members, b.Truncated, b.TruncatedLayouts, _, err = s.encodeMembers(context.Background(), ms, MaxBookBytes-bookFrameBytes-len(class)-len(front)-2)
	if err != nil {
		t.Fatal(err)
	}
	rec := &countWriter{}
	if err := b.Encode(rec); err != nil {
		t.Fatal(err)
	}
	return b, len("{\"status\":\"OK\",\"data\":") + rec.n + len("}\n")
}

// AC2: 60 members x 40 maximal notes: at most 8 MiB, the cut members listed, the earlier members complete and the later ones without notes.
func TestClassBookStaysUnderTheCap(t *testing.T) {
	s, ms := maximalClass()
	b, size := encodeClass(t, s, ms)
	t.Logf("%d bytes (%.2f MiB), %d members cut", size, float64(size)/(1<<20), len(b.Truncated))
	if size > MaxBookBytes {
		t.Fatalf("%d bytes is over the %d cap", size, MaxBookBytes)
	}
	if len(b.Truncated) == 0 || len(b.Truncated) == MaxMembers || len(b.Members) != MaxMembers {
		t.Fatalf("%d cut, %d members", len(b.Truncated), len(b.Members))
	}
	first := MaxMembers - len(b.Truncated)
	for i, raw := range b.Members {
		var m BookMember
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if complete := i < first; complete != (len(m.Notes) == notes.MaxClassBookNotes) || (!complete && (len(m.Notes) != 0 || m.NoteCount != 0 || !m.Truncated || m.ApprovedTotal != 40 || m.Profile == nil)) {
			t.Fatalf("member %d: %d notes, note_count %d, truncated %v", i, len(m.Notes), m.NoteCount, m.Truncated)
		}
		if i >= first && b.Truncated[i-first] != m.MemberID {
			t.Fatalf("truncated[%d] = %s, want %s", i-first, b.Truncated[i-first], m.MemberID)
		}
	}
	// the budget is tight: one more full member would not have fitted
	if size+len(b.Members[first-1]) < MaxBookBytes-bookFrameBytes-MaxMembers*strippedMemberMax {
		t.Fatalf("the cut is far below the cap: %d bytes", size)
	}
}

// The worst stripped member fits the reservation the cap makes for it.
func TestStrippedMemberFitsItsReserve(t *testing.T) {
	s, ms := maximalClass()
	bm, _ := s.bookMember(context.Background(), ms[0])
	bm.Notes, bm.NoteCount = []*notes.BookNote{}, 0
	if raw, _ := json.Marshal(bm); len(raw) > strippedMemberMax {
		t.Fatalf("a stripped member is %d bytes, the reserve is %d", len(raw), strippedMemberMax)
	}
}

// fakeDesigns answers the same resolved design for every member.
type fakeDesigns struct {
	d   *bookdesign.Resolved
	err error
}

func (f fakeDesigns) Resolved(context.Context, uint64, string, *string) (*bookdesign.Resolved, error) {
	return f.d, f.err
}

// worstDesign is the longest design any shipped template can resolve to: every text at its maximum in "<" (six bytes in JSON).
func worstDesign(t *testing.T) (*bookdesign.Resolved, string) {
	t.Helper()
	var worst *bookdesign.Resolved
	var id string
	size := 0
	for _, info := range templates.List() {
		d, ok := templates.Design(info.ID)
		if !ok {
			continue
		}
		texts := map[string]string{}
		for _, x := range d.Texts {
			texts[x.ID] = strings.Repeat("<", x.Max)
		}
		stored, _ := json.Marshal(map[string]any{"v": 1, "texts": texts})
		r := bookdesign.Resolve(d, stored)
		if raw, _ := json.Marshal(r); len(raw) > size {
			worst, id, size = r, info.ID, len(raw)
		}
	}
	if worst == nil {
		t.Fatal("no template has a design block")
	}
	return worst, id
}

// T-146: the design is in every member, also a stripped one, and the worst design still fits the reserve (with the worst profile).
func TestStrippedMemberWithDesignFitsItsReserve(t *testing.T) {
	d, _ := worstDesign(t)
	s, ms := maximalClass()
	s.designs = fakeDesigns{d: d}
	bm, err := s.bookMember(context.Background(), ms[0])
	if err != nil || bm.Design != d {
		t.Fatalf("%v %v", bm.Design, err)
	}
	bm.Notes, bm.NoteCount = []*notes.BookNote{}, 0
	if raw, _ := json.Marshal(bm); len(raw) > strippedMemberMax {
		t.Fatalf("a stripped member is %d bytes, the reserve is %d", len(raw), strippedMemberMax)
	}
}

// T-146: the cap holds for a class whose members all carry the worst design.
func TestClassBookStaysUnderTheCapWithDesigns(t *testing.T) {
	d, _ := worstDesign(t)
	s, ms := maximalClass()
	s.designs = fakeDesigns{d: d}
	b, size := encodeClass(t, s, ms)
	if size > MaxBookBytes || len(b.Truncated) == 0 || len(b.Members) != MaxMembers {
		t.Fatalf("%d bytes, %d cut, %d members", size, len(b.Truncated), len(b.Members))
	}
}

// A design read that says the book is gone leaves the member without a page; any other failure fails the request.
func TestBookMemberDesignErrors(t *testing.T) {
	s, ms := maximalClass()
	s.designs = fakeDesigns{err: apperr.New(apperr.NotFound, "yearbook not found")}
	if bm, err := s.bookMember(context.Background(), ms[0]); err != nil || bm.Linked || bm.Profile != nil {
		t.Fatalf("%+v %v", bm, err)
	}
	boom := apperr.New(apperr.Internal, "boom")
	s.designs = fakeDesigns{err: boom}
	if _, err := s.bookMember(context.Background(), ms[0]); err != boom {
		t.Fatalf("%v", err)
	}
}

// A normal class is never cut: truncated is [] (never null) and every note is in.
func TestNormalClassIsNotCut(t *testing.T) {
	s, ms := maximalClass()
	s.notes = fakeNotes{ns: []*notes.BookNote{maximalNote(1)}, total: 1}
	b, size := encodeClass(t, s, ms[:30])
	if len(b.Truncated) != 0 || b.Truncated == nil || size > MaxBookBytes {
		t.Fatalf("%v, %d bytes", b.Truncated, size)
	}
	var out bytes.Buffer
	_ = b.Encode(&out)
	if !strings.Contains(out.String(), `"truncated":[]`) || !strings.Contains(out.String(), `"truncated_layouts":[]`) {
		t.Fatal("truncated and truncated_layouts are not []")
	}
}

// Encode writes exactly what json.Marshal makes of the same ClassBook, so the two cannot drift apart.
func TestEncodeMatchesMarshal(t *testing.T) {
	s, ms := maximalClass()
	b, _ := encodeClass(t, s, ms)
	var got bytes.Buffer
	if err := b.Encode(&got); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(b)
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("Encode differs from json.Marshal")
	}
}

// heapObjects is the bytes of heap objects now, live or not yet collected (HeapAlloc), read without stopping the world.
func heapObjects() int {
	sample := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(sample)
	return int(sample[0].Value.Uint64())
}

// AC3: building and writing the maximal class never holds more than 3x the response on top of what was there before. The body goes
// to a writer that keeps nothing, as the HTTP server does. The collector runs often so that garbage is not counted as peak (it would
// make the number depend on the sync.Pool of encoding/json, which -race empties at random). Run under -race in CI.
func TestMaximalClassPeakAllocation(t *testing.T) {
	s, ms := maximalClass() // the fixture (50 MiB of notes) is built before the measurement
	defer debug.SetGCPercent(debug.SetGCPercent(5))
	runtime.GC()
	base := heapObjects()
	var peak atomic.Int64
	note := func() {
		for cur := int64(heapObjects() - base); cur > peak.Load() && !peak.CompareAndSwap(peak.Load(), cur); {
		}
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for {
			note()
			select {
			case <-stop:
				return
			case <-time.After(200 * time.Microsecond):
			}
		}
	}()
	b := &ClassBook{FrontPages: worstPages(), BackPages: []Page{}}
	var err error
	b.Members, b.Truncated, b.TruncatedLayouts, _, err = s.encodeMembers(context.Background(), ms, MaxBookBytes-bookFrameBytes-100<<10)
	w := &countWriter{}
	if err == nil {
		err = b.Encode(w)
	}
	note()
	close(stop)
	<-done
	if err != nil {
		t.Fatal(err)
	}
	got := int(peak.Load())
	t.Logf("response %d bytes, peak %d (%.2fx)", w.n, got, float64(got)/float64(w.n))
	if got > 3*w.n {
		t.Fatalf("peak %d bytes for a %d byte response (limit 3x)", got, w.n)
	}
}

// ---- T-167: the members' layouts in the class book ----------------------------------------------------------------------------

// fakeLayouts answers a layout document per owner (nil when the owner has none), and remembers who it was read as.
type fakeLayouts struct {
	docs  map[uint64]json.RawMessage
	err   error
	owner *uint64
}

func (f fakeLayouts) Resolved(_ context.Context, owner uint64, _ string, _ *string) (*booklayout.Resolved, error) {
	if f.owner != nil {
		*f.owner = owner
	}
	if f.err != nil || f.docs[owner] == nil {
		return nil, f.err
	}
	return &booklayout.Resolved{Layout: f.docs[owner], Version: 9}, nil
}

// layoutDoc is a layout document of n bytes (a stand-in: the class book never parses it, the layout service validated it).
func layoutDoc(n int) json.RawMessage {
	const head = `{"v":1,"pad":"`
	return json.RawMessage(head + strings.Repeat("x", n-len(head)-2) + `"}`)
}

// maxLayout is the largest layout the layout service stores (128 KiB).
const maxLayout = 128 << 10

func allLayouts(doc json.RawMessage) fakeLayouts {
	f := fakeLayouts{docs: map[uint64]json.RawMessage{}}
	for i := range MaxMembers {
		f.docs[uint64(i+1)] = doc
	}
	return f
}

// The layout is read as the book's owner, sent as the document only (no version), and null when the owner has none or it no longer
// validates (the service answers nil: the template changed, the document is stale).
func TestBookMemberLayout(t *testing.T) {
	var owner uint64
	doc := layoutDoc(300)
	y := &yearbook.Yearbook{TemplateID: new(string), Profile: yearbook.Profile{FullName: "Linh"}}
	s := &Service{books: fakeBooks{y: y}, notes: fakeNotes{}, layouts: fakeLayouts{docs: map[uint64]json.RawMessage{42: doc}, owner: &owner}}
	bm, err := s.bookMember(context.Background(), bookMember{id: "m", book: "b", userID: 42})
	if err != nil || owner != 42 || string(bm.Layout) != string(doc) {
		t.Fatalf("read as %d, %v, %s", owner, err, bm.Layout)
	}
	if raw, _ := json.Marshal(bm); strings.Contains(string(raw), "version") {
		t.Fatalf("only the document is sent: %s", raw)
	}
	bm, err = s.bookMember(context.Background(), bookMember{id: "m", book: "b", userID: 43}) // no layout, or one that no longer validates
	if raw, _ := json.Marshal(bm); err != nil || bm.Layout != nil || !strings.Contains(string(raw), `"layout":null`) {
		t.Fatalf("%v %s", err, raw)
	}
	// an unlinked member has none, and is not even read
	s.layouts = fakeLayouts{docs: map[uint64]json.RawMessage{0: doc}, err: apperr.New(apperr.Internal, "must not be read")}
	if bm, err = s.bookMember(context.Background(), bookMember{id: "m", name: "N"}); err != nil || bm.Layout != nil {
		t.Fatalf("%v %s", err, bm.Layout)
	}
	// a book deleted meanwhile is no page; any other failure fails the request
	s.layouts = fakeLayouts{err: apperr.New(apperr.NotFound, "yearbook not found")}
	if bm, err = s.bookMember(context.Background(), bookMember{id: "m", book: "b", userID: 1}); err != nil || bm.Linked || bm.Layout != nil {
		t.Fatalf("%+v %v", bm, err)
	}
	boom := apperr.New(apperr.Internal, "boom")
	s.layouts = fakeLayouts{err: boom}
	if _, err = s.bookMember(context.Background(), bookMember{id: "m", book: "b", userID: 1}); err != boom {
		t.Fatalf("%v", err)
	}
}

// The reserve proof is unchanged: a member with no notes and no layout (the worst profile) fits strippedMemberMax.
func TestStrippedMemberIgnoresLayout(t *testing.T) {
	s, ms := maximalClass()
	s.layouts = allLayouts(layoutDoc(maxLayout))
	bm, _ := s.bookMember(context.Background(), ms[0])
	if len(bm.Layout) != maxLayout {
		t.Fatalf("layout %d bytes", len(bm.Layout))
	}
	bm.Notes, bm.NoteCount, bm.Layout = []*notes.BookNote{}, 0, nil
	if raw, _ := json.Marshal(bm); len(raw) > strippedMemberMax {
		t.Fatalf("a stripped member is %d bytes, the reserve is %d", len(raw), strippedMemberMax)
	}
}

// AC2: 60 members with a 128 KiB layout and 40 maximal notes each: at most 8 MiB, layouts dropped from some member on (never a later
// one kept), every dropped member listed in book order, and every kept layout is whole.
func TestClassBookStaysUnderTheCapWithMaximalLayouts(t *testing.T) {
	s, ms := maximalClass()
	doc := layoutDoc(maxLayout)
	s.layouts = allLayouts(doc)
	b, size := encodeClass(t, s, ms)
	t.Logf("%d bytes (%.2f MiB), %d notes cut, %d layouts dropped", size, float64(size)/(1<<20), len(b.Truncated), len(b.TruncatedLayouts))
	if size > MaxBookBytes || len(b.Members) != MaxMembers || len(b.TruncatedLayouts) == 0 || len(b.TruncatedLayouts) == MaxMembers {
		t.Fatalf("%d bytes, %d members, %d layouts dropped", size, len(b.Members), len(b.TruncatedLayouts))
	}
	kept := MaxMembers - len(b.TruncatedLayouts)
	for i, raw := range b.Members {
		var m BookMember
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if (i < kept) != (string(m.Layout) == string(doc)) || (i >= kept && (string(m.Layout) != "null" || b.TruncatedLayouts[i-kept] != m.MemberID)) {
			t.Fatalf("member %d: layout %d bytes, truncated_layouts %v", i, len(m.Layout), b.TruncatedLayouts)
		}
	}
	// layouts come first: while they are kept the notes were cut before the layout of the same member was
	for i, raw := range b.Members[:kept] {
		var m BookMember
		_ = json.Unmarshal(raw, &m)
		if len(m.Notes) == 0 && !m.Truncated {
			t.Fatalf("member %d lost notes without being listed", i)
		}
	}
}

// The cap holds with layouts of every size from nothing to the maximum, with and without notes, and a small layout that fits is never
// dropped (a normal class loses nothing).
func TestClassBookCapWithLayoutSizes(t *testing.T) {
	for _, n := range []int{200, 2 << 10, 16 << 10, 64 << 10, maxLayout} {
		s, ms := maximalClass()
		s.layouts = allLayouts(layoutDoc(n))
		b, size := encodeClass(t, s, ms)
		if size > MaxBookBytes || len(b.Members) != MaxMembers {
			t.Fatalf("layout %d: %d bytes", n, size)
		}
		s.notes = fakeNotes{}
		b, size = encodeClass(t, s, ms)
		if size > MaxBookBytes {
			t.Fatalf("layout %d, no notes: %d bytes", n, size)
		}
		if n <= 2<<10 && len(b.TruncatedLayouts) != 0 {
			t.Fatalf("layout %d without notes lost %d layouts", n, len(b.TruncatedLayouts))
		}
	}
	// a normal class: 30 members, a few KiB of layout and one note each
	s, ms := maximalClass()
	s.notes = fakeNotes{ns: []*notes.BookNote{maximalNote(1)}, total: 1}
	s.layouts = allLayouts(layoutDoc(8 << 10))
	b, _ := encodeClass(t, s, ms[:30])
	if len(b.Truncated) != 0 || len(b.TruncatedLayouts) != 0 || b.TruncatedLayouts == nil {
		t.Fatalf("a normal class lost %v %v", b.Truncated, b.TruncatedLayouts)
	}
}

// QA T-133 boundary with layouts: a member whose layout (notes cut) just fits keeps it and loses the notes; one byte less and the layout
// goes, the notes come back if they fit; a member that is only a layout is dropped at the same boundary and listed once.
func TestClassBookLayoutBoundary(t *testing.T) {
	ns := []*notes.BookNote{maximalNote(1)}
	y := &yearbook.Yearbook{PageSize: "A4", Language: "vi", Profile: yearbook.Profile{FullName: "A"}}
	doc := layoutDoc(60 << 10) // bigger than the note, so that dropping the layout is what lets the note in
	ms := []bookMember{{id: "01K7Q8W3ZR5N2M4X6V8B0C1001", name: "A", book: "b", userID: 1}}
	build := func(withNotes bool) *Service {
		s := &Service{books: fakeBooks{y: y}, layouts: fakeLayouts{docs: map[uint64]json.RawMessage{1: doc}}}
		s.notes = notesByOwner{}
		if withNotes {
			s.notes = notesByOwner{1: ns}
		}
		return s
	}
	ctx := context.Background()
	s := build(true)
	full, cut, cutL, in, err := s.encodeMembers(ctx, ms, 1<<40)
	if err != nil || len(cut) != 0 || len(cutL) != 0 || in != 1 {
		t.Fatalf("%v %v %v %d", err, cut, cutL, in)
	}
	// budget == the whole member: nothing is cut
	if _, cut, cutL, _, _ = s.encodeMembers(ctx, ms, len(full[0])+1); len(cut) != 0 || len(cutL) != 0 {
		t.Fatalf("exact: %v %v", cut, cutL)
	}
	// one byte less: the notes go (the layout is paid first), the layout stays
	got, cut, cutL, in, _ := s.encodeMembers(ctx, ms, len(full[0]))
	var m BookMember
	_ = json.Unmarshal(got[0], &m)
	if len(cut) != 1 || len(cutL) != 0 || in != 1 || string(m.Layout) != string(doc) || len(m.Notes) != 0 || !m.Truncated {
		t.Fatalf("notes cut: %v %v %d %+v", cut, cutL, in, m)
	}
	// the budget cannot hold the layout: it is dropped instead, and the notes (which fit alone) stay
	noLayout := &Service{books: s.books, notes: s.notes}
	plain, _, _, _, _ := noLayout.encodeMembers(ctx, ms, 1<<40)
	got, cut, cutL, in, _ = s.encodeMembers(ctx, ms, len(plain[0])+1)
	m = BookMember{}
	_ = json.Unmarshal(got[0], &m)
	if len(cut) != 0 || len(cutL) != 1 || cutL[0] != ms[0].id || in != 0 || string(m.Layout) != "null" || len(m.Notes) != 1 {
		t.Fatalf("layout dropped, notes kept: %v %v %d %+v", cut, cutL, in, m)
	}
	// a member that is only a layout (no notes): dropped at the same boundary, listed once, never in truncated
	s = build(false)
	only, _, _, _, _ := s.encodeMembers(ctx, ms, 1<<40)
	_, cut, cutL, in, _ = s.encodeMembers(ctx, ms, len(only[0])) // one byte short
	if len(cut) != 0 || len(cutL) != 1 || in != 0 {
		t.Fatalf("layout only: %v %v %d", cut, cutL, in)
	}
	if _, cut, cutL, in, _ = s.encodeMembers(ctx, ms, len(only[0])+1); len(cut)+len(cutL) != 0 || in != 1 {
		t.Fatalf("layout only fits: %v %v %d", cut, cutL, in)
	}
}

// Encode writes truncated_layouts between truncated and the end, [] when none, and the same bytes as json.Marshal.
func TestEncodeTruncatedLayouts(t *testing.T) {
	b := &ClassBook{FrontPages: []Page{}, BackPages: []Page{}, Members: []json.RawMessage{}, Truncated: []string{}, TruncatedLayouts: []string{"01K7Q8W3ZR5N2M4X6V8B0C1001"}}
	var got bytes.Buffer
	_ = b.Encode(&got)
	want, _ := json.Marshal(b)
	if !bytes.Equal(got.Bytes(), want) || !strings.HasSuffix(got.String(), `"truncated":[],"truncated_layouts":["01K7Q8W3ZR5N2M4X6V8B0C1001"]}`) {
		t.Fatalf("%s", got.Bytes())
	}
}
