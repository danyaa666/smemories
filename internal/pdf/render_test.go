package pdf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"codeberg.org/go-pdf/fpdf"
	"golang.org/x/text/unicode/norm"

	"github.com/danyaa666/smemories/internal/pdf/fonts"
	"github.com/danyaa666/smemories/internal/templates"
)

var fixedNow = time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

const vietnamese = "Chúc mừng tốt nghiệp! Đặng Thị Hồng"

func tmpl(t testing.TB, id string) *templates.Template {
	t.Helper()
	tt, ok := templates.Get(id)
	if !ok {
		t.Fatalf("no template %q", id)
	}
	return tt
}

func render(t testing.TB, id string, b Book, src ImageSource, o Options) ([]byte, Report) {
	t.Helper()
	if o.Now.IsZero() {
		o.Now = fixedNow
	}
	var buf bytes.Buffer
	rep, err := Render(context.Background(), tmpl(t, id), b, src, &buf, o)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) || !bytes.HasSuffix(bytes.TrimSpace(buf.Bytes()), []byte("%%EOF")) {
		t.Fatal("output is not a PDF")
	}
	return buf.Bytes(), rep
}

func codes(r Report) string {
	var c []string
	for _, w := range r.Warnings {
		c = append(c, w.Code)
	}
	return strings.Join(c, ",")
}

func TestRenderBothTemplatesBothSizes(t *testing.T) {
	src := mapSource{"c": synthJPEG(t, 1800, 2400, 1), "p": synthJPEG(t, 1800, 2400, 2), "n": synthJPEG(t, 1800, 1200, 3)}
	b := Book{Title: "Khoá 2026", School: "Đại học Bách khoa", Class: "K66", Year: "2026", Motto: "Hẹn gặp lại",
		CoverPhoto: "c", Profile: Profile{FullName: "Đặng Thị Hồng", PhotoID: "p", Nickname: "Hồng"},
		Notes: []Note{{ID: "n1", Author: "Lê Văn Ưu", Relationship: "Bạn cùng lớp", Message: "Chúc mừng!", PhotoIDs: []string{"n", "n"}}}}
	for _, id := range []string{"classic", "modern"} {
		for _, size := range []string{"A5", "A4"} {
			t.Run(id+"/"+size, func(t *testing.T) {
				out, rep := render(t, id, b, src, Options{PageSize: size, Lang: "vi"})
				if rep.Pages != 4 {
					t.Errorf("pages = %d, want 4", rep.Pages)
				}
				w, h := mediaBox(t, out)
				wantW, wantH := 419.53, 595.28
				if size == "A4" {
					wantW, wantH = 595.28, 841.89
				}
				if math.Abs(w-wantW) > 0.6 || math.Abs(h-wantH) > 0.6 {
					t.Errorf("MediaBox %.2f x %.2f, want %.2f x %.2f", w, h, wantW, wantH)
				}
				if size == "A5" && rep.Has(WarnMissingGlyph) {
					t.Errorf("unexpected warnings %s", codes(rep))
				}
				all := ""
				for _, s := range pageStreams(t, out, rep.Pages) {
					all += allText(s) + "\n"
				}
				for _, want := range []string{"Khoá 2026", "Đặng Thị Hồng", "Biệt danh: Hồng", "Lê Văn Ưu", "Hẹn gặp lại"} {
					if !strings.Contains(all, want) {
						t.Errorf("text %q not found in the PDF", want)
					}
				}
			})
		}
	}
}

// AC4: Vietnamese comes out of the PDF exactly, also when the input is decomposed (NFD).
func TestVietnameseTextIsNFCAndExact(t *testing.T) {
	for name, in := range map[string]string{"nfc": vietnamese, "nfd": norm.NFD.String(vietnamese)} {
		t.Run(name, func(t *testing.T) {
			out, rep := render(t, "modern", Book{Profile: Profile{Quote: in}}, nil, Options{})
			if len(rep.Warnings) != 0 {
				t.Fatalf("warnings: %s", codes(rep))
			}
			var got []string
			for _, d := range textsOf(pageStreams(t, out, rep.Pages)[1]) {
				got = append(got, d.text)
			}
			if len(got) != 1 || got[0] != vietnamese {
				t.Errorf("extracted %q, want exactly %q", got, vietnamese)
			}
		})
	}
}

