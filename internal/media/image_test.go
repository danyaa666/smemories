package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
)

// quad returns a w x h image that is white except for a red 16x16 block in the top-left corner.
func quad(w, h int, alpha bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	bg := color.NRGBA{255, 255, 255, 255}
	if alpha {
		bg = color.NRGBA{}
	}
	for y := range h {
		for x := range w {
			if x < 16 && y < 16 {
				img.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
			} else {
				img.SetNRGBA(x, y, bg)
			}
		}
	}
	return img
}

// exifSegment builds an APP1 segment with the given orientation and a GPS IFD (latitude reference).
func exifSegment(orientation uint16) []byte {
	var t bytes.Buffer
	w := func(vs ...any) {
		for _, v := range vs {
			if err := binary.Write(&t, binary.BigEndian, v); err != nil {
				panic(err)
			}
		}
	}
	t.WriteString("MM\x00*")
	w(uint32(8))
	w(uint16(2))                                                    // IFD0: orientation + GPS pointer
	w(uint16(0x0112), uint16(3), uint32(1), orientation, uint16(0)) // tag, SHORT, count, value
	w(uint16(0x8825), uint16(4), uint32(1), uint32(8+2+24+4))       // GPSInfo: offset of the GPS IFD
	w(uint32(0))
	w(uint16(1)) // GPS IFD: GPSLatitudeRef = "N"
	w(uint16(1), uint16(2), uint32(2), [4]byte{'N', 0, 0, 0})
	w(uint32(0))
	seg := append([]byte("Exif\x00\x00"), t.Bytes()...)
	out := []byte{0xFF, 0xE1, byte((len(seg) + 2) >> 8), byte(len(seg) + 2)}
	return append(out, seg...)
}

func segment(marker byte, payload string) []byte {
	return append([]byte{0xFF, marker, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}, payload...)
}

// jpegWith encodes img and inserts extra segments right after SOI.
func jpegWith(t *testing.T, img image.Image, extra ...[]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	out := append([]byte{}, b[:2]...)
	for _, e := range extra {
		out = append(out, e...)
	}
	return append(out, b[2:]...)
}

func encPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// markers lists the marker bytes of the header segments of a JPEG (up to SOS).
func markers(t *testing.T, d []byte) []byte {
	t.Helper()
	var out []byte
	for i := 2; i+4 <= len(d); {
		if d[i] != 0xFF {
			t.Fatalf("not a marker at %d", i)
		}
		m := d[i+1]
		out = append(out, m)
		if m == 0xDA {
			break
		}
		i += 2 + int(binary.BigEndian.Uint16(d[i+2:]))
	}
	return out
}

func decode(t *testing.T, d []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(d))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func isRed(c color.Color) bool { r, g, b, _ := c.RGBA(); return r>>8 > 200 && g>>8 < 90 && b>>8 < 90 }
func isWhite(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r>>8 > 200 && g>>8 > 200 && b>>8 > 200
}

func TestProcessStripsAllMetadata(t *testing.T) {
	in := jpegWith(t, quad(64, 32, false), exifSegment(1), segment(0xE2, "ICC_PROFILE\x00secret"), segment(0xFE, "comment with a name"), segment(0xED, "Photoshop 3.0"))
	if !bytes.Contains(in, []byte("Exif")) || !bytes.Contains(in, []byte("comment with a name")) {
		t.Fatal("fixture should carry metadata")
	}
	out, err := process(in)
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"display": out.display, "thumb": out.thumb} {
		for _, m := range markers(t, b) {
			if m >= 0xE0 && m <= 0xEF || m == 0xFE { // APPn and COM segments
				t.Errorf("%s keeps segment FF%02X", name, m)
			}
		}
		for _, s := range []string{"Exif", "ICC_PROFILE", "comment with a name", "Photoshop"} {
			if bytes.Contains(b, []byte(s)) {
				t.Errorf("%s still contains %q", name, s)
			}
		}
	}
}

func TestProcessAppliesOrientation(t *testing.T) {
	// Where the source's top-left red block ends up for EXIF orientation 1..8.
	for o, c := range []struct {
		w, h   int
		corner string
	}{{64, 32, "TL"}, {64, 32, "TR"}, {64, 32, "BR"}, {64, 32, "BL"}, {32, 64, "TL"}, {32, 64, "TR"}, {32, 64, "BR"}, {32, 64, "BL"}} {
		out, err := process(jpegWith(t, quad(64, 32, false), exifSegment(uint16(o+1))))
		if err != nil {
			t.Fatalf("orientation %d: %v", o+1, err)
		}
		if out.width != c.w || out.height != c.h {
			t.Fatalf("orientation %d: size %dx%d, want %dx%d", o+1, out.width, out.height, c.w, c.h)
		}
		img := decode(t, out.display)
		x, y := 6, 6
		if strings.HasSuffix(c.corner, "R") {
			x = c.w - 7
		}
		if strings.HasPrefix(c.corner, "B") {
			y = c.h - 7
		}
		if !isRed(img.At(x, y)) {
			t.Errorf("orientation %d: expected the red block at %s, pixel (%d,%d) is %v", o+1, c.corner, x, y, img.At(x, y))
		}
		if !isWhite(img.At(c.w/2, c.h/2)) {
			t.Errorf("orientation %d: centre is not white", o+1)
		}
	}
}

func TestProcessWebP(t *testing.T) {
	data, err := os.ReadFile("testdata/rot6.webp") // lossy, 64x32, EXIF orientation 6, red block top-left
	if err != nil {
		t.Fatal(err)
	}
	out, err := process(data)
	if err != nil {
		t.Fatal(err)
	}
	if out.contentType != "image/jpeg" || out.width != 32 || out.height != 64 {
		t.Fatalf("got %s %dx%d, want image/jpeg 32x64", out.contentType, out.width, out.height)
	}
	if img := decode(t, out.display); !isRed(img.At(32-7, 6)) {
		t.Errorf("WebP orientation not applied: pixel is %v", img.At(32-7, 6))
	}
}

