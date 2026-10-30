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
	want := `{"member_id":"m","name":"N","linked":false,"yearbook":null,"profile":null,"design":null,"notes":[],"note_count":0,"approved_total":0,"truncated":false}`
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
	b.Members, b.Truncated, err = s.encodeMembers(context.Background(), ms, MaxBookBytes-bookFrameBytes-len(class)-len(front)-2)
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
	if !strings.Contains(out.String(), `"truncated":[]`) {
		t.Fatal("truncated is not []")
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
	b.Members, b.Truncated, err = s.encodeMembers(context.Background(), ms, MaxBookBytes-bookFrameBytes-100<<10)
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
