//go:build spike

package spike

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/signintech/gopdf"
	"golang.org/x/text/unicode/norm"
)

func newGopdf(w, h float64) *gopdf.GoPdf {
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{Unit: gopdf.UnitMM, PageSize: gopdf.Rect{W: w, H: h}})
	must(0, pdf.AddTTFFontData("bvp", readFont("BeVietnamPro-Regular.ttf")))
	must(0, pdf.AddTTFFontData("bvpb", readFont("BeVietnamPro-Bold.ttf")))
	must(0, pdf.AddTTFFontData("emoji", readFont("NotoEmoji-Regular.ttf")))
	return pdf
}

func gsave(t *testing.T, pdf *gopdf.GoPdf, name string) string {
	t.Helper()
	p := outPath(name)
	if err := pdf.WritePdf(p); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return p
}

// gText draws one left-aligned line at (x, y) with per-rune font fallback; returns the unknown runes.
func gText(t *testing.T, pdf *gopdf.GoPdf, x, y, size float64, text string) (unknown []rune) {
	t.Helper()
	pdf.SetXY(x, y)
	for _, r := range splitRuns(text, fallbackFonts()) {
		family, s := "bvp", r.text
		switch r.font {
		case 1:
			family = "emoji"
		case -1:
			unknown = append(unknown, []rune(r.text)...)
			s = strings.Repeat("?", len([]rune(r.text)))
		}
		must(0, pdf.SetFont(family, "", size))
		w := must(pdf.MeasureTextWidth(s))
		pdf.SetXY(x, y)
		must(0, pdf.Text(s))
		x += w
	}
	return unknown
}

// roundedPoly approximates a rounded rectangle (r = min(w,h)/2 gives a circle) as a polygon,
// because gopdf can only clip to polygons.
func roundedPoly(x, y, w, h, r float64) []gopdf.Point {
	const steps = 24
	var pts []gopdf.Point
	corner := func(cx, cy, a0 float64) {
		for i := 0; i <= steps; i++ {
			a := (a0 + 90*float64(i)/steps) * math.Pi / 180
			pts = append(pts, gopdf.Point{X: cx + r*math.Cos(a), Y: cy + r*math.Sin(a)})
		}
	}
	corner(x+w-r, y+h-r, 0)
	corner(x+r, y+h-r, 90)
	corner(x+r, y+r, 180)
	corner(x+w-r, y+r, 270)
	return pts
}

func gImage(t *testing.T, pdf *gopdf.GoPdf, holder gopdf.ImageHolder, fx, fy, fw, fh float64) {
	t.Helper()
	x, y, w, h, _ := coverRect(photoW, photoH, fx, fy, fw, fh)
	if err := pdf.ImageByHolder(holder, x, y, &gopdf.Rect{W: w, H: h}); err != nil {
		t.Fatal(err)
	}
}

func TestGopdf_TextAndFallback(t *testing.T) {
	pdf := newGopdf(a5W, a5H)
	pdf.AddPage()
	opt := gopdf.CellOption{Align: gopdf.Center | gopdf.Top, BreakOption: &gopdf.BreakOption{Mode: gopdf.BreakModeIndicatorSensitive, BreakIndicator: ' '}}
	must(0, pdf.SetFont("bvpb", "", 20))
	pdf.SetXY(10, 15)
	must(0, pdf.MultiCellWithOption(&gopdf.Rect{W: 128, H: 9}, norm.NFC.String(vietnamese), opt))
	must(0, pdf.SetFont("bvp", "", 13))
	pdf.SetXY(10, 55)
	opt.Align = gopdf.Left | gopdf.Top
	must(0, pdf.MultiCellWithOption(&gopdf.Rect{W: 128, H: 7}, norm.NFC.String(vietnamese)+". "+norm.NFC.String(vietnamese), opt))

	if u := gText(t, pdf, 10, 100, 16, "Vinh biệt trường xưa "+emojiLine+" hẹn gặp lại"); len(u) != 0 {
		t.Fatalf("unexpected unknown runes %q", string(u))
	}
	// What the engine does with a rune that no font has, if we do NOT guard against it:
	pdf.SetXY(10, 112)
	must(0, pdf.SetFont("bvp", "", 16))
	var rawErr error
	noPanic(t, "raw rune in no font", func() { rawErr = pdf.Text("x" + missing) })
	t.Logf("raw unknown rune: Text returned error = %v", rawErr)
	has, _ := pdf.IsCurrFontContainGlyph('ệ')
	hasNo, _ := pdf.IsCurrFontContainGlyph([]rune(missing)[0])
	t.Logf("IsCurrFontContainGlyph(ệ)=%v (U+13000)=%v", has, hasNo)

	var unknown []rune
	noPanic(t, "rune in no font", func() { unknown = gText(t, pdf, 10, 120, 16, "Lạ: "+missing+" (không có font)") })
	if string(unknown) != missing {
		t.Errorf("unknown rune not detected: %q", string(unknown))
	}
	gText(t, pdf, 10, 140, 14, norm.NFD.String("Đặng Thị Hồng")) // NFD in, NFC out
	p := gsave(t, pdf, "gopdf-text.pdf")

	raw := extractText(t, p, 0)
	got := squash(raw)
	if !strings.Contains(got, squash(norm.NFC.String(vietnamese))) {
		t.Errorf("Vietnamese text not recovered intact from the PDF:\n got  %q\n want %q", got, squash(vietnamese))
	}
	if !strings.Contains(got, squash("ĐặngThịHồng")) {
		t.Errorf("NFD input did not normalise to NFC: %q", got)
	}
	// gopdf writes a broken ToUnicode entry for non-BMP code points: the glyphs draw fine (see the
	// screenshot) but copy/paste and extraction return the wrong characters.
	if !strings.Contains(raw, "❤") {
		t.Errorf("BMP emoji missing from the extracted text: %q", raw)
	}
	t.Logf("non-BMP emoji extracted as %q (input %q)", raw[strings.Index(raw, "xưa ")+len("xưa "):][:6], emojiLine)
	renderPNG(t, p, 0, "gopdf-text.png")
}

