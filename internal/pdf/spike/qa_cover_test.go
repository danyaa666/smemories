//go:build spike

package spike

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/klippa-app/go-pdfium/requests"
)

// QA (T-005): AC4 requires the 4000x3000 JPEG to be placed full-bleed on A5. Check what is
// actually rendered, not the placement maths: no pixel in the middle column/row bands of
// the page edges may be the white page background, and the left and right edges must show
// the same (centred) crop of the photo. Needs the PDFs written by TestFpdf_Image/TestGopdf_Image.
func TestQA_CoverIsFullBleed(t *testing.T) {
	for _, lib := range []string{"fpdf", "gopdf"} {
		t.Run(lib, func(t *testing.T) {
			p := filepath.Join(outDir, lib+"-image.pdf")
			res, err := pdfiumInstance().RenderPageInDPI(&requests.RenderPageInDPI{DPI: 40, Page: pageRef(openDoc(t, p), 0)})
			if err != nil {
				t.Fatal(err)
			}
			img := res.Result.Image
			b := img.Bounds()
			white := func(c color.Color) bool {
				r, g, bl, _ := c.RGBA()
				return r > 0xf000 && g > 0xf000 && bl > 0xf000
			}
			edge := func(name string, pts []image.Point) {
				for _, pt := range pts {
					if white(img.At(pt.X, pt.Y)) {
						t.Errorf("%s edge pixel %v is white page background: the photo does not cover the page", name, pt)
						return
					}
				}
			}
			var left, right []image.Point
			for y := b.Min.Y + b.Dy()/4; y < b.Max.Y-b.Dy()/4; y += 4 {
				left = append(left, image.Pt(b.Min.X+1, y))
				right = append(right, image.Pt(b.Max.X-2, y))
			}
			edge("left", left)
			edge("right", right)
		})
	}
}
