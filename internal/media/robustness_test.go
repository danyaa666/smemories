package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/apperr"
)

// T-104. These tests replace the package seams decodeImage and encodeImage, so none of them is parallel.

func swapSeams(t *testing.T, dec func(io.Reader) (image.Image, string, error), enc func(image.Image, bool, int) ([]byte, error)) {
	t.Helper()
	d, e := decodeImage, encodeImage
	t.Cleanup(func() { decodeImage, encodeImage = d, e })
	if dec != nil {
		decodeImage = dec
	}
	if enc != nil {
		encodeImage = enc
	}
}

// wantPanicRefused fails unless err is the 400 invalid-image error that hides a recovered panic from the client.
func wantPanicRefused(t *testing.T, err error, where string) {
	t.Helper()
	var pe *panicError
	if !errors.As(err, &pe) || pe.where != where {
		t.Fatalf("err = %v, want a recovered panic in %q", err, where)
	}
	if c := apperr.CodeOf(err); c != CodeInvalidImage || c.HTTP != 400 {
		t.Errorf("code = %v, want ERROR_INVALID_IMAGE 400", c)
	}
	if strings.Contains(apperr.Message(err), "boom") {
		t.Errorf("client message leaks the panic: %q", apperr.Message(err))
	}
}

func TestProcessRecoversFromADecoderPanic(t *testing.T) {
	swapSeams(t, func(io.Reader) (image.Image, string, error) { panic("boom") }, nil)
	_, err := process(jpegWith(t, quad(40, 30, false)))
	wantPanicRefused(t, err, "process")
}

func TestProcessRecoversFromAnEncoderPanic(t *testing.T) {
	swapSeams(t, nil, func(image.Image, bool, int) ([]byte, error) { panic("boom") })
	_, err := process(jpegWith(t, quad(40, 30, false)))
	wantPanicRefused(t, err, "process")
}

// The print version is encoded in its own goroutine, which no HTTP middleware covers: without the recover this test kills the test binary.
func TestProcessRecoversFromAPanicInThePrintGoroutine(t *testing.T) {
	swapSeams(t, nil, func(img image.Image, alpha bool, q int) ([]byte, error) {
		if q == printQuality {
			panic("boom")
		}
		return encode(img, alpha, q)
	})
	_, err := process(jpegWith(t, quad(2000, 1500, false))) // long edge above printEdge: a print goroutine runs
	wantPanicRefused(t, err, "print")
}

// panicPixels is a decoded image whose pixels cannot be read: the scaler panics on it.
type panicPixels struct{ image.Rectangle }

func (p panicPixels) ColorModel() color.Model      { return color.NRGBAModel }
func (p panicPixels) Bounds() image.Rectangle      { return p.Rectangle }
func (panicPixels) At(int, int) color.Color        { panic("boom") }
func (panicPixels) RGBA64At(int, int) color.RGBA64 { panic("boom") } // the scaler prefers this one

func TestProcessRecoversFromAScalerPanic(t *testing.T) {
	swapSeams(t, func(io.Reader) (image.Image, string, error) {
		return panicPixels{image.Rect(0, 0, 40, 30)}, "jpeg", nil
	}, nil)
	_, err := process(jpegWith(t, quad(40, 30, false)))
	wantPanicRefused(t, err, "process")
}

func TestPrintOfRecoversFromAPanic(t *testing.T) {
	swapSeams(t, func(io.Reader) (image.Image, string, error) { panic("boom") }, nil)
	_, err := printOf(jpegWith(t, quad(40, 30, false)))
	wantPanicRefused(t, err, "printOf")
}

// A decoder that disagrees with its own header is refused after decode, and without a panic.
func TestProcessRefusesADecodedSizeThatDiffersFromTheHeader(t *testing.T) {
	data := jpegWith(t, quad(40, 30, false))
	for name, img := range map[string]image.Image{
		"bigger":  image.NewNRGBA(image.Rect(0, 0, 400, 30)),
		"smaller": image.NewNRGBA(image.Rect(0, 0, 40, 29)),
	} {
		swapSeams(t, func(io.Reader) (image.Image, string, error) { return img, "jpeg", nil }, nil)
		_, err := process(data)
		var pe *panicError
		if !errors.Is(err, ErrInvalidImage) || errors.As(err, &pe) {
			t.Errorf("%s: err = %v, want plain ErrInvalidImage", name, err)
		}
	}
}