func TestEmojiAndMissingGlyphs(t *testing.T) {
	note := func(msg string) Book {
		return Book{Notes: []Note{{ID: "n1", Author: "A", Message: msg}}}
	}
	t.Run("emoji use the fallback font", func(t *testing.T) {
		out, rep := render(t, "classic", note("Vinh biệt 🎓🎉❤ nhé"), nil, Options{})
		if len(rep.Warnings) != 0 {
			t.Fatalf("warnings: %s", codes(rep))
		}
		if txt := allText(pageStreams(t, out, rep.Pages)[2]); !strings.Contains(txt, "❤") {
			t.Errorf("emoji glyphs missing from %q", txt)
		}
	})
	t.Run("only emoji, ZWJ sequence and skin tone", func(t *testing.T) {
		_, rep := render(t, "classic", note("👨‍👩‍👧‍👦 👍🏽 🎓🎓🎓 ❤️"), nil, Options{})
		if rep.Has(WarnMissingGlyph) {
			t.Errorf("unexpected: %+v", rep.Warnings)
		}
	})
	t.Run("rune in no font is drawn as ? and reported", func(t *testing.T) {
		for _, c := range []string{"\U00013000", "中", "", "\U0010FFFF", "\xff"} {
			out, rep := render(t, "classic", note("x"+c+"y"), nil, Options{})
			if len(rep.Warnings) != 1 || rep.Warnings[0].Code != WarnMissingGlyph || rep.Warnings[0].NoteID != "n1" ||
				rep.Warnings[0].Slot != "note_message" || rep.Warnings[0].Page != 3 {
				t.Fatalf("%q: warnings %+v", c, rep.Warnings)
			}
			if c != "\xff" && rep.Warnings[0].Rune != c {
				t.Errorf("warning rune %q, want %q", rep.Warnings[0].Rune, c)
			}
			if txt := allText(pageStreams(t, out, rep.Pages)[2]); !strings.Contains(txt, "x?y") {
				t.Errorf("%q: drawn %q lacks x?y", c, txt)
			}
		}
	})
	t.Run("the same missing rune is reported once per element", func(t *testing.T) {
		_, rep := render(t, "classic", note(strings.Repeat("中", 50)), nil, Options{})
		if len(rep.Warnings) != 1 {
			t.Errorf("warnings: %+v", rep.Warnings)
		}
	})
}

// ADR 0002 item 7: fpdf panics on runes above U+FFFF. If this test fails, fpdf fixed it and toPUA can go.
func TestFpdf_NonBMPRunePanics(t *testing.T) {
	pdf := fpdf.NewCustom(&fpdf.InitType{OrientationStr: "P", UnitStr: "mm", Size: fpdf.SizeType{Wd: 148, Ht: 210}})
	pdf.AddUTF8FontFromBytes("emoji", "", fonts.Emoji)
	pdf.AddPage()
	pdf.SetFont("emoji", "", 16)
	defer func() {
		if recover() == nil {
			t.Error("expected a panic: if fpdf fixed this, the PUA workaround can go")
		}
	}()
	pdf.Write(8, "🎓")
}

func TestClassify(t *testing.T) {
	g, err := newGlyphs()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		r    rune
		want glyph
	}{
		{'a', glyph{'a', fontPrimary, true}},
		{'ự', glyph{'ự', fontPrimary, true}},
		{'🎓', glyph{toPUA('🎓'), fontEmoji, true}},
		{'❤', glyph{'❤', fontEmoji, true}},
		{'\U00013000', glyph{'?', fontPrimary, false}},
		{0xE393, glyph{'?', fontPrimary, false}}, // user private use must not hit the emoji aliases
		{0xFFFD, glyph{'?', fontPrimary, false}},
	} {
		if got := g.classify(tc.r); got != tc.want {
			t.Errorf("classify(%q) = %+v, want %+v", tc.r, got, tc.want)
		}
	}
	if d := g.classify('🎓').draw; d > 0xFFFF {
		t.Errorf("draw rune %U is above U+FFFF", d)
	}
}

