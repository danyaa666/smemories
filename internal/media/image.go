package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers the WebP decoder
)

var (
	// ErrUnsupported: the content is not a JPEG, PNG or WebP image (decided by sniffing, never by name or Content-Type).
	ErrUnsupported = errors.New("media: unsupported image type")
	// ErrInvalidImage: the content looks like a supported image but is corrupt or too large to process safely.
	ErrInvalidImage = errors.New("media: invalid image")
)

const (
	maxSide          = 12000
	maxPixels        = 50_000_000 // JPEG; PNG and WebP are capped lower (maxPixelsPNGWebP) because they decode to more bytes per pixel
	maxDecodedMiB    = 128        // estimated bytes of the decoded image, from the header; see docs/media.md
	bandSize         = 128        // rows or columns scaled at once, bounds the scaler's float64 scratch buffer
	maxPixelsPNGWebP = 25_000_000
	displayEdge      = 3000
	thumbEdge        = 480
	displayQuality   = 92
	thumbQuality     = 80
)

// processed is the normalised result of an upload: metadata-free display and thumbnail encodings.
type processed struct {
	display, thumb []byte
	contentType    string // of display: image/jpeg, or image/png when the image has transparency
	ext            string // "jpg" or "png"
	width, height  int    // of display
}

// process validates an untrusted image and re-encodes it. Re-encoding is what strips metadata: the
// pixels are decoded, oriented, resized and written by Go's encoders, which emit no EXIF, GPS, ICC
// or comment segments, so nothing from the original file survives except the picture.
func process(data []byte) (processed, error) {
	if _, ok := sniff(data); !ok {
		return processed{}, ErrUnsupported
	}
	// Dimensions come from the header, before any pixel buffer exists (decompression-bomb guard).
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !withinLimits(data, cfg, format) {
		return processed{}, ErrInvalidImage
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return processed{}, ErrInvalidImage
	}

	// Scale first, orient second: the rotation then runs on at most 3000 px, not on the original.
	// img is not used after resize, so the decoded pixels can be collected while the rest runs.
	alpha := hasAlpha(img)
	disp := orient(resize(img, displayEdge), exifOrientation(data, format))
	out := processed{width: disp.Rect.Dx(), height: disp.Rect.Dy()}

	var buf bytes.Buffer
	if alpha {
		out.contentType, out.ext = "image/png", "png"
		err = png.Encode(&buf, disp)
	} else {
		out.contentType, out.ext = "image/jpeg", "jpg"
		err = jpeg.Encode(&buf, disp, &jpeg.Options{Quality: displayQuality})
	}
	if err != nil {
		return processed{}, err
	}
	out.display = buf.Bytes()

	// The thumbnail is always a JPEG; transparency is flattened onto white.
	th := resize(disp, thumbEdge)
	flat := image.NewNRGBA(th.Bounds())
	draw.Draw(flat, flat.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), th, th.Bounds().Min, draw.Over)
	buf = bytes.Buffer{}
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: thumbQuality}); err != nil {
		return processed{}, err
	}
	out.thumb = buf.Bytes()
	return out, nil
}

// sniff returns the content type of the first bytes when it is one we accept.
func sniff(data []byte) (string, bool) {
	ct := http.DetectContentType(data[:min(len(data), 512)])
	return ct, ct == "image/jpeg" || ct == "image/png" || ct == "image/webp"
}

// hasAlpha reports whether img has any non-opaque pixel.
func hasAlpha(img image.Image) bool {
	o, ok := img.(interface{ Opaque() bool })
	return !ok || !o.Opaque()
}

// resize returns img as NRGBA with its long edge at most edge px (never upscaled).
func resize(img image.Image, edge int) *image.NRGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if long := max(w, h); long > edge {
		w, h = max(1, (w*edge+long/2)/long), max(1, (h*edge+long/2)/long)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w == b.Dx() && h == b.Dy() {
		draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
		return dst
	}
	// Scaling in one call makes x/image allocate a float64 buffer of dst width x SOURCE height x 4
	// (480 MB for a 50 MP photo), more than the picture itself. Scale the width in bands of source
	// rows and then the height in bands of columns instead: each pass leaves the other axis at scale 1
	// (an exact identity for CatmullRom), so the result is the same picture and the buffer stays small.
	mid := image.NewNRGBA(image.Rect(0, 0, w, b.Dy()))
	for y := 0; y < b.Dy(); y += bandSize {
		y1 := min(y+bandSize, b.Dy())
		draw.CatmullRom.Scale(mid, image.Rect(0, y, w, y1), img, image.Rect(b.Min.X, b.Min.Y+y, b.Max.X, b.Min.Y+y1), draw.Src, nil)
	}
	for x := 0; x < w; x += bandSize {
		x1 := min(x+bandSize, w)
		draw.CatmullRom.Scale(dst, image.Rect(x, 0, x1, h), mid, image.Rect(x, 0, x1, b.Dy()), draw.Src, nil)
	}
	return dst
}

// withinLimits checks the header's size and the memory the decode would need before any pixel buffer exists.
func withinLimits(data []byte, cfg image.Config, format string) bool {
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSide || cfg.Height > maxSide {
		return false
	}
	px := cfg.Width * cfg.Height
	if px > maxPixels || (format != "jpeg" && px > maxPixelsPNGWebP) {
		return false
	}
	return decodedBytes(data, cfg, format) <= maxDecodedMiB<<20
}

