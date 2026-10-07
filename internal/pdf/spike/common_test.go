//go:build spike

package spike

import (
	"bytes"
	"fmt"
	"github.com/klippa-app/go-pdfium"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/text/unicode/norm"
)

const (
	vietnamese = "Chúc mừng tốt nghiệp! Đặng Thị Hồng, Nguyễn Quỳnh Phương, Trần Văn Ưu"
	emojiLine  = "🎓🎉❤"
	missing    = "\U00013000" // Egyptian hieroglyph: in none of our fonts
	a5W, a5H   = 148.0, 210.0
	photoW     = 4000
	photoH     = 3000
	nPhotos    = 40
)

var (
	fontDir   = filepath.Join("..", "fonts")
	outDir    = "out"
	photosDir = filepath.Join(outDir, "photos")
	assetsDir = filepath.Join("..", "..", "..", "docs", "adr", "0002-assets")
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func readFont(name string) []byte { return must(os.ReadFile(filepath.Join(fontDir, name))) }

func outPath(name string) string {
	must(0, os.MkdirAll(outDir, 0o755))
	return filepath.Join(outDir, name)
}

func photoPath(i int) string { return filepath.Join(photosDir, fmt.Sprintf("photo%02d.jpg", i)) }

// ---- synthetic fixtures (never real people) ----

// makePhoto draws a smooth colour gradient with soft circles and fine noise, which
// JPEG-compresses to a few MB like a phone photo does.
func makePhoto(seed int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, photoW, photoH))
	rng := uint32(seed*2654435761 + 12345)
	next := func() uint32 { rng ^= rng << 13; rng ^= rng >> 17; rng ^= rng << 5; return rng }
	cx, cy := float64(next()%photoW), float64(next()%photoH)
	hue := float64(seed*37%360) / 360
	for y := 0; y < photoH; y++ {
		for x := 0; x < photoW; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy) / 2500
			r := 255 * (0.5 + 0.5*math.Sin(6.28*(hue+d)))
			g := 255 * (0.5 + 0.5*math.Sin(6.28*(hue+d+0.33)))
			b := 255 * (0.5 + 0.5*math.Sin(6.28*(hue+d+0.66)))
			// A frame marker so cropping is visible: dark border 40 px wide.
			if x < 40 || y < 40 || x >= photoW-40 || y >= photoH-40 {
				r, g, b = 20, 20, 20
			}
			img.SetRGBA(x, y, color.RGBA{uint8(r), uint8(g), uint8(b), 255})
		}
	}
	for by := 0; by < photoH; by += 2 { // fine noise, like sensor grain
		for bx := 0; bx < photoW; bx += 2 {
			n := int(next()%21) - 10
			for y := by; y < by+2; y++ {
				for x := bx; x < bx+2; x++ {
					c := img.RGBAAt(x, y)
					img.SetRGBA(x, y, color.RGBA{clamp(int(c.R) + n), clamp(int(c.G) + n), clamp(int(c.B) + n), 255})
				}
			}
		}
	}
	return img
}

func clamp(v int) uint8 { return uint8(max(0, min(255, v))) }

// pngAlpha is a 400x400 disc with a radial alpha fade (needs real alpha support to look right).
func pngAlpha() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 400, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 400; x++ {
			d := math.Hypot(float64(x-200), float64(y-200)) / 200
			a := 255 * math.Max(0, 1-d)
			img.SetNRGBA(x, y, color.NRGBA{230, 60, 90, uint8(a)})
		}
	}
	var b bytes.Buffer
	must(0, png.Encode(&b, img))
	return b.Bytes()
}

// ensurePhotos writes the 40 benchmark photos once (cached in out/photos).
func ensurePhotos() {
	must(0, os.MkdirAll(photosDir, 0o755))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i := 0; i < nPhotos; i++ {
		if _, err := os.Stat(photoPath(i)); err == nil {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var b bytes.Buffer
			must(0, jpeg.Encode(&b, makePhoto(i), &jpeg.Options{Quality: 85}))
			must(0, os.WriteFile(photoPath(i), b.Bytes(), 0o644))
		}(i)
	}
	wg.Wait()
}