func TestPrepareText(t *testing.T) {
	got, cut := prepareText("a\r\nb\tc\u200d\ufe0f\x00d")
	if got != "a\nb cd" || cut {
		t.Errorf("got %q cut=%v", got, cut)
	}
	if _, cut := prepareText(strings.Repeat("x", maxTextRunes+1)); !cut {
		t.Error("long text was not cut")
	}
}

// ---- text fitting (AC6) ----

func testRenderer(t *testing.T) *renderer {
	t.Helper()
	g, err := newGlyphs()
	if err != nil {
		t.Fatal(err)
	}
	r := &renderer{tmpl: tmpl(t, "classic"), glyphs: g, widths: map[widthKey]float64{}}
	r.pdf = r.newDoc([2]float64{148, 210}, fixedNow)
	r.pdf.AddPage()
	return r
}

func (r *renderer) fitText(text string, b textBox) layout {
	t, cut := prepareText(text)
	us, _ := r.toUnits(t, b.bold)
	return r.fit(us, b, cut)
}

func assertInside(t *testing.T, l layout, b textBox) {
	t.Helper()
	if h := float64(len(l.lines)) * b.lineMM(l.size); h > b.h+1e-6 {
		t.Errorf("text is %.2f mm high in a %.2f mm box", h, b.h)
	}
	for i, ln := range l.lines {
		if w := ln.em * l.size * ptMM; w > b.w+1e-6 {
			t.Errorf("line %d is %.2f mm wide in a %.2f mm box", i, w, b.w)
		}
	}
}

