package bookexport

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/medialink"
	"github.com/danyaa666/smemories/internal/notes"
	"github.com/danyaa666/smemories/internal/yearbook"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

const (
	bookID  = "01JZZZZZZZZZZZZZZZZZZZZZZZ"
	baseURL = "https://app.example.com"
)

var fixedNow = time.Date(2026, 10, 19, 23, 59, 0, 0, time.UTC)

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
	err   error
}

func (f fakeNotes) ExportNotes(context.Context, uint64, bool, string) ([]*notes.BookNote, int, error) {
	return f.ns, f.total, f.err
}

func ptr[T any](v T) *T { return &v }

func bn(answers string, photos ...string) *notes.BookNote {
	return &notes.BookNote{ID: "01JNOTEIDNOTEIDNOTEIDNOTE0", Answers: []byte(answers), PhotoIDs: append([]string{}, photos...)}
}

func export(t *testing.T, y *yearbook.Yearbook, ns []*notes.BookNote, total int, lang string) *Export {
	t.Helper()
	signer, _ := medialink.New(bytes.Repeat([]byte{7}, medialink.MinSecretBytes))
	s := NewService(fakeBooks{y: y}, fakeNotes{ns: ns, total: total}, signer, baseURL, nil, func() time.Time { return fixedNow })
	e, err := s.GetText(context.Background(), 1, true, bookID, lang)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func fullBook() *yearbook.Yearbook {
	return &yearbook.Yearbook{
		ID: bookID, Title: "Lớp 12A1 — Kỷ yếu", SchoolName: "THPT Nguyễn Huệ", ClassName: "12A1", GraduationYear: ptr(2026),
		Motto: "Học, học nữa, học mãi", Language: "vi", TemplateID: ptr("memory-book"), CoverMediaID: ptr("01JCCCCCCCCCCCCCCCCCCCCCCC"), LinkEpoch: 2,
		Profile: yearbook.Profile{FullName: "Nguyễn Thị Ánh Tuyết", Nickname: "Tuyết", Quote: "Cười lên nào 😊", Hobbies: "Vẽ\nĐọc sách",
			FuturePlans: "Bác sĩ", PhotoMediaID: ptr("01JPPPPPPPPPPPPPPPPPPPPPPP")},
	}
}

func fullNotes() []*notes.BookNote {
	return []*notes.BookNote{
		bn(`{"name":"Trần Văn Ngô","message":"Dòng một\n\nDòng ba 🎉","how_we_met":"Ở lớp 10","wish":"Luôn vui"}`,
			"01JAAAAAAAAAAAAAAAAAAAAAAA", "01JBBBBBBBBBBBBBBBBBBBBBBB", "01JCCCCCCCCCCCCCCCCCCCCCCC", "01JDDDDDDDDDDDDDDDDDDDDDDD"),
		bn(`{"name":"Lê Thị Hoà","message":"Chúc bạn thành công"}`),
		bn(`{"name":"Phạm Anh","relationship":"bạn cùng bàn","message":"Giữ liên lạc nhé","best_memory":"Chuyến đi Đà Lạt"}`, "01JEEEEEEEEEEEEEEEEEEEEEEE"),
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".txt")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from the golden file (run go test -update and read the diff):\n%s", name, got)
	}
}

// AC1: golden files in both languages for a full book, a book with no template, an empty book and a cut book.
func TestGolden(t *testing.T) {
	notemplate := fullBook()
	notemplate.TemplateID, notemplate.CoverMediaID = nil, nil
	empty := &yearbook.Yearbook{ID: bookID, Title: "Empty", Language: "en"}
	cases := map[string]struct {
		y     *yearbook.Yearbook
		ns    []*notes.BookNote
		total int
	}{
		"full":       {fullBook(), fullNotes(), 3},
		"notemplate": {notemplate, fullNotes(), 3}, // default fields: name, relationship, message; the rest follow the template's
		"empty":      {empty, nil, 0},
		"cut":        {empty, fullNotes()[:2], 1500},
	}
	for name, c := range cases {
		for _, lang := range []string{"en", "vi"} {
			t.Run(name+"_"+lang, func(t *testing.T) {
				e := export(t, c.y, c.ns, c.total, lang)
				golden(t, name+"_"+lang, e.Text)
				checkText(t, e.Text)
			})
		}
	}
}

// checkText holds for every document: UTF-8, NFC, \n only, no BOM, no trailing blanks, no internal id or e-mail address.
func checkText(t *testing.T, text string) {
	t.Helper()
	if !utf8.ValidString(text) || !norm.NFC.IsNormalString(text) || strings.ContainsRune(text, '\r') || strings.HasPrefix(text, "\xef\xbb\xbf") ||
		!strings.HasSuffix(text, "\n") {
		t.Error("text is not NFC UTF-8 with \\n line ends and no BOM")
	}
	for _, ln := range strings.Split(text, "\n") {
		if ln != strings.TrimRight(ln, " \t") {
			t.Errorf("trailing blank in %q", ln)
		}
	}
	if strings.Contains(text, bookID) || strings.Contains(text, "01JNOTEIDNOTEIDNOTEIDNOTE0") || strings.Contains(text, "@") {
		t.Error("text holds an internal id or an e-mail address")
	}
}

