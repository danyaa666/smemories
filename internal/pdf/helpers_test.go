package pdf

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"regexp"
	"strconv"
	"testing"
	"unicode/utf16"
)

// mapSource serves synthetic photos by id; unknown ids are an error, like a missing S3 object.
type mapSource map[string][]byte

func (m mapSource) Image(_ context.Context, id string) ([]byte, error) {
	if b, ok := m[id]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("media %q not found", id)
}

// synthPhoto draws a colour gradient with a dark frame marker and fine noise (never a real photo).
func synthPhoto(w, h, seed int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := uint32(seed)*2654435761 + 12345
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rng ^= rng << 13
			rng ^= rng >> 17
			rng ^= rng << 5
			n := int(rng%17) - 8
			c := color.RGBA{clamp(40 + x*200/w + n + seed*13), clamp(60 + y*180/h + n), clamp(200 - x*120/w + n), 255}
			if x < 8 || y < 8 || x >= w-8 || y >= h-8 {
				c = color.RGBA{20, 20, 20, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func clamp(v int) uint8 { return uint8(max(0, min(255, v))) }

func synthJPEG(t testing.TB, w, h, seed int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, synthPhoto(w, h, seed), &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func synthPNG(t testing.TB, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, synthPhoto(w, h, 3)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func synthGIF(t testing.TB) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := gif.Encode(&b, image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// ---- reading the PDF back (fpdf output, no external tools) ----

var streamRe = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)

// pageStreams returns the decompressed content streams of the first n pages. fpdf writes the page
// objects (each followed by its content stream) before fonts and images.
func pageStreams(t testing.TB, pdf []byte, n int) []string {
	t.Helper()
	var out []string
	for _, m := range streamRe.FindAllSubmatch(pdf, n) {
		zr, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			t.Fatalf("page stream is not zlib data: %v", err)
		}
		b, err := io.ReadAll(zr)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	if len(out) != n {
		t.Fatalf("found %d page streams, want %d", len(out), n)
	}
	return out
}

// drawn is one Text call: position in PDF points (origin bottom-left) and decoded text.
type drawn struct {
	x, y float64
	text string
}

var tjRe = regexp.MustCompile(`(?s)BT ([-\d.]+) ([-\d.]+) Td \(((?:\\.|[^\\)])*)\) Tj ET`)

func unescapePDF(s string) []byte {
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'r':
				b = append(b, '\r')
			case 'n':
				b = append(b, '\n')
			default:
				b = append(b, s[i])
			}
			continue
		}
		b = append(b, s[i])
	}
	return b
}

// textsOf decodes every Text call of a page stream (UTF-16BE text, as fpdf writes it).
func textsOf(stream string) []drawn {
	var out []drawn
	for _, m := range tjRe.FindAllStringSubmatch(stream, -1) {
		x, _ := strconv.ParseFloat(m[1], 64)
		y, _ := strconv.ParseFloat(m[2], 64)
		raw := unescapePDF(m[3])
		u := make([]uint16, 0, len(raw)/2)
		for i := 0; i+1 < len(raw); i += 2 {
			u = append(u, uint16(raw[i])<<8|uint16(raw[i+1]))
		}
		out = append(out, drawn{x, y, string(utf16.Decode(u))})
	}
	return out
}

func allText(stream string) string {
	var s string
	for _, d := range textsOf(stream) {
		s += d.text
	}
	return s
}

var imgRe = regexp.MustCompile(`q ([-\d.]+) 0 0 ([-\d.]+) ([-\d.]+) ([-\d.]+) cm /I[0-9a-f]+ Do Q`)

type placedImage struct{ w, h, x, y float64 } // PDF points

func imagesOf(stream string) []placedImage {
	var out []placedImage
	for _, m := range imgRe.FindAllStringSubmatch(stream, -1) {
		var p [4]float64
		for i := range p {
			p[i], _ = strconv.ParseFloat(m[i+1], 64)
		}
		out = append(out, placedImage{p[0], p[1], p[2], p[3]})
	}
	return out
}

var mediaBoxRe = regexp.MustCompile(`/MediaBox\s*\[\s*0(?:\.0+)?\s+0(?:\.0+)?\s+([\d.]+)\s+([\d.]+)\s*\]`)

func mediaBox(t testing.TB, pdf []byte) (w, h float64) {
	t.Helper()
	m := mediaBoxRe.FindSubmatch(pdf)
	if m == nil {
		t.Fatal("no MediaBox")
	}
	w, _ = strconv.ParseFloat(string(m[1]), 64)
	h, _ = strconv.ParseFloat(string(m[2]), 64)
	return w, h
}