func TestFitText(t *testing.T) {
	r := testRenderer(t)
	box := textBox{w: 60, h: 20, size: 14, minSize: 8, lineH: 1.3}
	t.Run("short text keeps its size on one line", func(t *testing.T) {
		l := r.fitText("Xin chào", box)
		if len(l.lines) != 1 || l.size != 14 || l.truncated {
			t.Errorf("%+v", l)
		}
	})
	t.Run("wraps by words", func(t *testing.T) {
		l := r.fitText("một hai ba bốn năm sáu bảy tám chín mười", textBox{w: 40, h: 40, size: 12, minSize: 12, lineH: 1.3})
		if len(l.lines) < 2 || l.truncated {
			t.Fatalf("%+v", l)
		}
		for _, ln := range strings.Split(l.plain(), "\n") {
			if strings.HasPrefix(ln, " ") || strings.HasSuffix(ln, " ") || len(strings.Fields(ln)) == 0 {
				t.Errorf("bad line %q", ln)
			}
		}
	})
	t.Run("shrinks before truncating", func(t *testing.T) {
		text := strings.Repeat("lorem ipsum ", 12)
		l := r.fitText(text, box)
		if l.truncated || l.size >= 14 || l.size < 8 {
			t.Errorf("size %.1f truncated=%v", l.size, l.truncated)
		}
		assertInside(t, l, box)
	})
	t.Run("a 3000 character message is truncated with an ellipsis", func(t *testing.T) {
		l := r.fitText(strings.Repeat("Chúc bạn thật nhiều niềm vui và thành công! ", 70), box)
		if !l.truncated || l.size != 8 {
			t.Errorf("size %.1f truncated=%v", l.size, l.truncated)
		}
		if !strings.HasSuffix(l.plain(), "…") {
			t.Errorf("no ellipsis: %q", l.plain())
		}
		assertInside(t, l, box)
	})
	t.Run("one 500 character word breaks by character", func(t *testing.T) {
		l := r.fitText(strings.Repeat("W", 500), textBox{w: 50, h: 200, size: 10, minSize: 10, lineH: 1.3})
		if len(l.lines) < 10 || l.truncated {
			t.Errorf("%d lines truncated=%v", len(l.lines), l.truncated)
		}
		assertInside(t, l, textBox{w: 50, h: 200, size: 10, lineH: 1.3})
	})
	t.Run("emoji and newlines", func(t *testing.T) {
		l := r.fitText(strings.Repeat("🎓🎉❤", 200)+"\n\n"+strings.Repeat("😀", 30), box)
		assertInside(t, l, box)
	})
	t.Run("exact fit is not truncated", func(t *testing.T) {
		b := textBox{w: 60, h: 2 * 14 * ptMM * 1.3, size: 14, minSize: 14, lineH: 1.3}
		l := r.fitText("a\nb", b)
		if len(l.lines) != 2 || l.truncated {
			t.Errorf("%+v", l)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if l := r.fitText("", box); len(l.lines) != 0 || l.truncated {
			t.Errorf("%+v", l)
		}
	})
}

// AC6 end to end: nothing drawn for a 3000-character note lies outside the note's box on the page.
func TestLongMessageStaysInItsBox(t *testing.T) {
	for _, size := range []string{"A5", "A4"} {
		b := Book{Notes: []Note{{ID: "n1", Author: "A", Message: strings.Repeat("Cảm ơn bạn rất nhiều vì tất cả! ", 95)}}}
		out, rep := render(t, "classic", b, nil, Options{PageSize: size})
		if !rep.Has(WarnTextTruncated) {
			t.Fatalf("%s: no text_truncated warning: %s", size, codes(rep))
		}
		page := pageStreams(t, out, rep.Pages)[2]
		_, hPt := mediaBox(t, out)
		sc := 1.0
		if size == "A4" {
			sc = 210.0 / 148
		}
		// classic note message: x=4 y=14 w=78 h=42 inside the first item at (12, 16), in A5 mm.
		left, top := (12+4)*sc, (16+14)*sc
		bottom := top + 42*sc
		const mm = 72 / 25.4
		n := 0
		for _, d := range textsOf(page) {
			yMM := (hPt - d.y) / mm
			if strings.ContainsRune(d.text, '…') || len(d.text) > 40 {
				n++
				if d.x/mm < left-0.02 || yMM < top || yMM > bottom {
					t.Errorf("%s: %q drawn at x=%.2f y=%.2f mm, box is x>=%.2f y=%.2f..%.2f", size, d.text[:10], d.x/mm, yMM, left, top, bottom)
				}
			}
		}
		if n == 0 {
			t.Errorf("%s: message text not found in the page", size)
		}
	}
}

// ---- notes pagination (AC3) ----

func TestNotesPagination(t *testing.T) {
	for _, tc := range []struct{ notes, pages int }{{0, 4}, {1, 4}, {3, 4}, {4, 5}, {7, 6}, {60, 23}} {
		t.Run(fmt.Sprint(tc.notes), func(t *testing.T) {
			var b Book
			for i := range tc.notes {
				b.Notes = append(b.Notes, Note{ID: fmt.Sprint(i), Author: fmt.Sprintf("Bạn số %d", i), Message: "Chúc mừng"})
			}
			out, rep := render(t, "classic", b, nil, Options{})
			if rep.Pages != tc.pages {
				t.Fatalf("pages = %d, want %d", rep.Pages, tc.pages)
			}
			streams := pageStreams(t, out, rep.Pages)
			for i := range tc.notes { // note i is on notes page i/3 (page index 2 + i/3), in order
				if !strings.Contains(allText(streams[2+i/3]), fmt.Sprintf("Bạn số %d", i)) {
					t.Fatalf("note %d not on page %d", i, 3+i/3)
				}
			}
		})
	}
}

// ---- images (AC7) ----

func TestImages(t *testing.T) {
	src := mapSource{
		"ok":    synthJPEG(t, 1800, 2400, 1),
		"small": synthJPEG(t, 300, 400, 2),
		"png":   synthPNG(t, 600, 800),
		"bad":   []byte("not an image"),
		"gif":   synthGIF(t),
		"wide":  synthPNG(t, maxImageSide+1, 1),
		"empty": nil,
	}
	cover := func(id string) (Report, []byte) {
		out, rep := render(t, "classic", Book{CoverPhoto: id}, src, Options{})
		return rep, out
	}
	if rep, _ := cover("ok"); len(rep.Warnings) != 0 {
		t.Errorf("good photo: %s", codes(rep))
	}
	if rep, _ := cover(""); len(rep.Warnings) != 0 {
		t.Errorf("no photo: %s", codes(rep))
	}
	rep, _ := cover("small")
	if len(rep.Warnings) != 1 || rep.Warnings[0].Code != WarnLowResolution || rep.Warnings[0].MediaID != "small" || rep.Warnings[0].DPI >= 300 {
		t.Errorf("small photo: %+v", rep.Warnings)
	}
	if rep, _ := cover("png"); !rep.Has(WarnLowResolution) || rep.Has(WarnMissingImage) {
		t.Errorf("png: %+v", rep.Warnings) // PNG is supported (600 px on 148 mm is low resolution)
	}
	for _, id := range []string{"bad", "gif", "wide", "empty", "missing"} {
		rep, out := cover(id)
		if len(rep.Warnings) != 1 || rep.Warnings[0].Code != WarnMissingImage || rep.Warnings[0].MediaID != id || rep.Warnings[0].Page != 1 {
			t.Errorf("%s: %+v", id, rep.Warnings)
		}
		if len(out) == 0 {
			t.Errorf("%s: no output", id)
		}
	}
	t.Run("a photo used twice is warned once and stored once", func(t *testing.T) {
		out, rep := render(t, "classic", Book{CoverPhoto: "small", Profile: Profile{PhotoID: "small"}}, src, Options{})
		if n := len(rep.Warnings); n != 1 {
			t.Errorf("%d warnings", n)
		}
		if n := bytes.Count(out, []byte("/Subtype /Image")); n != 1 {
			t.Errorf("%d image objects", n)
		}
	})
	t.Run("more photos than slots", func(t *testing.T) {
		b := Book{Notes: []Note{{ID: "n", Author: "A", PhotoIDs: []string{"ok", "ok", "ok", "ok"}}}}
		if _, rep := render(t, "classic", b, src, Options{}); !rep.Has(WarnExtraPhotos) {
			t.Errorf("%s", codes(rep))
		}
	})
	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := Render(ctx, tmpl(t, "classic"), Book{}, src, &bytes.Buffer{}, Options{}); err != context.Canceled {
			t.Errorf("err = %v", err)
		}
	})
}

