package bookexport

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/danyaa666/smemories/internal/notefields"
)

// labels are the fixed words of the document in one language. The field labels of the notes come from the catalogue.
type labels struct {
	school, class, graduation, motto, exported string
	about, friends, noNotes                    string
	fullName, nickname, quote, hobbies, plans  string
	photo, cover                               string
	partial                                    func(shown, total int) string
}

var texts = map[string]labels{
	"en": {
		school: "School", class: "Class", graduation: "Graduation year", motto: "Motto", exported: "Exported",
		about: "About the owner", friends: "Notes from friends", noNotes: "No notes yet",
		fullName: "Full name", nickname: "Nickname", quote: "Favourite quote", hobbies: "Hobbies", plans: "Future plans",
		photo: "Photo", cover: "Cover photo",
		partial: func(shown, total int) string {
			return strconv.Itoa(shown) + " of " + strconv.Itoa(total) + " notes are included"
		},
	},
	"vi": {
		school: "Trường", class: "Lớp", graduation: "Năm tốt nghiệp", motto: "Châm ngôn", exported: "Xuất ngày",
		about: "Giới thiệu về mình", friends: "Lời nhắn từ bạn bè", noNotes: "Chưa có lời nhắn nào",
		fullName: "Họ và tên", nickname: "Biệt danh", quote: "Câu nói yêu thích", hobbies: "Sở thích", plans: "Dự định tương lai",
		photo: "Ảnh", cover: "Ảnh bìa",
		partial: func(shown, total int) string {
			return "Đã đưa vào " + strconv.Itoa(shown) + " trong tổng số " + strconv.Itoa(total) + " lời nhắn"
		},
	},
}

// note is one approved note ready to print: its answers by field id and its photo links.
type note struct {
	answers map[string]string
	photos  []string // full links, at most maxNotePhotos are printed
}

// book is everything the document is made of; the builder reads nothing else.
type book struct {
	title, school, class, motto        string
	graduationYear                     *int
	fullName, nickname, quote, hobbies string
	plans                              string
	photoLink, coverLink               string // "" = none
	fields                             []notefields.FieldRef
	notes                              []note
	total                              int // every approved note, more than len(notes) when the book was cut
	now                                time.Time
}

const (
	maxNotePhotos = 3
	maxSlug       = 40 // characters of the title part of the file name
)

// catalogue maps a field id to its definition; order gives the position for fields a template no longer asks for.
var catalogue, position = func() (m map[string]notefields.Field, pos map[string]int) {
	m, pos = map[string]notefields.Field{}, map[string]int{}
	for i, f := range notefields.All() {
		m[f.ID], pos[f.ID] = f, i
	}
	return m, pos
}()

// render writes the document (docs/book-export.md). User text is copied as it is; only the indent of multi-line values is added.
func render(lang string, b *book) (text string, photos int) {
	l := texts[lang]
	var w strings.Builder
	// line writes "label: value", or the label alone and the value indented on the lines below when it has several lines or block is set.
	line := func(label, value string, block bool) {
		if strings.TrimSpace(value) == "" {
			return
		}
		if !block && !strings.Contains(value, "\n") {
			w.WriteString(label + ": " + value + "\n")
			return
		}
		w.WriteString(label + ":\n")
		for _, ln := range strings.Split(value, "\n") {
			if ln != "" {
				w.WriteString("  ")
			}
			w.WriteString(ln + "\n")
		}
	}
	link := func(label, url string) {
		if url != "" {
			photos++
			line(label, url, false)
		}
	}

	w.WriteString("SMemories — " + b.title + "\n")
	line(l.school, b.school, false)
	line(l.class, b.class, false)
	if b.graduationYear != nil {
		line(l.graduation, strconv.Itoa(*b.graduationYear), false)
	}
	line(l.motto, b.motto, false)
	line(l.exported, b.now.UTC().Format(time.DateOnly), false)

	if b.fullName+b.nickname+b.quote+b.hobbies+b.plans+b.photoLink+b.coverLink != "" {
		w.WriteString("\n== " + l.about + " ==\n")
		line(l.fullName, b.fullName, false)
		line(l.nickname, b.nickname, false)
		line(l.quote, b.quote, false)
		line(l.hobbies, b.hobbies, false)
		line(l.plans, b.plans, false)
		link(l.photo, b.photoLink)
		link(l.cover, b.coverLink)
	}

	w.WriteString("\n== " + l.friends + " (" + strconv.Itoa(len(b.notes)) + ") ==\n")
	if b.total > len(b.notes) {
		w.WriteString(l.partial(len(b.notes), b.total) + "\n")
	}
	if len(b.notes) == 0 {
		w.WriteString(l.noNotes + "\n")
	}
	for i, n := range b.notes {
		w.WriteString("\n--- " + strconv.Itoa(i+1) + " / " + strconv.Itoa(len(b.notes)) + " ---\n")
		for _, id := range fieldOrder(b.fields, n.answers) {
			f, known := catalogue[id]
			label := id // a field no longer in the catalogue (never happens today) keeps its id so the answer is not dropped
			if known {
				label = f.Label.En
				if lang == "vi" {
					label = f.Label.Vi
				}
			}
			line(label, n.answers[id], f.Kind == notefields.LongText)
		}
		for j, p := range n.photos[:min(len(n.photos), maxNotePhotos)] {
			link(l.photo+" "+strconv.Itoa(j+1), p)
		}
	}
	return w.String(), photos
}

// fieldOrder is the template's fields that have an answer, in the template's order, then the answered fields the template no longer
// asks for (catalogue order, unknown ids last by id): nothing a friend wrote is dropped.
func fieldOrder(refs []notefields.FieldRef, answers map[string]string) []string {
	var out []string
	asked := map[string]bool{}
	for _, r := range refs {
		asked[r.ID] = true
		if strings.TrimSpace(answers[r.ID]) != "" {
			out = append(out, r.ID)
		}
	}
	var extra []string
	for id, v := range answers {
		if !asked[id] && strings.TrimSpace(v) != "" {
			extra = append(extra, id)
		}
	}
	rank := func(id string) int { // catalogue position; unknown ids sort after every known one
		if i, ok := position[id]; ok {
			return i
		}
		return len(position)
	}
	slices.SortFunc(extra, func(a, b string) int { return cmp.Or(cmp.Compare(rank(a), rank(b)), strings.Compare(a, b)) })
	return append(out, extra...)
}

// filename is "<title as lower-case ASCII words joined by ->-<yyyymmdd>.txt" (title part at most 40 characters), or
// "yearbook-<yyyymmdd>.txt" when nothing of the title survives.
func filename(title string, now time.Time) string {
	var slug strings.Builder
	dash := true // no leading dash
	for _, r := range norm.NFD.String(strings.ToLower(title)) {
		switch {
		case r == 'đ':
			r = 'd'
		case unicode.Is(unicode.Mn, r):
			continue // combining mark: the base letter stays
		}
		if r < 128 && (r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			slug.WriteRune(r)
			dash = false
		} else if !dash {
			slug.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(slug.String(), "-")
	if len(s) > maxSlug {
		s = strings.TrimRight(s[:maxSlug], "-")
	}
	return cmp.Or(s, "yearbook") + "-" + now.UTC().Format("20060102") + ".txt"
}