// decodedBytes estimates what image.Decode allocates for a header-valid image. Bytes per pixel come
// from the colour model (8 for 16-bit RGBA, 4 for 8-bit RGBA, 1 for 8-bit gray or palette, ...). For
// JPEG the model is always YCbCr, so the chroma subsampling comes from the SOF segment, and a progressive
// file also keeps its coefficients (4 bytes per sample) until the last scan.
func decodedBytes(data []byte, cfg image.Config, format string) int {
	px := cfg.Width * cfg.Height
	switch cm := cfg.ColorModel; {
	case format == "jpeg":
		num, den, progressive := jpegSamples(data)
		if progressive {
			num *= 5
		}
		return px * num / den
	case cm == color.GrayModel:
		return px
	case cm == color.Gray16Model:
		return px * 2
	case cm == color.YCbCrModel || cm == color.NYCbCrAModel: // lossy WebP: 4:2:0, plus alpha
		return px * 5 / 2
	case cm == color.RGBAModel || cm == color.NRGBAModel || cm == color.CMYKModel:
		return px * 4
	}
	if _, ok := cfg.ColorModel.(color.Palette); ok {
		return px
	}
	return px * 8 // RGBA64, NRGBA64 and anything unknown: assume the worst
}

// jpegSamples returns the samples per pixel as a fraction (1.5 for 4:2:0, 3 for 4:4:4, 1 for gray) and
// whether the file is progressive, read from the SOF segment. Unreadable: the worst case, progressive 4:4:4.
func jpegSamples(d []byte) (num, den int, progressive bool) {
	for i := 2; i+4 <= len(d) && d[i] == 0xFF; {
		m := d[i+1]
		if m == 0xFF {
			i++
			continue
		}
		if m == 0x01 || (m >= 0xD0 && m <= 0xD8) {
			i += 2
			continue
		}
		n := int(binary.BigEndian.Uint16(d[i+2:]))
		if n < 2 || i+2+n > len(d) {
			break
		}
		if seg := d[i+4 : i+2+n]; m >= 0xC0 && m <= 0xCF && m != 0xC4 && m != 0xC8 && m != 0xCC { // a SOF marker
			if len(seg) < 6 || len(seg) < 6+3*int(seg[5]) || seg[5] == 0 {
				break
			}
			hmax, vmax, sum := 1, 1, 0
			for c := range int(seg[5]) {
				hv := seg[6+3*c+1]
				hmax, vmax, sum = max(hmax, int(hv>>4)), max(vmax, int(hv&15)), sum+int(hv>>4)*int(hv&15)
			}
			if sum == 0 {
				break
			}
			return sum, hmax * vmax, m == 0xC2 || m == 0xC6 || m == 0xCA || m == 0xCE
		}
		i += 2 + n
	}
	return 3, 1, true
}

// orient applies EXIF orientation o (1-8) so the pixels are upright.
func orient(src *image.NRGBA, o int) *image.NRGBA {
	if o <= 1 || o > 8 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		for x := range dw {
			var sx, sy int
			switch o {
			case 2:
				sx, sy = w-1-x, y
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sx, sy = x, h-1-y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8:
				sx, sy = w-1-y, x
			}
			si, di := src.PixOffset(sx, sy), dst.PixOffset(x, y)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}

// exifOrientation reads the EXIF Orientation tag of a JPEG or WebP; 1 (upright) when absent or malformed.
func exifOrientation(data []byte, format string) int {
	switch format {
	case "jpeg":
		return jpegOrientation(data)
	case "webp":
		return webpOrientation(data)
	}
	return 1 // ponytail: PNG eXIf chunks are ignored (phones do not write them); add if a camera app does
}

func jpegOrientation(d []byte) int {
	for i := 2; i+4 <= len(d); { // after SOI
		if d[i] != 0xFF {
			return 1
		}
		switch m := d[i+1]; {
		case m == 0xFF:
			i++
		case m == 0x01 || (m >= 0xD0 && m <= 0xD8):
			i += 2 // markers without a length
		case m == 0xDA || m == 0xD9:
			return 1 // pixels start: no EXIF after this
		default:
			n := int(binary.BigEndian.Uint16(d[i+2:]))
			if n < 2 || i+2+n > len(d) {
				return 1
			}
			if seg := d[i+4 : i+2+n]; m == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
				return tiffOrientation(seg[6:])
			}
			i += 2 + n
		}
	}
	return 1
}

func webpOrientation(d []byte) int {
	for i := 12; i+8 <= len(d); { // chunks follow "RIFF<size>WEBP"
		n := int(binary.LittleEndian.Uint32(d[i+4:]))
		end := i + 8 + n
		if n < 0 || end > len(d) {
			return 1
		}
		if string(d[i:i+4]) == "EXIF" {
			return tiffOrientation(bytes.TrimPrefix(d[i+8:end], []byte("Exif\x00\x00")))
		}
		i = end + n%2
	}
	return 1
}

// tiffOrientation finds tag 0x0112 in the first IFD of a TIFF block.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:]))
	if bo.Uint16(t[2:]) != 42 || off < 8 || off+2 > len(t) {
		return 1
	}
	for i, n := 0, int(bo.Uint16(t[off:])); i < n; i++ {
		e := off + 2 + 12*i
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			if v := int(bo.Uint16(t[e+8:])); bo.Uint16(t[e+2:]) == 3 && v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}