// ADR 0002 item 5: a cover crop starts at a negative x or y and fpdf draws the image at the margin
// unless AllowNegativePosition is set. This checks the drawing commands of the rendered page: the
// image must cover the whole clipped frame (the modern cover is full bleed), so no page background
// shows. (The spike used PDFium to render pixels; that dependency is gone, ADR item 9.)
func TestCoverImageIsPlacedAtNegativeOffset(t *testing.T) {
	for _, tc := range []struct {
		name     string
		w, h     int
		wantNegX bool
	}{{"landscape photo", 4000, 3000, true}, {"tall photo", 2000, 4000, false}} {
		t.Run(tc.name, func(t *testing.T) {
			src := mapSource{"c": synthJPEG(t, tc.w/10, tc.h/10, 1)} // same aspect, small file
			out, rep := render(t, "modern", Book{CoverPhoto: "c"}, src, Options{})
			imgs := imagesOf(pageStreams(t, out, rep.Pages)[0])
			if len(imgs) != 1 {
				t.Fatalf("%d images on the cover", len(imgs))
			}
			im := imgs[0]
			pw, ph := mediaBox(t, out)
			const eps = 0.01
			if im.x > eps || im.x+im.w < pw-eps || im.y > eps || im.y+im.h < ph-eps {
				t.Errorf("image %+v does not cover the page %.2f x %.2f", im, pw, ph)
			}
			if tc.wantNegX != (im.x < -eps) {
				t.Errorf("x = %.2f: negative offset expected = %v", im.x, tc.wantNegX)
			}
			if !tc.wantNegX && im.y >= -eps {
				t.Errorf("y = %.2f, expected a negative offset for a portrait photo", im.y)
			}
		})
	}
}

