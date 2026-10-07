package pdf

import (
	"bytes"
	"context"
	"math/rand"
	"testing"
)

// randStr builds hostile text: raw bytes, lone surrogates, emoji, private use, joiners, NUL, combining marks.
func randStr(r *rand.Rand, n int) string {
	var b []byte
	for len(b) < n {
		switch r.Intn(8) {
		case 0:
			b = append(b, byte(r.Intn(256)))
		case 1:
			b = append(b, 0xED, 0xA0+byte(r.Intn(0x20)), 0x80+byte(r.Intn(0x40))) // surrogates
		case 2:
			b = append(b, string(rune(0x1F000+r.Intn(0xB00)))...)
		case 3:
			b = append(b, string(rune(r.Intn(0x110000)))...)
		case 4:
			b = append(b, string(rune(0xE000+r.Intn(0x1900)))...)
		case 5:
			b = append(b, []string{"\u200d", "\ufe0f", "\u0301", "\u0323", " ", "\n\r\t", "\x00", "\u202e"}[r.Intn(8)]...)
		case 6:
			b = append(b, string(rune(0x300+r.Intn(0x70)))...) // combining marks
		default:
			b = append(b, "Đặng Thị Hồng ưỡ"[r.Intn(10):]...)
		}
	}
	return string(b)
}

func TestRenderSurvivesHostileText(t *testing.T) {
	r := rand.New(rand.NewSource(42)) //nolint:gosec // deterministic test data, not security
	src := mapSource{"j": synthJPEG(t, 300, 200, 1)}
	for i := 0; i < 10; i++ {
		l := []int{1, 5, 40, 300, 3000}[r.Intn(5)]
		b := Book{Title: randStr(r, l), School: randStr(r, l), Class: randStr(r, l), Year: randStr(r, l), Motto: randStr(r, l), CoverPhoto: randStr(r, 5),
			Profile: Profile{FullName: randStr(r, l), Nickname: randStr(r, l), Quote: randStr(r, l), Hobbies: randStr(r, l), Plans: randStr(r, l), PhotoID: "j"}}
		for k := 0; k < 4; k++ {
			b.Notes = append(b.Notes, Note{ID: randStr(r, 8), Author: randStr(r, l), Relationship: randStr(r, l), Message: randStr(r, l), PhotoIDs: []string{"j", randStr(r, 3), "../../etc/passwd", "/etc/passwd"}})
		}
		for _, tid := range []string{"classic", "modern"} {
			for _, sz := range []string{"A5", "A4"} {
				var buf bytes.Buffer
				_, err := Render(context.Background(), tmpl(t, tid), b, src, &buf, Options{PageSize: sz, Lang: "vi", Now: fixedNow})
				if err != nil {
					t.Fatalf("iter %d %s %s: %v", i, tid, sz, err)
				}
			}
		}
	}
}
