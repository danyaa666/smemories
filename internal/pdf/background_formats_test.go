package pdf

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"

	"github.com/danyaa666/smemories/internal/templates"
)

// Backgrounds in the PNG flavours a designer's export tool may produce must validate AND render, because the
// validator reads only the header: a flavour fpdf cannot embed would otherwise fail on the request path.
func TestBackgroundPNGFlavoursRender(t *testing.T) {
	const w, h = 1240, 1760
	enc := func(img image.Image) []byte {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	pal := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{color.White, color.Black, color.RGBA{200, 0, 0, 255}})
	flavours := map[string][]byte{
		"rgb":     enc(image.NewRGBA(image.Rect(0, 0, w, h))),
		"nrgba":   enc(image.NewNRGBA(image.Rect(0, 0, w, h))), // alpha channel
		"gray":    enc(image.NewGray(image.Rect(0, 0, w, h))),
		"gray16":  enc(image.NewGray16(image.Rect(0, 0, w, h))),
		"rgba64":  enc(image.NewRGBA64(image.Rect(0, 0, w, h))),
		"palette": enc(pal),
	}
	for name, data := range flavours {
		t.Run(name, func(t *testing.T) {
			bg := fmt.Sprintf(`{"type":"background","asset":"qa/%s.png"}`, name)
			spec := fmt.Sprintf(`{"id":"qa","name":{"en":"Q","vi":"Q"},"unit":"mm","page_sizes":["A5"],
 "theme":{"font":"BeVietnamPro","colors":{"ink":"#000000","accent":"#aa0000","paper":"#ffffff"}},
 "pages":[{"kind":"cover","elements":[%[1]s]},{"kind":"profile","elements":[%[1]s]},
  {"kind":"notes","elements":[%[1]s],"flow":{"x":10,"y":10,"w":100,"h":100,"gap":4,"item_h":50,"elements":[
   {"type":"text","slot":"note_message","x":0,"y":0,"w":50,"h":10,"size":10}]}},
  {"kind":"back","elements":[%[1]s]}]}`, bg)
			tt, err := templates.ParseFS([]byte(spec), fstest.MapFS{"qa/" + name + ".png": {Data: data}})
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			rep, err := Render(context.Background(), tt, Book{Title: "x"}, nil, &out, Options{Now: fixedNow})
			if err != nil || len(rep.Warnings) != 0 || rep.Pages != 4 {
				t.Fatalf("pages %d, warnings %s, err %v", rep.Pages, codes(rep), err)
			}
		})
	}
}