func TestFixtures(t *testing.T) {
	ensurePhotos()
	st, err := os.Stat(photoPath(0))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fixtures ready: %d photos %dx%d, photo00 = %.2f MB", nPhotos, photoW, photoH, float64(st.Size())/1e6)
}

// ---- cover crop math, shared by both engines ----

// coverRect returns where to place a pxW x pxH image so it covers the frame (fx,fy,fw,fh)
// completely, centred: the overflow is clipped away. Also returns the effective DPI.
func coverRect(pxW, pxH int, fx, fy, fw, fh float64) (x, y, w, h, dpi float64) {
	s := math.Max(fw/float64(pxW), fh/float64(pxH)) // mm per pixel
	w, h = float64(pxW)*s, float64(pxH)*s
	x, y = fx+(fw-w)/2, fy+(fh-h)/2
	return x, y, w, h, 25.4 / s
}

// ---- glyph coverage and per-rune font fallback ----

type run struct {
	font int // index into the font list; -1 = no font has the rune
	text string
}

func loadSfnt(data []byte) *sfnt.Font { return must(sfnt.Parse(data)) }

func hasGlyph(f *sfnt.Font, r rune) bool {
	var b sfnt.Buffer
	idx, err := f.GlyphIndex(&b, r)
	return err == nil && idx != 0
}

// splitRuns walks text (NFC-normalised first) and groups consecutive runes by the first font,
// in priority order, that has a glyph for them. Runes no font has come back as font = -1 so the
// caller can report them and draw a replacement instead of silently printing .notdef boxes.
func splitRuns(text string, fonts []*sfnt.Font) []run {
	var runs []run
	for _, r := range norm.NFC.String(text) {
		fi := -1
		if r == ' ' || r == '\n' { // whitespace sticks to the current run
			fi = -2
		} else {
			for i, f := range fonts {
				if hasGlyph(f, r) {
					fi = i
					break
				}
			}
		}
		if fi == -2 {
			if n := len(runs); n > 0 && runs[n-1].font == 0 { // spaces always use the primary font
				runs[n-1].text += string(r)
			} else {
				runs = append(runs, run{0, string(r)})
			}
			continue
		}
		if n := len(runs); n > 0 && runs[n-1].font == fi {
			runs[n-1].text += string(r)
		} else {
			runs = append(runs, run{fi, string(r)})
		}
	}
	return runs
}

// toPUA maps the supplementary-plane emoji U+1F000..U+1FAFF to U+E000..U+EAFF, the aliases
// added to NotoEmoji-Regular.ttf (see ../fonts/README.md). Needed for go-pdf/fpdf only: it indexes
// glyph widths with a 64K table and panics on any rune above U+FFFF.
func toPUA(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 0x1F000 && r <= 0x1FAFF {
			return 0xE000 + r - 0x1F000
		}
		return r
	}, s)
}

func TestSplitRuns(t *testing.T) {
	fonts := []*sfnt.Font{loadSfnt(readFont("BeVietnamPro-Regular.ttf")), loadSfnt(readFont("NotoEmoji-Regular.ttf"))}
	runs := splitRuns("Chúc mừng 🎓🎉❤ ok "+missing+"!", fonts)
	want := []run{{0, "Chúc mừng "}, {1, "🎓🎉❤"}, {0, " ok "}, {-1, missing}, {0, "!"}}
	if fmt.Sprint(runs) != fmt.Sprint(want) {
		t.Fatalf("runs = %v, want %v", runs, want)
	}
}

// ---- verification helpers ----

// PDFium (the engine behind Chrome's PDF viewer) compiled to WebAssembly and run by wazero:
// pure Go, no cgo. It is both our "Go extractor" and our "opens in Chrome" check.
var (
	pdfiumOnce sync.Once
	pdfiumInst pdfium.Pdfium
)