func TestCoverRect(t *testing.T) {
	x, y, w, h, dpi := coverRect(4000, 3000, 0, 0, 148, 210)
	if x >= 0 || math.Abs(y) > 1e-9 || math.Abs(h-210) > 1e-9 || x+w < 148 || math.Abs(x+w/2-74) > 1e-9 {
		t.Errorf("x=%.2f y=%.2f w=%.2f h=%.2f", x, y, w, h)
	}
	if math.Abs(dpi-363) > 1 {
		t.Errorf("dpi = %.1f, want about 363", dpi)
	}
}

// ---- determinism and robustness (AC8) ----

func TestDeterministicOutput(t *testing.T) {
	// Distinct widths on purpose: fpdf orders image objects by width and, for equal widths, in random map
	// order, so photos of the same width can swap object numbers between renders (see docs/templates.md).
	src := mapSource{"c": synthJPEG(t, 600, 800, 1), "n": synthJPEG(t, 640, 400, 2)}
	b := Book{Title: "Khoá 🎓 2026", CoverPhoto: "c", Profile: Profile{FullName: "Đặng Thị Hồng", PhotoID: "c"},
		Notes: []Note{{ID: "1", Author: "A", Message: "Chúc mừng ❤", PhotoIDs: []string{"n", "n"}}, {ID: "2", Author: "B", Message: "x"}}}
	for _, id := range []string{"classic", "modern"} {
		a1, _ := render(t, id, b, src, Options{})
		a2, _ := render(t, id, b, src, Options{})
		if !bytes.Equal(a1, a2) {
			t.Errorf("%s: two renders differ", id)
		}
		a3, _ := render(t, id, b, src, Options{Now: fixedNow.Add(time.Hour)})
		if bytes.Equal(a1, a3) {
			t.Errorf("%s: creation date has no effect", id)
		}
	}
}

func TestRenderRejectsBadOptions(t *testing.T) {
	for _, o := range []Options{{PageSize: "A3"}, {PageSize: "letter"}} {
		if _, err := Render(context.Background(), tmpl(t, "classic"), Book{}, nil, &bytes.Buffer{}, o); err == nil {
			t.Errorf("%+v accepted", o)
		}
	}
}

func TestEmptyBookRenders(t *testing.T) {
	for _, id := range []string{"classic", "modern"} {
		if _, rep := render(t, id, Book{}, nil, Options{}); rep.Pages != 4 || len(rep.Warnings) != 0 {
			t.Errorf("%s: %+v", id, rep)
		}
	}
}

// T-037 AC3: the MediaBox of each page size, and a Letter-reference template renders on Letter only.
func TestPageBoxes(t *testing.T) {
	for size, want := range map[string][2]float64{"A5": {419.53, 595.28}, "A4": {595.28, 841.89}, "Letter": {612, 792}} {
		t.Run(size, func(t *testing.T) {
			tt := *tmpl(t, "classic")
			tt.ID = "box-test"
			tt.PageSizes = []string{size}
			if size == "Letter" { // classic's A5 coordinates also lie inside the Letter page
				tt.Reference = "Letter"
			}
			if err := templates.Validate(&tt); err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if _, err := Render(context.Background(), &tt, Book{Title: "T"}, nil, &buf, Options{PageSize: size, Now: fixedNow}); err != nil {
				t.Fatal(err)
			}
			w, h := mediaBox(t, buf.Bytes())
			if math.Abs(w-want[0]) > 0.5 || math.Abs(h-want[1]) > 0.5 {
				t.Errorf("MediaBox %.2f x %.2f, want %.2f x %.2f", w, h, want[0], want[1])
			}
		})
	}
}

func TestRenderRefusesUnsupportedSize(t *testing.T) {
	for _, id := range []string{"classic", "modern"} {
		_, err := Render(context.Background(), tmpl(t, id), Book{}, nil, io.Discard, Options{PageSize: "Letter"})
		if err == nil || !strings.Contains(err.Error(), "does not support page size") {
			t.Errorf("%s on Letter: err = %v", id, err)
		}
	}
}