// The links are the T-092 tokens of the book's current epoch; a note shows at most three photos.
func TestLinks(t *testing.T) {
	e := export(t, fullBook(), fullNotes(), 3, "en")
	signer, _ := medialink.New(bytes.Repeat([]byte{7}, medialink.MinSecretBytes))
	re := regexp.MustCompile(regexp.QuoteMeta(baseURL+linkPath) + `(\S+)`)
	links := re.FindAllStringSubmatch(e.Text, -1)
	if len(links) != 2+3+1 || e.PhotoCount != len(links) {
		t.Fatalf("%d links, photo_count %d, want 6:\n%s", len(links), e.PhotoCount, e.Text)
	}
	for _, m := range links {
		id, mac, ok := medialink.Parse(m[1])
		if !ok || !signer.Verify(id, mac, bookID, 2) || signer.Verify(id, mac, bookID, 3) {
			t.Errorf("link %q is not a token of epoch 2", m[1])
		}
	}
	if strings.Contains(e.Text, "01JDDDDDDDDDDDDDDDDDDDDDDD") || strings.Contains(strings.ToLower(e.Text), "photo 4") {
		t.Error("a fourth photo is printed")
	}
}

func TestTemplateFieldsThenLeftovers(t *testing.T) {
	y := fullBook()
	y.TemplateID = ptr("classic")
	y.CoverMediaID = nil
	e := export(t, y, fullNotes()[:1], 1, "en")
	name, msg, how, wish := strings.Index(e.Text, "Your name"), strings.Index(e.Text, "Your message"), strings.Index(e.Text, "How we met"), strings.Index(e.Text, "My wish for you")
	if name <= 0 || name >= msg || msg >= how || how >= wish {
		t.Errorf("order: name %d message %d how %d wish %d\n%s", name, msg, how, wish, e.Text)
	}
}

func TestTruncatedAndCounts(t *testing.T) {
	e := export(t, fullBook(), fullNotes()[:2], 1500, "en")
	if !e.Truncated || e.ApprovedTotal != 1500 || e.NoteCount != 2 || !strings.Contains(e.Text, "2 of 1500 notes are included") {
		t.Errorf("%+v", e)
	}
	e = export(t, fullBook(), fullNotes(), 3, "")
	if e.Truncated || e.NoteCount != 3 || !strings.Contains(e.Text, "Lời nhắn từ bạn bè (3)") {
		t.Errorf("lang defaults to the book's: %+v", e.Filename)
	}
}

func TestFilename(t *testing.T) {
	for title, want := range map[string]string{
		"Lớp 12A1 — Kỷ yếu":                           "lop-12a1-ky-yeu-20261019.txt",
		"Đặng  Đức!!":                                 "dang-duc-20261019.txt",
		"😊 日本語":                                       "yearbook-20261019.txt",
		strings.Repeat("a", 60):                       strings.Repeat("a", 40) + "-20261019.txt",
		"abcdefgh " + strings.Repeat("b", 31) + " cd": "abcdefgh-" + strings.Repeat("b", 31) + "-20261019.txt",
	} {
		got := filename(title, fixedNow)
		if got != want || !regexp.MustCompile(`^[a-z0-9.-]+$`).MatchString(got) {
			t.Errorf("filename(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestErrors(t *testing.T) {
	signer, _ := medialink.New(bytes.Repeat([]byte{7}, medialink.MinSecretBytes))
	bad := apperr.New(apperr.Param, "yearbook_id must be a yearbook id")
	gone := apperr.New(apperr.NotFound, "yearbook not found")
	for name, c := range map[string]struct {
		s    *Service
		lang string
		want error
	}{
		"bad lang":   {NewService(fakeBooks{}, fakeNotes{}, signer, baseURL, nil, nil), "fr", nil},
		"notes err":  {NewService(fakeBooks{}, fakeNotes{err: bad}, signer, baseURL, nil, nil), "en", bad},
		"no book":    {NewService(fakeBooks{err: gone}, fakeNotes{}, signer, baseURL, nil, nil), "en", gone},
		"bad answer": {NewService(fakeBooks{y: fullBook()}, fakeNotes{ns: []*notes.BookNote{bn(`[1]`)}, total: 1}, signer, baseURL, nil, nil), "en", nil},
	} {
		_, err := c.s.GetText(context.Background(), 1, true, bookID, c.lang)
		if err == nil || (c.want != nil && err != c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := NewService(fakeBooks{}, fakeNotes{}, signer, baseURL, nil, nil).GetText(context.Background(), 1, true, bookID, "fr"); !apperr.Is(err, apperr.Param) {
		t.Errorf("bad lang is %v", err)
	}
}

// T-085 style: a route mounted without RequireUser answers ERROR_UNAUTHORIZED instead of running with a nil user.
func TestHandlerRefusesARequestWithoutAUser(t *testing.T) {
	rec := newRecorder()
	(&Handler{}).getText(rec, newRequest())
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "ERROR_UNAUTHORIZED") {
		t.Errorf("got %d %s", rec.Code, rec.Body)
	}
}