func pdfiumInstance() pdfium.Pdfium {
	pdfiumOnce.Do(func() {
		pool := must(webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: 1, MaxTotal: 1}))
		pdfiumInst = must(pool.GetInstance(time.Minute))
	})
	return pdfiumInst
}

func openDoc(t *testing.T, path string) references.FPDF_DOCUMENT {
	t.Helper()
	doc, err := pdfiumInstance().OpenDocument(&requests.OpenDocument{FilePath: &path})
	if err != nil {
		t.Fatalf("PDFium cannot open %s: %v", path, err)
	}
	return doc.Document
}

func pageRef(doc references.FPDF_DOCUMENT, i int) requests.Page {
	return requests.Page{ByIndex: &requests.PageByIndex{Document: doc, Index: i}}
}

// extractText returns the text of page index i as PDFium reads it.
func extractText(t *testing.T, path string, i int) string {
	t.Helper()
	res, err := pdfiumInstance().GetPageText(&requests.GetPageText{Page: pageRef(openDoc(t, path), i)})
	if err != nil {
		t.Fatalf("extract %s: %v", path, err)
	}
	return res.Text
}

// pageCount also proves the file opens in PDFium.
func pageCount(t *testing.T, path string) int {
	t.Helper()
	res, err := pdfiumInstance().FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: openDoc(t, path)})
	if err != nil {
		t.Fatal(err)
	}
	return res.PageCount
}

// renderPNG writes page i of path to docs/adr/0002-assets/name, lowering the DPI until it is < 300 KB.
func renderPNG(t *testing.T, path string, i int, name string) {
	t.Helper()
	doc := openDoc(t, path)
	dst := filepath.Join(assetsDir, name)
	must(0, os.MkdirAll(assetsDir, 0o755))
	for dpi := 110; dpi >= 40; dpi -= 10 {
		res, err := pdfiumInstance().RenderPageInDPI(&requests.RenderPageInDPI{DPI: dpi, Page: pageRef(doc, i)})
		if err != nil {
			t.Fatalf("render %s: %v", path, err)
		}
		var b bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		must(0, enc.Encode(&b, res.Result.Image))
		if b.Len() < 300_000 {
			must(0, os.WriteFile(dst, b.Bytes(), 0o644))
			t.Logf("screenshot %s: %d dpi, %d KB", name, dpi, b.Len()/1000)
			return
		}
	}
	t.Fatalf("%s: could not get the screenshot under 300 KB", name)
}

// squash removes all whitespace so line-wrapping differences do not matter.
func squash(s string) string { return strings.Join(strings.Fields(s), "") }

func mediaBox(t *testing.T, path string) (w, h float64) {
	t.Helper()
	b := must(os.ReadFile(path))
	m := regexp.MustCompile(`/MediaBox\s*\[\s*0(?:\.0+)?\s+0(?:\.0+)?\s+([\d.]+)\s+([\d.]+)\s*\]`).FindSubmatch(b)
	if m == nil {
		t.Fatalf("%s: no MediaBox", path)
	}
	w, _ = strconv.ParseFloat(string(m[1]), 64)
	h, _ = strconv.ParseFloat(string(m[2]), 64)
	return
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func peakRSSMB() float64 {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	if runtime.GOOS == "darwin" {
		return float64(ru.Maxrss) / 1e6 // bytes
	}
	return float64(ru.Maxrss) / 1e3 // kilobytes
}

func machine() string {
	cpu := ""
	if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
		cpu = strings.TrimSpace(string(out)) + ", "
	}
	return fmt.Sprintf("%s%s/%s, %d CPUs, %s", cpu, runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version())
}

// noPanic is used around "unknown rune" handling: it must not panic in either engine.
func noPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s panicked: %v", what, r)
		}
	}()
	fn()
}
