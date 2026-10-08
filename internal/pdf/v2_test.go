package pdf

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/danyaa666/smemories/internal/pdf/fonts"
	"github.com/danyaa666/smemories/internal/templates"
)

// A second, regular-only family for the tests: the bold face of Be Vietnam Pro under another name.
func init() { fonts.Register("TestFixture", fonts.Family{Regular: fonts.Bold}) }

// gradientPNG is a smooth picture (it compresses well, like a flat illustration) of the given pixel size.
func gradientPNG(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 180, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// v2Spec is a template that uses every format-v2 feature; %s is the body of the cover page.
const v2Spec = `{
  "id": "v2test", "name": {"en": "V2", "vi": "V2"}, "unit": "mm", "page_sizes": ["A5", "A4"],
  "theme": {"fonts": {"body": "BeVietnamPro", "display": "TestFixture"}, "colors": {"ink": "#000000", "accent": "#aa0000", "paper": "#ffffff"}},
  "pages": [
    {"kind": "cover", "elements": [%s]},
    {"kind": "profile", "elements": [{"type": "background", "asset": "v2test/bg.jpg"}]},
    {"kind": "notes", "elements": [{"type": "background", "asset": "v2test/bg.jpg"}],
     "flow": {"x": 10, "y": 10, "w": 128, "h": 190, "gap": 4, "item_h": 60, "elements": [
       {"type": "text", "slot": "note_message", "x": 0, "y": 0, "w": 100, "h": 20, "size": 10}]}},
    {"kind": "back", "elements": []}
  ]}`

const v2Cover = `
  {"type": "background", "asset": "v2test/bg.png"},
  {"type": "text", "text": {"en": "All About Me", "vi": "Về mình"}, "x": 20, "y": 18, "w": 90, "h": 14, "size": 22, "font": "display", "color": "accent", "rotate": -2},
  {"type": "image", "slot": "cover_photo", "x": 30, "y": 40, "w": 60, "h": 40, "shape": "ellipse", "rotate": 3},
  {"type": "rect", "x": 10, "y": 100, "w": 40, "h": 10, "fill": "accent", "rotate": 45},
  {"type": "text", "slot": "title", "x": 10, "y": 120, "w": 100, "h": 12, "size": 14}`

func v2Assets(t testing.TB) fstest.MapFS {
	return fstest.MapFS{
		"v2test/bg.png": {Data: gradientPNG(t, 1240, 1760)},
		"v2test/bg.jpg": {Data: synthJPEG(t, 1240, 1760, 5)},
	}
}

func v2Template(t testing.TB, cover string) *templates.Template {
	t.Helper()
	tt, err := templates.ParseFS([]byte(fmt.Sprintf(v2Spec, cover)), v2Assets(t))
	if err != nil {
		t.Fatal(err)
	}
	return tt
}

func renderV2(t testing.TB, tt *templates.Template, o Options) ([]byte, Report) {
	t.Helper()
	src := mapSource{"c": synthJPEG(t, 900, 600, 1)}
	o.Now = fixedNow
	var buf bytes.Buffer
	rep, err := Render(context.Background(), tt, Book{Title: "Kỷ yếu 🎓", CoverPhoto: "c", Notes: []Note{{ID: "n", Message: "Hi"}}}, src, &buf, o)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.Bytes(), rep
}

// matrixRe finds the transforms fpdf writes for TransformBegin/TransformRotate.
var matrixRe = regexp.MustCompile(`q\n([-\d.]+) ([-\d.]+) ([-\d.]+) ([-\d.]+) ([-\d.]+) ([-\d.]+) cm\n`)

// rotations returns the clockwise rotation in degrees of every rotated element of a page stream, in order.
func rotations(stream string) []float64 {
	var out []float64
	for _, m := range matrixRe.FindAllStringSubmatch(stream, -1) {
		a, _ := strconv.ParseFloat(m[1], 64)
		b, _ := strconv.ParseFloat(m[2], 64)
		out = append(out, math.Round(-math.Atan2(b, a)*180/math.Pi*100)/100) // PDF y points up: b > 0 is counter-clockwise
	}
	return out
}

var textTokenRe = regexp.MustCompile(`\((?:\\.|[^\\)])*\) Tj`)

// balanced reports whether every graphics-state save (q) has its restore (Q).
func balanced(stream string) bool {
	stream = textTokenRe.ReplaceAllString(stream, "")
	return len(regexp.MustCompile(`\bq\b`).FindAllString(stream, -1)) == len(regexp.MustCompile(`\bQ\b`).FindAllString(stream, -1))
}

func TestV2FeaturesRenderCleanInBothLanguages(t *testing.T) {
	tt := v2Template(t, v2Cover)
	for lang, want := range map[string]string{"en": "All About Me", "vi": "Về mình", "": "All About Me", "fr": "All About Me"} {
		t.Run("lang="+lang, func(t *testing.T) {
			out, rep := renderV2(t, tt, Options{Lang: lang})
			if len(rep.Warnings) != 0 {
				t.Errorf("warnings: %s", codes(rep))
			}
			stream := pageStreams(t, out, 1)[0]
			if got := allText(stream); !strings.Contains(got, want) {
				t.Errorf("cover text %q lacks %q", got, want)
			}
			if got := rotations(stream); !slices.Equal(got, []float64{-2, 3, 45}) {
				t.Errorf("rotations = %v, want [-2 3 45] (clockwise degrees: text, image, rect)", got)
			}
			if !balanced(stream) {
				t.Error("q/Q are unbalanced: a transform or clip was left open")
			}
		})
	}
}

func TestEllipseClipIsCurved(t *testing.T) {
	photo := func(shape string) string {
		cover := fmt.Sprintf(`{"type": "image", "slot": "cover_photo", "x": 30, "y": 40, "w": 60, "h": 40, "shape": %q}`, shape)
		out, _ := renderV2(t, v2Template(t, cover), Options{})
		return pageStreams(t, out, 1)[0]
	}
	if s := photo("ellipse"); strings.Count(s, " c\n")+strings.Count(s, " c ") < 4 {
		t.Errorf("ellipse clip has no Bézier curves:\n%s", s)
	}
	if s := photo("rect"); strings.Contains(s, " c\n") {
		t.Error("a rect clip drew curves")
	}
}

func TestBackgroundIsFirstFullPageAndRegisteredOnce(t *testing.T) {
	jpg := v2Assets(t)["v2test/bg.jpg"].Data
	out, rep := renderV2(t, v2Template(t, v2Cover), Options{})
	if len(rep.Warnings) != 0 {
		t.Fatalf("warnings: %s", codes(rep))
	}
	streams := pageStreams(t, out, 4)
	// Page 1 (PNG background): paper fill, then the background, then everything else.
	s := streams[0]
	paper, bg, firstFont := strings.Index(s, " re f"), strings.Index(s, " Do Q"), strings.Index(s, " Tf")
	if paper < 0 || paper >= bg || bg >= firstFont {
		t.Errorf("order: paper %d, background %d, first text %d", paper, bg, firstFont)
	}
	pw, ph := 148*72/25.4, 210*72/25.4
	for i, st := range streams[:2] {
		im := imagesOf(st)[0]
		if math.Abs(im.w-pw) > 0.01 || math.Abs(im.h-ph) > 0.01 || im.x != 0 || im.y != 0 {
			t.Errorf("page %d background %+v does not fill the %.2f x %.2f page", i+1, im, pw, ph)
		}
	}
	// The JPEG is used by the profile and the notes page and embedded once, byte for byte (pass-through),
	// so the page pixel at any probe point is the asset's pixel.
	if n := bytes.Count(out, jpg); n != 1 {
		t.Errorf("the JPEG background is embedded %d times, want 1", n)
	}
	if n := bytes.Count(out, []byte("/Subtype /Image")); n != 3 { // PNG, JPEG, cover photo
		t.Errorf("%d image objects, want 3", n)
	}
}

func TestBackgroundOnEveryNotesPage(t *testing.T) {
	notes := make([]Note, 7)
	for i := range notes {
		notes[i] = Note{ID: fmt.Sprint(i), Message: "x"}
	}
	var buf bytes.Buffer
	rep, err := Render(context.Background(), v2Template(t, v2Cover), Book{Notes: notes}, nil, &buf, Options{Now: fixedNow})
	if err != nil || rep.Pages < 6 {
		t.Fatalf("pages %d, err %v", rep.Pages, err)
	}
	streams := pageStreams(t, buf.Bytes(), rep.Pages)
	for i, st := range streams[2 : rep.Pages-1] { // the notes pages
		if len(imagesOf(st)) == 0 {
			t.Errorf("notes page %d has no background", i+1)
		}
	}
	if len(imagesOf(streams[rep.Pages-1])) != 0 {
		t.Error("the back page has no background in its spec but got an image")
	}
	if n := bytes.Count(buf.Bytes(), []byte("/Subtype /Image")); n != 2 {
		t.Errorf("%d image objects for %d pages, want 2 (one PNG, one JPEG)", n, rep.Pages)
	}
}

func TestSecondFontFamily(t *testing.T) {
	embedded := func(cover string) (out []byte, used bool) {
		out, rep := renderV2(t, v2Template(t, cover), Options{})
		if len(rep.Warnings) != 0 {
			t.Errorf("warnings: %s", codes(rep))
		}
		return out, bytes.Contains(out, []byte("/BaseFont /utf8f-testfixture"))
	}
	body := `{"type": "text", "slot": "title", "x": 10, "y": 20, "w": 100, "h": 12, "size": 14}`
	// Asking for bold from a regular-only family works: the regular face is used for both weights.
	display := `{"type": "text", "slot": "title", "x": 10, "y": 20, "w": 100, "h": 12, "size": 14, "font": "display", "bold": true}`
	// The display family is only embedded when an element uses it (the theme names it in both cases).
	if _, used := embedded(body); used {
		t.Error("the display family was embedded although no element uses it")
	}
	out, used := embedded(display)
	if !used {
		t.Fatal("the display family was not embedded")
	}
	// Vietnamese and emoji work with the second family: no warning (checked above), glyphs drawn.
	if got := allText(pageStreams(t, out, 1)[0]); !strings.Contains(got, "Kỷ yếu") {
		t.Errorf("text %q", got)
	}
}

func TestStaticTextTooLongForItsBoxIsTruncatedNotFatal(t *testing.T) {
	long := strings.Repeat("Một hai ba bốn năm ", 25)[:480]
	cover := fmt.Sprintf(`{"type": "text", "text": {"en": %q, "vi": %q}, "x": 10, "y": 10, "w": 20, "h": 6, "size": 12}`, long, long)
	_, rep := renderV2(t, v2Template(t, cover), Options{})
	if len(rep.Warnings) != 1 || rep.Warnings[0].Code != WarnTextTruncated || rep.Warnings[0].Slot != "" {
		t.Errorf("warnings = %+v, want one text_truncated without a slot", rep.Warnings)
	}
}

func TestRotationLimits(t *testing.T) {
	for _, deg := range []float64{45, -45} {
		cover := fmt.Sprintf(`{"type": "text", "text": {"en": "Tilt", "vi": "Nghiêng"}, "x": 20, "y": 20, "w": 90, "h": 14, "size": 20, "rotate": %g}`, deg)
		out, rep := renderV2(t, v2Template(t, cover), Options{})
		s := pageStreams(t, out, 1)[0]
		if got := rotations(s); !slices.Equal(got, []float64{deg}) || len(rep.Warnings) != 0 || !balanced(s) {
			t.Errorf("rotate %g: rotations %v, warnings %s, balanced %v", deg, got, codes(rep), balanced(s))
		}
	}
}

// T-038 export budget: a Letter-size book of 24 pages, 30 photos and a full-page background on every page
// must stay within the export budget (60 s, 512 MB). Run: go test ./internal/pdf -run '^$' -bench LetterBook -benchtime 1x
func BenchmarkLetterBookWithBackgrounds(b *testing.B) {
	const spec = `{
  "id": "budget", "name": {"en": "B", "vi": "B"}, "unit": "mm", "reference": "Letter", "page_sizes": ["Letter"],
  "theme": {"font": "BeVietnamPro", "colors": {"ink": "#000000", "accent": "#aa0000", "paper": "#ffffff"}},
  "pages": [
    {"kind": "cover", "elements": [{"type": "background", "asset": "budget/bg.png"},
      {"type": "image", "slot": "cover_photo", "x": 20, "y": 40, "w": 170, "h": 120}]},
    {"kind": "profile", "elements": [{"type": "background", "asset": "budget/bg.png"},
      {"type": "image", "slot": "photo", "x": 20, "y": 40, "w": 100, "h": 100, "shape": "ellipse"}]},
    {"kind": "notes", "elements": [{"type": "background", "asset": "budget/bg.png"}],
     "flow": {"x": 10, "y": 10, "w": 195, "h": 250, "gap": 4, "item_h": 250, "elements": [
       {"type": "text", "slot": "note_message", "x": 0, "y": 0, "w": 100, "h": 40, "size": 10},
       {"type": "image", "slot": "note_photo_1", "x": 110, "y": 0, "w": 80, "h": 60, "shape": "rounded", "radius": 3},
       {"type": "image", "slot": "note_photo_2", "x": 110, "y": 70, "w": 80, "h": 60}]}},
    {"kind": "back", "elements": [{"type": "background", "asset": "budget/bg.png"}]}
  ]}`
	// 1700 x 2200 px is 200 DPI on Letter. The gradient has a few banding steps, like a flat illustration.
	tt, err := templates.ParseFS([]byte(spec), fstest.MapFS{"budget/bg.png": {Data: gradientPNG(b, 1700, 2200)}})
	if err != nil {
		b.Fatal(err)
	}
	src := mapSource{"cover": synthJPEG(b, 2400, 1800, 1), "me": synthJPEG(b, 2400, 1800, 2)}
	book := Book{Title: "Budget", CoverPhoto: "cover", Profile: Profile{FullName: "A", PhotoID: "me"}}
	photos := 2
	for i := range 21 { // 3 other pages + 21 notes pages = 24 pages
		n := Note{ID: fmt.Sprint(i), Message: "Chúc mừng tốt nghiệp"}
		for j := range 1 + i/7 { // 1 to 3 photos per note: 21 + 7 = 28 plus cover and profile = 30
			if photos == 30 {
				break
			}
			id := fmt.Sprintf("p%d-%d", i, j)
			src[id] = synthJPEG(b, 2400, 1800, i+j)
			n.PhotoIDs = append(n.PhotoIDs, id)
			photos++
		}
		book.Notes = append(book.Notes, n)
	}
	b.ResetTimer()
	for range b.N {
		stop, peak := make(chan struct{}), make(chan uint64)
		go func() { // peak heap in use, sampled every 5 ms
			var peakHeap uint64
			var m runtime.MemStats
			for {
				select {
				case <-stop:
					peak <- peakHeap
					return
				case <-time.After(5 * time.Millisecond):
					runtime.ReadMemStats(&m)
					peakHeap = max(peakHeap, m.HeapInuse)
				}
			}
		}()
		var buf bytes.Buffer
		rep, err := Render(context.Background(), tt, book, src, &buf, Options{PageSize: "Letter", Now: fixedNow})
		close(stop)
		p := <-peak
		if err != nil || rep.Pages != 24 {
			b.Fatalf("pages %d, err %v", rep.Pages, err)
		}
		b.ReportMetric(float64(p)/(1<<20), "peak-heap-MB")
		b.ReportMetric(float64(buf.Len())/(1<<20), "pdf-MB")
	}
}
