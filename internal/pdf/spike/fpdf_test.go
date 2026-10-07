//go:build spike

package spike

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"codeberg.org/go-pdf/fpdf"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/text/unicode/norm"
)

func newFpdf(size string) *fpdf.Fpdf {
	// fpdf's built-in "A5" is 148.5 x 210 mm (420.94 pt), 0.5 mm too wide: use exact sizes.
	dims := map[string]fpdf.SizeType{"A5": {Wd: a5W, Ht: a5H}, "A4": {Wd: 210, Ht: 297}}[size]
	pdf := fpdf.NewCustom(&fpdf.InitType{OrientationStr: "P", UnitStr: "mm", Size: dims})
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetMargins(10, 10, 10)
	pdf.AddUTF8FontFromBytes("bvp", "", readFont("BeVietnamPro-Regular.ttf"))
	pdf.AddUTF8FontFromBytes("bvp", "B", readFont("BeVietnamPro-Bold.ttf"))
	pdf.AddUTF8FontFromBytes("emoji", "", readFont("NotoEmoji-Regular.ttf"))
	return pdf
}

func save(t *testing.T, pdf *fpdf.Fpdf, name string) string {
	t.Helper()
	p := outPath(name)
	if err := pdf.OutputFileAndClose(p); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return p
}

func fallbackFonts() []*sfnt.Font {
	return []*sfnt.Font{loadSfnt(readFont("BeVietnamPro-Regular.ttf")), loadSfnt(readFont("NotoEmoji-Regular.ttf"))}
}

// fpdfWriteRuns draws text inline with per-rune font fallback and returns the runes no font has.
func fpdfWriteRuns(pdf *fpdf.Fpdf, lineH, size float64, text string) (unknown []rune) {
	for _, r := range splitRuns(text, fallbackFonts()) {
		switch r.font {
		case 1:
			pdf.SetFont("emoji", "", size)
			pdf.Write(lineH, toPUA(r.text)) // fpdf only reads BMP code points: see ADR 0002
		case -1:
			unknown = append(unknown, []rune(r.text)...)
			pdf.SetFont("bvp", "", size)
			pdf.Write(lineH, strings.Repeat("?", len([]rune(r.text))))
		default:
			pdf.SetFont("bvp", "", size)
			pdf.Write(lineH, r.text)
		}
	}
	return unknown
}

func TestFpdf_TextAndFallback(t *testing.T) {
	pdf := newFpdf("A5")
	pdf.AddPage()
	pdf.SetFont("bvp", "B", 20)
	pdf.SetXY(10, 15)
	pdf.MultiCell(128, 9, norm.NFC.String(vietnamese), "", "C", false)
	pdf.SetFont("bvp", "", 13)
	pdf.SetXY(10, 55)
	pdf.MultiCell(128, 7, norm.NFC.String(vietnamese)+". "+norm.NFC.String(vietnamese), "", "L", false)

	pdf.SetXY(10, 100)
	if u := fpdfWriteRuns(pdf, 9, 16, "Vinh biệt trường xưa "+emojiLine+" hẹn gặp lại"); len(u) != 0 {
		t.Fatalf("unexpected unknown runes %q", string(u))
	}
	// What the engine does with an unguarded BMP rune that no font has (U+4E2D): nothing visible, no panic.
	pdf.SetXY(10, 110)
	pdf.SetFont("bvp", "", 16)
	noPanic(t, "raw BMP rune in no font", func() { pdf.Write(9, "x\u4e2dx") })
	pdf.SetXY(10, 120)
	var unknown []rune
	noPanic(t, "rune in no font", func() { unknown = fpdfWriteRuns(pdf, 9, 16, "Lạ: "+missing+" (không có font)") })
	if string(unknown) != missing {
		t.Errorf("unknown rune not detected: %q", string(unknown))
	}
	// NFD input (what a macOS keyboard or paste can produce) must come out the same after NFC.
	pdf.SetXY(10, 140)
	fpdfWriteRuns(pdf, 9, 14, norm.NFD.String("Đặng Thị Hồng"))
	p := save(t, pdf, "fpdf-text.pdf")

	raw := extractText(t, p, 0)
	got := squash(raw)
	if !strings.Contains(raw, "\ue393\ue389\u2764") { // 🎓🎉 come back as their PUA aliases, ❤ as itself
		t.Errorf("emoji glyphs missing from the extracted text: %q", raw)
	}
	renderPNG(t, p, 0, "fpdf-text.png")
	if !strings.Contains(got, squash(norm.NFC.String(vietnamese))) {
		t.Errorf("Vietnamese text not recovered intact from the PDF:\n got  %q\n want %q", got, squash(vietnamese))
	}
	if !strings.Contains(got, squash("ĐặngThịHồng")) {
		t.Errorf("NFD input did not normalise to NFC: %q", got)
	}
}