func TestGopdf_Image(t *testing.T) {
	ensurePhotos()
	jpg := must(os.ReadFile(photoPath(0)))
	pdf := newGopdf(a5W, a5H)
	pdf.AddPage()
	x, y, w, h, dpi := coverRect(photoW, photoH, 0, 0, a5W, a5H)
	holder := must(gopdf.ImageHolderByBytes(jpg))
	pdf.SaveGraphicsState()
	pdf.ClipPolygon([]gopdf.Point{{X: 0, Y: 0}, {X: a5W, Y: 0}, {X: a5W, Y: a5H}, {X: 0, Y: a5H}})
	must(0, pdf.ImageByHolder(holder, x, y, &gopdf.Rect{W: w, H: h}))
	pdf.RestoreGraphicsState()
	pdf.AddPage()
	pdf.SetFillColor(240, 240, 200)
	pdf.RectFromUpperLeftWithStyle(0, 0, a5W, a5H, "F")
	png := pngAlpha()
	must(0, pdf.ImageByHolder(must(gopdf.ImageHolderByBytes(png)), 24, 60, &gopdf.Rect{W: 100, H: 100}))
	p := gsave(t, pdf, "gopdf-image.pdf")

	renderPNG(t, p, 0, "gopdf-image-cover.png")
	renderPNG(t, p, 1, "gopdf-image-alpha.png")
	if n := pageCount(t, p); n != 2 {
		t.Errorf("pages = %d", n)
	}
	out := must(os.ReadFile(p))
	if dpi < 300 {
		t.Errorf("effective DPI %.0f < 300", dpi)
	}
	if !strings.Contains(string(out), string(jpg)) {
		t.Errorf("JPEG bytes were re-encoded (source not found verbatim in the PDF)")
	}
	if !strings.Contains(string(out), "/SMask") {
		t.Errorf("PNG alpha not written as an SMask")
	}
	t.Logf("cover on A5: image placed %.1fx%.1f mm at (%.1f,%.1f), effective %.0f DPI; PDF %d bytes vs JPEG %d bytes (+PNG %d)",
		w, h, x, y, dpi, len(out), len(jpg), len(png))
}