func TestProcessTransparency(t *testing.T) {
	cases := map[string][]byte{"png": encPNG(t, quad(32, 32, true))}
	w, err := os.ReadFile("testdata/alpha.webp")
	if err != nil {
		t.Fatal(err)
	}
	cases["webp"] = w
	for name, in := range cases {
		out, err := process(in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out.contentType != "image/png" || out.ext != "png" {
			t.Fatalf("%s: stored as %s, want PNG", name, out.contentType)
		}
		if _, _, _, a := decode(t, out.display).At(30, 14).RGBA(); a != 0 {
			t.Errorf("%s: transparency lost (alpha %d)", name, a)
		}
		if th := decode(t, out.thumb); !isWhite(th.At(th.Bounds().Dx()-1, th.Bounds().Dy()-1)) { // flattened onto white
			t.Errorf("%s: thumbnail is not flattened onto white", name)
		}
	}
}

func TestProcessOpaquePNGBecomesJPEG(t *testing.T) {
	out, err := process(encPNG(t, quad(32, 32, false)))
	if err != nil || out.contentType != "image/jpeg" || out.ext != "jpg" {
		t.Fatalf("got %v, %v", out.contentType, err)
	}
}

func TestProcessResizeAndThumb(t *testing.T) {
	big := image.NewGray(image.Rect(0, 0, 4000, 1000))
	out, err := process(encPNG(t, big))
	if err != nil {
		t.Fatal(err)
	}
	if out.width != 3000 || out.height != 750 {
		t.Fatalf("display %dx%d, want 3000x750", out.width, out.height)
	}
	if b := decode(t, out.thumb).Bounds(); b.Dx() != 480 || b.Dy() != 120 {
		t.Fatalf("thumb %v, want 480x120", b)
	}
	small, err := process(encPNG(t, image.NewNRGBA(image.Rect(0, 0, 100, 50))))
	if err != nil {
		t.Fatal(err)
	}
	if small.width != 100 || small.height != 50 { // never upscaled
		t.Fatalf("small image became %dx%d", small.width, small.height)
	}
	if b := decode(t, small.thumb).Bounds(); b.Dx() != 100 || b.Dy() != 50 {
		t.Fatalf("small thumb %v", b)
	}
}

// pngHeaderClaiming returns a PNG whose IHDR claims w x h but which holds a single pixel of data.
func pngHeaderClaiming(t *testing.T, w, h uint32) []byte {
	t.Helper()
	b := encPNG(t, image.NewGray(image.Rect(0, 0, 1, 1)))
	binary.BigEndian.PutUint32(b[16:], w)
	binary.BigEndian.PutUint32(b[20:], h)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29])) // keep the IHDR checksum valid
	return b
}

func TestProcessRejects(t *testing.T) {
	var g bytes.Buffer
	_ = gif.Encode(&g, image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White}), nil)
	good := jpegWith(t, quad(64, 32, false))
	for name, c := range map[string]struct {
		in   []byte
		want error
	}{
		"empty":                {nil, ErrUnsupported},
		"gif":                  {g.Bytes(), ErrUnsupported},
		"html":                 {[]byte("<html><script>alert(1)</script></html>"), ErrUnsupported},
		"svg":                  {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>1</script></svg>`), ErrUnsupported},
		"windows executable":   {append([]byte("MZ\x90\x00"), make([]byte, 600)...), ErrUnsupported},
		"jpeg header, no body": {[]byte("\xff\xd8\xff<html>"), ErrInvalidImage},
		"truncated jpeg":       {good[:len(good)/2], ErrInvalidImage},
		"truncated png":        {encPNG(t, quad(64, 32, false))[:40], ErrInvalidImage},
		"png claims 60000 px":  {pngHeaderClaiming(t, 60000, 60000), ErrInvalidImage},
		"png claims 4G px":     {pngHeaderClaiming(t, 1<<31, 1<<31), ErrInvalidImage},
		"over 12000 px wide":   {encPNG(t, image.NewGray(image.Rect(0, 0, 12001, 1))), ErrInvalidImage},
		"over 50 megapixels":   {pngHeaderClaiming(t, 8000, 7000), ErrInvalidImage},
	} {
		if _, err := process(c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", name, err, c.want)
		}
	}
	if _, err := process(encPNG(t, image.NewGray(image.Rect(0, 0, 12000, 20)))); err != nil { // exactly the side limit
		t.Errorf("12000 px wide should pass: %v", err)
	}
}

func TestProcessPolyglotLosesItsPayload(t *testing.T) {
	in := append(jpegWith(t, quad(64, 32, false)), "<script>alert(1)</script>"...)
	out, err := process(in)
	if err != nil {
		return // rejecting it is fine too
	}
	if bytes.Contains(out.display, []byte("<script")) || bytes.Contains(out.thumb, []byte("<script")) {
		t.Fatal("HTML payload survived re-encoding")
	}
}

func TestExifParsersSurviveTruncation(t *testing.T) {
	in := jpegWith(t, quad(16, 16, false), exifSegment(6))
	if jpegOrientation(in) != 6 {
		t.Fatal("fixture orientation not read")
	}
	for i := range in {
		jpegOrientation(in[:i]) // must not panic
		tiffOrientation(in[:i])
		webpOrientation(in[:i])
	}
	bad := jpegWith(t, quad(16, 16, false), []byte{0xFF, 0xE1, 0xFF, 0xFF}) // segment length past the end
	if jpegOrientation(bad) != 1 {
		t.Fatal("malformed segment should read as upright")
	}
}