// A lossy WebP with transparency whose alpha plane is shorter than its luma plane would panic in (*NYCbCrA).Opaque.
func TestProcessRefusesAShortAlphaPlane(t *testing.T) {
	data, err := os.ReadFile("testdata/rot6.webp") // lossy, 64x32
	if err != nil {
		t.Fatal(err)
	}
	luma, ok := decode(t, data).(*image.YCbCr)
	if !ok {
		t.Fatalf("rot6.webp decodes to %T, want *image.YCbCr", decode(t, data))
	}
	img := &image.NYCbCrA{YCbCr: *luma, A: make([]byte, 64*32/2), AStride: 64} // half the plane
	swapSeams(t, func(io.Reader) (image.Image, string, error) { return img, "webp", nil }, nil)
	_, err = process(data)
	var pe *panicError
	if !errors.Is(err, ErrInvalidImage) || errors.As(err, &pe) {
		t.Fatalf("err = %v, want plain ErrInvalidImage (refused before Opaque can panic)", err)
	}
}

// vp8x-canvas-mismatch.webp (58 bytes): a VP8X chunk with the alpha flag and a 1x1 canvas, a 1x1 ALPH chunk, and a VP8 key-frame header that
// declares 8000 x 8000 (64 MP, over the 25 MP cap). golang.org/x/image v0.46.0 webp.decode compares the frame header with the canvas
// before it allocates (decode.go, case fccVP8: "seenVP8X && fh.Width != widthMinusOne+1" is errInvalidFormat), so Decode refuses it
// and the tiny header never buys a big buffer; DecodeConfig alone reports the 1x1 canvas, which is why the check after decode exists too.
func TestProcessRefusesAWebPWhoseFrameIsLargerThanItsCanvas(t *testing.T) {
	data, err := os.ReadFile("testdata/vp8x-canvas-mismatch.webp")
	if err != nil {
		t.Fatal(err)
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil || cfg.Width != 1 || cfg.Height != 1 {
		t.Fatalf("DecodeConfig = %v, %v; the fixture should look like a 1x1 image", cfg, err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err = process(data)
	runtime.ReadMemStats(&after)
	var pe *panicError
	if !errors.Is(err, ErrInvalidImage) || errors.As(err, &pe) {
		t.Fatalf("err = %v, want plain ErrInvalidImage", err)
	}
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 8<<20 { // an 8000 x 8000 YCbCr frame is 96 MB
		t.Errorf("refusing the fixture allocated %d MiB", grown>>20)
	}
}

func testService(slots int) (*Service, *bytes.Buffer, *int) {
	var logs bytes.Buffer
	panics := 0
	s := &Service{sem: make(chan struct{}, slots)}
	s.SetPanicReporter(slog.New(slog.NewTextHandler(&logs, nil)), func() { panics++ })
	return s, &logs, &panics
}

// Every way out of processPhoto gives the slot back, a panic in the loader or the pipeline included, and a panic is counted and logged once.
func TestProcessPhotoGivesTheSlotBackOnEveryPath(t *testing.T) {
	good := jpegWith(t, quad(40, 30, false))
	cases := []struct {
		name       string
		load       func() ([]byte, error)
		dec        func(io.Reader) (image.Image, string, error)
		wantPanic  string
		wantErr    bool
		wantCounts int
	}{
		{name: "ok", load: func() ([]byte, error) { return good, nil }},
		{name: "load error", load: func() ([]byte, error) { return nil, io.ErrUnexpectedEOF }, wantErr: true},
		{name: "not an image", load: func() ([]byte, error) { return []byte("hello"), nil }, wantErr: true},
		{name: "loader panics", load: func() ([]byte, error) { panic("boom") }, wantErr: true, wantPanic: "load", wantCounts: 1},
		{name: "decoder panics", load: func() ([]byte, error) { return good, nil }, wantErr: true, wantPanic: "process", wantCounts: 1,
			dec: func(io.Reader) (image.Image, string, error) { panic("boom") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.dec != nil {
				swapSeams(t, c.dec, nil)
			}
			s, logs, panics := testService(2)
			_, err := s.processPhoto(context.Background(), c.load, 0)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, want error: %v", err, c.wantErr)
			}
			if c.wantPanic != "" {
				wantPanicRefused(t, err, c.wantPanic)
				if !strings.Contains(logs.String(), "level=ERROR") || !strings.Contains(logs.String(), "where="+c.wantPanic) || strings.Count(logs.String(), "image processing panicked") != 1 {
					t.Errorf("want one ERROR line for %s, got %q", c.wantPanic, logs.String())
				}
			} else if logs.Len() != 0 {
				t.Errorf("unexpected log: %q", logs.String())
			}
			if *panics != c.wantCounts {
				t.Errorf("panics counted = %d, want %d", *panics, c.wantCounts)
			}
			if n := s.SlotsInUse(); n != 0 {
				t.Errorf("slots in use = %d, want 0", n)
			}
		})
	}
}

func TestProcessPhotoGivesTheSlotBackWhenTheWaitFails(t *testing.T) {
	s, _, _ := testService(1)
	s.sem <- struct{}{} // the only slot is taken
	if _, err := s.processPhoto(context.Background(), nil, 5e6); !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
	if n := s.SlotsInUse(); n != 1 { // still the other caller's
		t.Errorf("slots in use = %d, want 1", n)
	}
}

// T-113. alpha-lossy-2048.webp (8.7 KB, made with `cwebp -q 80 -alpha_q 100 -alpha_method 1 -exact` from a 2048 x 2048 RGBA PNG with a flat
// colour and 5 alpha levels in 32 px bands) is a lossy WebP with an ALPH chunk compressed with method 1. golang.org/x/image/webp decodes that alpha
// through vp8l, which builds a whole NRGBA (4 bytes per pixel) just to read its green channel, plus the packed palette indices (2 bytes per pixel
// for 5 to 16 levels, the worst case), on top of the VP8 frame (1.5) and the alpha plane (1).
// decodedBytes must bound what one image.Decode allocates; this fails if the estimate for lossy WebP drops back to 2.5 bytes per pixel.
func TestDecodedBytesBoundsALossyWebPWithAlpha(t *testing.T) {
	data, err := os.ReadFile("testdata/alpha-lossy-2048.webp")
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "webp" || cfg.Width != 2048 || cfg.Height != 2048 {
		t.Fatalf("DecodeConfig = %v, %q, %v; want 2048 x 2048 webp", cfg, format, err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	img, _, err := image.Decode(bytes.NewReader(data))
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := img.(*image.NYCbCrA); !ok {
		t.Fatalf("decoded %T, want *image.NYCbCrA (the fixture must keep its ALPH chunk)", img)
	}
	alloc, est := int(after.TotalAlloc-before.TotalAlloc), decodedBytes(data, cfg, format)
	t.Logf("2048 x 2048 lossy WebP with ALPH: decode allocated %d bytes (%.2f per pixel), estimate %d bytes (%.2f per pixel)",
		alloc, float64(alloc)/(2048*2048), est, float64(est)/(2048*2048))
	if est < alloc {
		t.Errorf("estimate %d bytes is below the %d bytes one decode allocated", est, alloc)
	}
	runtime.KeepAlive(img)
}

// The same estimate decides what is accepted: a 4000 x 4000 lossy WebP with alpha (140 MB estimated) is over the 128 MiB cap, 3800 x 3800 is not,
// and a lossy WebP without an ALPH chunk keeps its 2.5 bytes per pixel.
func TestWithinLimitsLossyWebPWithAlpha(t *testing.T) {
	alpha, err := os.ReadFile("testdata/alpha-lossy-2048.webp")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := os.ReadFile("testdata/rot6.webp")
	if err != nil {
		t.Fatal(err)
	}
	cfg := func(w int) image.Config { return image.Config{ColorModel: color.YCbCrModel, Width: w, Height: w} }
	for _, tc := range []struct {
		name string
		data []byte
		side int
		want bool
	}{
		{"alpha 3800", alpha, 3800, true},
		{"alpha 4000", alpha, 4000, false},
		{"alpha 5000", alpha, 5000, false},
		{"plain 5000", plain, 5000, true},
	} {
		if got := withinLimits(tc.data, cfg(tc.side), "webp"); got != tc.want {
			t.Errorf("%s: allowed = %v, want %v", tc.name, got, tc.want)
		}
	}
}