func TestGopdf_Layout(t *testing.T) {
	ensurePhotos()
	holder := must(gopdf.ImageHolderByBytes(must(os.ReadFile(photoPath(1)))))
	pdf := newGopdf(a5W, a5H)
	pdf.AddPage()
	pdf.SetFillColor(250, 235, 215)
	pdf.RectFromUpperLeftWithStyle(0, 0, a5W, a5H, "F")

	// rounded-rectangle and circular image clipping (polygon approximation)
	pdf.SaveGraphicsState()
	pdf.ClipPolygon(roundedPoly(70, 10, 68, 50, 8))
	gImage(t, pdf, holder, 70, 10, 68, 50)
	pdf.RestoreGraphicsState()
	pdf.SaveGraphicsState()
	pdf.ClipPolygon(roundedPoly(10, 40, 40, 40, 20))
	gImage(t, pdf, holder, 10, 40, 40, 40)
	pdf.RestoreGraphicsState()

	// rotated image and rotated text
	pdf.Rotate(-12, 105, 94) // gopdf rotates the other way round from fpdf
	pdf.SaveGraphicsState()
	pdf.ClipPolygon([]gopdf.Point{{X: 80, Y: 75}, {X: 130, Y: 75}, {X: 130, Y: 113}, {X: 80, Y: 113}})
	gImage(t, pdf, holder, 80, 75, 50, 38)
	pdf.RestoreGraphicsState()
	pdf.RotateReset()
	pdf.Rotate(20, 10, 100)
	must(0, pdf.SetFont("bvpb", "", 16))
	pdf.SetTextColor(200, 30, 60)
	pdf.SetXY(10, 94)
	must(0, pdf.Text("Kỷ niệm 2026"))
	pdf.RotateReset()

	// wrapped text in a box: left and centre, explicit line height, colour and opacity
	text := norm.NFC.String(vietnamese)
	pdf.SetTextColor(20, 20, 20)
	must(0, pdf.SetFont("bvp", "", 10))
	box := 60.0
	lines := must(pdf.SplitTextWithWordWrap(text, box))
	if len(lines) < 2 {
		t.Errorf("text did not wrap: %q", lines)
	}
	for _, l := range lines {
		if w := must(pdf.MeasureTextWidth(l)); w > box+0.01 {
			t.Errorf("line wider than the box: %q (%.1f)", l, w)
		}
	}
	// MultiCell has no line-height control (pitch = font metrics) and its Border option draws one box
	// per line, so wrap with SplitTextWithWordWrap and place each line ourselves.
	pdf.SetLineWidth(0.2)
	for _, c := range []struct {
		x, lineH float64
		align    int
	}{{10, 4.5, gopdf.Left}, {78, 7, gopdf.Center}} {
		pdf.RectFromUpperLeftWithStyle(c.x, 120, box, c.lineH*float64(len(lines)), "D")
		for i, l := range lines {
			pdf.SetXY(c.x, 120+float64(i)*c.lineH)
			must(0, pdf.CellWithOption(&gopdf.Rect{W: box, H: c.lineH}, l, gopdf.CellOption{Align: c.align | gopdf.Middle}))
		}
	}
	// opacity only applies through CellOption.Transparency (SetTransparency did not affect Text/Cell)
	tr := must(gopdf.NewTransparency(0.35, ""))
	pdf.SetTextColor(0, 0, 0)
	must(0, pdf.SetFont("bvpb", "", 30))
	pdf.SetXY(15, 172)
	must(0, pdf.CellWithOption(&gopdf.Rect{W: 100, H: 12}, "BẢN NHÁP", gopdf.CellOption{Transparency: &tr}))
	p := gsave(t, pdf, "gopdf-layout.pdf")
	if w, h := mediaBox(t, p); !near(w, 419.53, 0.6) || !near(h, 595.28, 0.6) {
		t.Errorf("A5 MediaBox = %.2f x %.2f", w, h)
	}
	renderPNG(t, p, 0, "gopdf-layout.png")

	a4 := newGopdf(210, 297)
	a4.AddPage()
	must(0, a4.SetFont("bvp", "", 12))
	a4.SetXY(10, 14)
	must(0, a4.Text("A4 page"))
	p4 := gsave(t, a4, "gopdf-a4.pdf")
	if w, h := mediaBox(t, p4); !near(w, 595.28, 0.6) || !near(h, 841.89, 0.6) {
		t.Errorf("A4 MediaBox = %.2f x %.2f", w, h)
	}
}

func TestBenchGopdf(t *testing.T) {
	ensurePhotos()
	start := time.Now()
	pdf := newGopdf(a5W, a5H)
	opt := gopdf.CellOption{Align: gopdf.Left | gopdf.Top, BreakOption: &gopdf.BreakOption{Mode: gopdf.BreakModeIndicatorSensitive, BreakIndicator: ' '}}
	for i := 0; i < nPhotos; i++ {
		pdf.AddPage()
		holder := must(gopdf.ImageHolderByPath(photoPath(i)))
		pdf.SaveGraphicsState()
		pdf.ClipPolygon(roundedPoly(10, 10, 128, 150, 6))
		gImage(t, pdf, holder, 10, 10, 128, 150)
		pdf.RestoreGraphicsState()
		must(0, pdf.SetFont("bvp", "", 11))
		pdf.SetXY(10, 168)
		must(0, pdf.MultiCellWithOption(&gopdf.Rect{W: 128, H: 5.5}, norm.NFC.String(vietnamese), opt))
	}
	p := gsave(t, pdf, "gopdf-bench.pdf")
	wall, rss := time.Since(start), peakRSSMB() // before PDFium starts: it would inflate the RSS
	st := must(os.Stat(p))
	if n := pageCount(t, p); n != nPhotos {
		t.Errorf("pages = %d", n)
	}
	t.Logf("BENCH lib=gopdf pages=%d photos=%d wall=%.2fs peak_rss=%.0fMB pdf=%.1fMB machine=%s",
		nPhotos, nPhotos, wall.Seconds(), rss, float64(st.Size())/1e6, machine())
}