func TestFpdf_Image(t *testing.T) {
	ensurePhotos()
	jpg := must(os.ReadFile(photoPath(0)))
	pdf := newFpdf("A5")
	pdf.AddPage()
	x, y, w, h, dpi := coverRect(photoW, photoH, 0, 0, a5W, a5H)
	pdf.ClipRect(0, 0, a5W, a5H, false)
	pdf.RegisterImageOptionsReader("p0", fpdf.ImageOptions{ImageType: "JPG"}, bytes.NewReader(jpg))
	pdf.ImageOptions("p0", x, y, w, h, false, fpdf.ImageOptions{ImageType: "JPG"}, 0, "")
	pdf.ClipEnd()
	pdf.AddPage()
	pdf.SetFillColor(240, 240, 200)
	pdf.Rect(0, 0, a5W, a5H, "F")
	png := pngAlpha()
	pdf.RegisterImageOptionsReader("alpha", fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(png))
	pdf.ImageOptions("alpha", 24, 60, 100, 100, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	p := save(t, pdf, "fpdf-image.pdf")

	renderPNG(t, p, 0, "fpdf-image-cover.png")
	renderPNG(t, p, 1, "fpdf-image-alpha.png")
	if n := pageCount(t, p); n != 2 {
		t.Errorf("pages = %d", n)
	}
	out := must(os.ReadFile(p))
	if dpi < 300 {
		t.Errorf("effective DPI %.0f < 300", dpi)
	}
	if !bytes.Contains(out, jpg) {
		t.Errorf("JPEG bytes were re-encoded (source not found verbatim in the PDF)")
	}
	if !bytes.Contains(out, []byte("/SMask")) {
		t.Errorf("PNG alpha not written as an SMask")
	}
	t.Logf("cover on A5: image placed %.1fx%.1f mm at (%.1f,%.1f), effective %.0f DPI; PDF %d bytes vs JPEG %d bytes (+PNG %d)",
		w, h, x, y, dpi, len(out), len(jpg), len(png))
}

func TestFpdf_Layout(t *testing.T) {
	ensurePhotos()
	jpg := must(os.ReadFile(photoPath(1)))
	pdf := newFpdf("A5")
	pdf.AddPage()
	pdf.RegisterImageOptionsReader("p1", fpdf.ImageOptions{ImageType: "JPG"}, bytes.NewReader(jpg))
	img := func(fx, fy, fw, fh float64) {
		x, y, w, h, _ := coverRect(photoW, photoH, fx, fy, fw, fh)
		pdf.ImageOptions("p1", x, y, w, h, false, fpdf.ImageOptions{ImageType: "JPG"}, 0, "")
	}
	pdf.SetFillColor(250, 235, 215)
	pdf.Rect(0, 0, a5W, a5H, "F")

	// rounded-rectangle and circular image clipping (cover-fitted into the frame)
	pdf.ClipRoundedRect(70, 10, 68, 50, 8, false)
	img(70, 10, 68, 50)
	pdf.ClipEnd()
	pdf.ClipCircle(30, 60, 20, false)
	img(10, 40, 40, 40)
	pdf.ClipEnd()

	// rotated image and rotated text
	pdf.TransformBegin()
	pdf.TransformRotate(12, 105, 94)
	pdf.ClipRect(80, 75, 50, 38, false)
	img(80, 75, 50, 38)
	pdf.ClipEnd()
	pdf.TransformEnd()
	pdf.TransformBegin()
	pdf.TransformRotate(-20, 10, 100)
	pdf.SetFont("bvp", "B", 16)
	pdf.SetTextColor(200, 30, 60)
	pdf.Text(10, 100, "Kỷ niệm 2026")
	pdf.TransformEnd()

	// wrapped text in a box: left and centre, explicit line height, colour and opacity
	text := norm.NFC.String(vietnamese)
	pdf.SetTextColor(20, 20, 20)
	pdf.SetFont("bvp", "", 10)
	box := 60.0
	lines := pdf.SplitText(text, box)
	if len(lines) < 2 {
		t.Errorf("text did not wrap: %q", lines)
	}
	for _, l := range lines {
		if pdf.GetStringWidth(l) > box+0.01 {
			t.Errorf("line wider than the box: %q", l)
		}
	}
	pdf.SetXY(10, 120)
	pdf.MultiCell(box, 4.5, text, "1", "L", false) // line height 4.5 mm
	pdf.SetXY(78, 120)
	pdf.MultiCell(box, 7, text, "1", "C", false) // line height 7 mm
	pdf.SetAlpha(0.35, "Normal")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("bvp", "B", 30)
	pdf.Text(15, 185, "BẢN NHÁP")
	pdf.SetAlpha(1, "Normal")
	p := save(t, pdf, "fpdf-layout.pdf")
	if w, h := mediaBox(t, p); !near(w, 419.53, 0.6) || !near(h, 595.28, 0.6) {
		t.Errorf("A5 MediaBox = %.2f x %.2f", w, h)
	}

	renderPNG(t, p, 0, "fpdf-layout.png")
	a4 := newFpdf("A4")
	a4.AddPage()
	a4.SetFont("bvp", "", 12)
	a4.Text(10, 20, "A4 page")
	p4 := save(t, a4, "fpdf-a4.pdf")
	if w, h := mediaBox(t, p4); !near(w, 595.28, 0.6) || !near(h, 841.89, 0.6) {
		t.Errorf("A4 MediaBox = %.2f x %.2f", w, h)
	}
}

func TestBenchFpdf(t *testing.T) {
	ensurePhotos()
	start := time.Now()
	pdf := newFpdf("A5")
	for i := 0; i < nPhotos; i++ {
		pdf.AddPage()
		name := photoPath(i)
		pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "JPG"}, bytes.NewReader(must(os.ReadFile(name))))
		x, y, w, h, _ := coverRect(photoW, photoH, 10, 10, 128, 150)
		pdf.ClipRoundedRect(10, 10, 128, 150, 6, false)
		pdf.ImageOptions(name, x, y, w, h, false, fpdf.ImageOptions{ImageType: "JPG"}, 0, "")
		pdf.ClipEnd()
		pdf.SetFont("bvp", "", 11)
		pdf.SetXY(10, 168)
		pdf.MultiCell(128, 5.5, norm.NFC.String(vietnamese), "", "L", false)
	}
	p := save(t, pdf, "fpdf-bench.pdf")
	wall, rss := time.Since(start), peakRSSMB() // before PDFium starts: it would inflate the RSS
	st := must(os.Stat(p))
	if n := pageCount(t, p); n != nPhotos { // after the timing, so PDFium start-up is not counted
		t.Errorf("pages = %d", n)
	}
	t.Logf("BENCH lib=fpdf pages=%d photos=%d wall=%.2fs peak_rss=%.0fMB pdf=%.1fMB machine=%s",
		nPhotos, nPhotos, wall.Seconds(), rss, float64(st.Size())/1e6, machine())
}

// Pins the known fpdf limitation (still present in v0.12.0): any rune above U+FFFF panics in
// Write/MultiCell. Production code must never hand such a rune to fpdf (see toPUA and splitRuns).
func TestFpdf_NonBMPRunePanics(t *testing.T) {
	pdf := newFpdf("A5")
	pdf.AddPage()
	pdf.SetFont("emoji", "", 16)
	defer func() {
		if recover() == nil {
			t.Error("expected a panic: if fpdf fixed this, the PUA workaround can go")
		}
	}()
	pdf.Write(8, "🎓")
}
