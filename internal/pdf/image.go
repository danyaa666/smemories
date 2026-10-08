package pdf

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg" // registers the JPEG decoder for DecodeConfig
	_ "image/png"  // registers the PNG decoder for DecodeConfig
	"math"

	"codeberg.org/go-pdf/fpdf"

	"github.com/danyaa666/smemories/internal/templates"
)

const (
	minDPI       = 300
	maxPixels    = 64_000_000 // defensive: the upload pipeline (T-009) already bounds these
	maxPNGPixels = 25_000_000 // fpdf decodes PNGs fully (4 bytes per pixel in memory); JPEGs are copied as is
	maxImageSide = 20_000
)

// ImageSource fetches photo bytes (JPEG or PNG) by media id.
type ImageSource interface {
	Image(ctx context.Context, id string) ([]byte, error)
}

// imgInfo is what is remembered about a media id; the bytes live in the fpdf document only.
type imgInfo struct {
	w, h  int
	ptype string // JPG or PNG
	ok    bool
}

// coverRect returns where to place a pxW x pxH image so that it covers the frame (fx, fy, fw, fh)
// completely, centred (the overflow is clipped away), and the effective resolution in DPI. Placement
// usually starts at a negative x or y: it must be drawn with AllowNegativePosition (see coverOpts).
func coverRect(pxW, pxH int, fx, fy, fw, fh float64) (x, y, w, h, dpi float64) {
	s := math.Max(fw/float64(pxW), fh/float64(pxH)) // mm per pixel
	w, h = float64(pxW)*s, float64(pxH)*s
	return fx + (fw-w)/2, fy + (fh-h)/2, w, h, 25.4 / s
}

// load fetches, checks and registers a photo once per id. A context error is returned; every other
// failure is remembered as "not ok" so it renders as a placeholder.
func (r *renderer) load(ctx context.Context, id string) (*imgInfo, error) {
	if info, ok := r.imgs[id]; ok {
		return info, nil
	}
	info := &imgInfo{}
	r.imgs[id] = info
	if r.src == nil {
		return info, nil
	}
	data, err := r.src.Image(ctx, id)
	if err != nil {
		return info, ctx.Err()
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImageSide || cfg.Height > maxImageSide ||
		cfg.Width*cfg.Height > maxPixels {
		return info, nil
	}
	switch format {
	case "jpeg":
		info.ptype = "JPG"
	case "png":
		if cfg.Width*cfg.Height > maxPNGPixels {
			return info, nil
		}
		info.ptype = "PNG"
	default:
		return info, nil
	}
	// fpdf copies JPEG data through without re-encoding; a parse failure sets its sticky error,
	// which we clear because a bad photo must only cost a placeholder.
	r.pdf.RegisterImageOptionsReader(id, fpdf.ImageOptions{ImageType: info.ptype}, bytes.NewReader(data))
	if r.pdf.Err() {
		r.pdf.ClearError()
		return info, nil
	}
	info.w, info.h, info.ok = cfg.Width, cfg.Height, true
	return info, nil
}

// clip starts a clip for the shape of e inside the frame and returns the matching end function.
func (r *renderer) clip(e templates.Element, x, y, w, h float64) func() {
	switch e.Shape {
	case "rounded":
		r.pdf.ClipRoundedRect(x, y, w, h, e.Radius*r.ss, false)
	case "circle":
		r.pdf.ClipCircle(x+w/2, y+h/2, w/2, false)
	case "ellipse":
		r.pdf.ClipEllipse(x+w/2, y+h/2, w/2, h/2, false)
	default:
		r.pdf.ClipRect(x, y, w, h, false)
	}
	return r.pdf.ClipEnd
}

// drawImage places the photo with the given id in the element's frame, cover-fitted and centred, or a
// neutral placeholder (with a missing_image warning) when it cannot be shown.
func (r *renderer) drawImage(ctx context.Context, e templates.Element, ox, oy float64, id string, d *slotData) error {
	if id == "" {
		return nil // optional photo not given
	}
	info, err := r.load(ctx, id)
	if err != nil {
		return err
	}
	x, y, w, h := (ox+e.X)*r.sx, (oy+e.Y)*r.sy, e.W*r.sx, e.H*r.sy
	defer r.rotated(e, x, y, w, h)() // deferred first, so it ends after the clip does
	end := r.clip(e, x, y, w, h)
	defer end()
	if !info.ok {
		r.pdf.SetFillColor(224, 224, 224)
		r.pdf.Rect(x, y, w, h, "F")
		r.warn(Warning{Code: WarnMissingImage, Slot: e.Slot, NoteID: d.noteID, MediaID: id})
		return nil
	}
	ix, iy, iw, ih, dpi := coverRect(info.w, info.h, x, y, w, h)
	if dpi < minDPI-0.5 && !r.lowRes[id] {
		r.lowRes[id] = true
		r.warn(Warning{Code: WarnLowResolution, Slot: e.Slot, NoteID: d.noteID, MediaID: id, DPI: math.Round(dpi)})
	}
	// AllowNegativePosition is mandatory: a cover crop starts at a negative x or y and fpdf otherwise
	// silently draws the image at the margin (ADR 0002, pinned by TestCoverImageIsPlacedAtNegativeOffset).
	r.pdf.ImageOptions(id, ix, iy, iw, ih, false, fpdf.ImageOptions{ImageType: info.ptype, AllowNegativePosition: true}, 0, "")
	return nil
}

// drawBackground places a page background over the whole page. An asset is registered with fpdf once and
// reused by every page that names it; the templates package has already checked its type, size and ratio.
func (r *renderer) drawBackground(asset string, pw, ph float64) error {
	key := "bg:" + asset
	ptype, ok := r.bgs[asset]
	if !ok {
		data, err := r.tmpl.Asset(asset)
		if err != nil {
			return fmt.Errorf("render pdf: background: %w", err)
		}
		_, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("render pdf: background %q: %w", asset, err)
		}
		ptype = map[string]string{"jpeg": "JPG", "png": "PNG"}[format]
		r.pdf.RegisterImageOptionsReader(key, fpdf.ImageOptions{ImageType: ptype}, bytes.NewReader(data))
		if err := r.pdf.Error(); err != nil {
			return fmt.Errorf("render pdf: background %q: %w", asset, err)
		}
		r.bgs[asset] = ptype
	}
	r.pdf.ImageOptions(key, 0, 0, pw, ph, false, fpdf.ImageOptions{ImageType: ptype}, 0, "")
	return nil
}
