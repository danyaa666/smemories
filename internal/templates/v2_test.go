package templates

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/danyaa666/smemories/internal/pdf/fonts"
)

// A regular-only second family for the validator tests (the bold face of Be Vietnam Pro under another name).
func init() { fonts.Register("TestFixture", fonts.Family{Regular: fonts.Bold}) }

func flatImage(w, h int) *image.Gray { // grey: the cheapest to encode, and the header is all the validator reads
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xEE
	}
	return img
}

func pngBytes(t testing.TB, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, flatImage(w, h)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegBytes(t testing.TB, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, flatImage(w, h), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// bgSpec returns the classic template with a background element added to the cover page.
func bgSpec(t *testing.T, asset string, more ...func(root map[string]any)) []byte {
	return mutate(t, func(r map[string]any) {
		p := r["pages"].([]any)[0].(map[string]any)
		p["elements"] = append([]any{map[string]any{"type": "background", "asset": asset}}, p["elements"].([]any)...)
		for _, f := range more {
			f(r)
		}
	})
}

// good is a valid background for the A5 reference page: 1240 x 1760 px is about 213 DPI, ratio 0.7045.
func good(t testing.TB) []byte { return pngBytes(t, 1240, 1760) }

func TestBackgroundAccepted(t *testing.T) {
	fsys := fstest.MapFS{
		"classic/bg.png":    {Data: good(t)},
		"classic/bg.jpg":    {Data: jpegBytes(t, 1240, 1760)},
		"classic/real.png":  {Data: jpegBytes(t, 1240, 1760)},   // a JPEG named .png: accepted, the content decides
		"classic/edge.png":  {Data: pngBytes(t, 874, 1240)},     // 150 DPI: the lowest allowed
		"classic/big.png":   {Data: pngBytes(t, 2331, 3307)},    // 400 DPI: the highest allowed
		"classic/ratio.png": {Data: pngBytes(t, 1240, 1760+17)}, // 0.7 % off the A5 ratio, within 1 %
	}
	for name := range fsys {
		file := strings.TrimPrefix(name, "classic/")
		t.Run(file, func(t *testing.T) {
			tt, err := ParseFS(bgSpec(t, name), fsys)
			if err != nil {
				t.Fatal(err)
			}
			data, err := tt.Asset(name)
			if err != nil || !bytes.Equal(data, fsys[name].Data) {
				t.Errorf("Asset(%q) = %d bytes, %v", name, len(data), err)
			}
		})
	}
}

func TestBackgroundRejected(t *testing.T) {
	big := append(good(t), make([]byte, maxAssetBytes)...) // trailing bytes: still a PNG, but over 1.5 MiB
	fsys := fstest.MapFS{
		"classic/ok.png":    {Data: good(t)},
		"classic/wide.png":  {Data: pngBytes(t, 1760, 1240)},
		"classic/low.png":   {Data: pngBytes(t, 870, 1234)}, // 149.3 DPI
		"classic/high.png":  {Data: pngBytes(t, 2340, 3320)},
		"classic/big.png":   {Data: big},
		"classic/text.png":  {Data: []byte("not an image")},
		"classic/empty.png": {Data: nil},
		"classic/a.gif":     {Data: good(t)},
		"classic/sub/x.png": {Data: good(t)},
		"other/ok.png":      {Data: good(t)},
		"classic":           {Data: good(t)},
	}
	tests := []struct {
		name, asset string
		want        []string
	}{
		{"missing file", "classic/none.png", []string{"does not exist"}},
		{"ratio too far off", "classic/wide.png", []string{"aspect ratio"}},
		{"below 150 DPI", "classic/low.png", []string{"150..400"}},
		{"above 400 DPI", "classic/high.png", []string{"150..400"}},
		{"over 1.5 MiB", "classic/big.png", []string{"1572864"}},
		{"not an image", "classic/text.png", []string{"not a PNG or JPEG"}},
		{"empty file", "classic/empty.png", []string{"not a PNG or JPEG"}},
		{"extension", "classic/a.gif", []string{"ending in .png, .jpg or .jpeg"}},
		{"parent folder", "classic/../other/ok.png", []string{"no folders", `".."`}},
		{"dots in the name", "classic/a..png", []string{"no folders"}},
		{"another template's folder", "other/ok.png", []string{`"classic/"`}},
		{"sub folder", "classic/sub/x.png", []string{"plain file name"}},
		{"absolute path", "/classic/ok.png", []string{`"classic/"`}},
		{"backslash", `classic\ok.png`, []string{`"classic/"`}},
		{"no file name", "classic/", []string{"plain file name"}},
		{"empty", "", []string{"plain file name"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseFS(bgSpec(t, tc.asset), fsys)
			if err == nil {
				t.Fatal("expected an error")
			}
			// Every error names the template, the page and the file.
			want := append([]string{`template "classic"`, "page 1 (cover)", "element 1 (background"}, tc.want...)
			if tc.asset != "" {
				want = append(want, fmt.Sprintf("%q", tc.asset))
			}
			for _, w := range want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err, w)
				}
			}
		})
	}
}

func TestBackgroundRules(t *testing.T) {
	fsys := fstest.MapFS{"classic/ok.png": {Data: good(t)}}
	tests := []struct {
		name string
		fn   func(root map[string]any)
		want string
	}{
		{"two on one page", func(r map[string]any) {
			p := r["pages"].([]any)[0].(map[string]any)
			p["elements"] = append(p["elements"].([]any), map[string]any{"type": "background", "asset": "classic/ok.png"})
		}, "more than one background"},
		{"in a notes flow", func(r map[string]any) {
			f := r["pages"].([]any)[2].(map[string]any)["flow"].(map[string]any)
			f["elements"] = append(f["elements"].([]any), map[string]any{"type": "background", "asset": "classic/ok.png"})
		}, "only allowed on a page"},
		{"with a box", func(r map[string]any) { el(r, 0, 0)["w"] = 100 }, "takes only type and asset"},
		{"with a slot", func(r map[string]any) { el(r, 0, 0)["slot"] = "title" }, "takes only type and asset"},
		{"rotated", func(r map[string]any) { el(r, 0, 0)["rotate"] = 5 }, "takes only type and asset"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseFS(bgSpec(t, "classic/ok.png", tc.fn), fsys)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "page ") {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
	// The same file may back several pages.
	two := bgSpec(t, "classic/ok.png", func(r map[string]any) {
		p := r["pages"].([]any)[3].(map[string]any)
		p["elements"] = append(p["elements"].([]any), map[string]any{"type": "background", "asset": "classic/ok.png"})
	})
	if _, err := ParseFS(two, fsys); err != nil {
		t.Errorf("one asset on two pages: %v", err)
	}
	// Without an asset folder (Validate on a hand-built template) a background cannot pass.
	tt, err := ParseFS(bgSpec(t, "classic/ok.png"), fsys)
	if err != nil {
		t.Fatal(err)
	}
	tt.assets = nil
	if err := Validate(tt); err == nil || !strings.Contains(err.Error(), "no asset folder") {
		t.Errorf("err = %v", err)
	}
	if _, err := tt.Asset("classic/ok.png"); err == nil {
		t.Error("Asset on a template without assets succeeded")
	}
}

func TestBackgroundTotalLimit(t *testing.T) {
	// Four pages of 1.5 MiB cannot reach 8 MiB today; the rule guards a future page limit.
	v := &validator{t: &Template{ID: "x"}, assetSize: map[string]int64{"x/a.png": maxAssetsTotal / 2, "x/b.png": maxAssetsTotal/2 + 1}}
	v.assetTotal()
	if len(v.errs) != 1 || !strings.Contains(v.errs[0].Error(), `template "x"`) {
		t.Errorf("errs = %v", v.errs)
	}
	v = &validator{t: &Template{ID: "x"}, assetSize: map[string]int64{"x/a.png": maxAssetsTotal}}
	if v.assetTotal(); len(v.errs) != 0 {
		t.Errorf("exactly 8 MiB must pass: %v", v.errs)
	}
}

// Every built-in template: every background decodes completely (the load-time check reads only the header).
func TestEveryTemplateBackgroundDecodes(t *testing.T) {
	for _, info := range List() {
		tt, _ := Get(info.ID)
		for _, p := range tt.Pages {
			for _, e := range p.Elements {
				if e.Type != TypeBackground {
					continue
				}
				data, err := tt.Asset(e.Asset)
				if err != nil {
					t.Errorf("%s: %v", info.ID, err)
					continue
				}
				if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
					t.Errorf("template %q, page %s, asset %q: %v", info.ID, p.Kind, e.Asset, err)
				}
			}
		}
	}
}

func staticText(en, vi any) map[string]any {
	return map[string]any{"type": "text", "text": map[string]any{"en": en, "vi": vi}, "x": 10, "y": 100, "w": 100, "h": 12, "size": 12}
}

func withText(e map[string]any, more ...func(map[string]any)) func(root map[string]any) {
	return func(r map[string]any) {
		for _, f := range more {
			f(e)
		}
		p := r["pages"].([]any)[0].(map[string]any)
		p["elements"] = append(p["elements"].([]any), e)
	}
}

func TestStaticText(t *testing.T) {
	tests := []struct {
		name string
		fn   func(root map[string]any)
		want string // empty: valid
	}{
		{"both languages", withText(staticText("All About Me", "Về mình")), ""},
		{"500 characters", withText(staticText(strings.Repeat("a", 500), strings.Repeat("ê", 500))), ""},
		{"with a display font", withText(staticText("A", "B"), func(e map[string]any) { e["font"] = "display" }), ""},
		{"missing vi", withText(staticText("A", "")), `missing the "vi" language`},
		{"blank en", withText(staticText("  ", "B")), `missing the "en" language`},
		{"501 characters", withText(staticText(strings.Repeat("a", 501), "B")), "longer than 500"},
		{"501 vi characters", withText(staticText("A", strings.Repeat("ê", 501))), "longer than 500"},
		{"newline", withText(staticText("A\nB", "B")), "control character"},
		{"tab", withText(staticText("A", "B\tC")), "control character"},
		{"nul", withText(staticText("A\x00", "B")), "control character"},
		{"third language", withText(staticText("A", "B"), func(e map[string]any) { e["text"].(map[string]any)["fr"] = "C" }), `unknown language "fr"`},
		{"slot and text", withText(staticText("A", "B"), func(e map[string]any) { e["slot"] = "title" }), "either a slot or a text object"},
		{"neither", withText(map[string]any{"type": "text", "x": 10, "y": 100, "w": 100, "h": 12, "size": 12}), "needs a slot or a text object"},
		{"label on static text", withText(staticText("A", "B"), func(e map[string]any) { e["label"] = map[string]any{"en": "x"} }), "label only applies"},
		{"too short a box", withText(staticText("A", "B"), func(e map[string]any) { e["h"] = 2 }), "too short for one line"},
		{"unknown font role", withText(staticText("A", "B"), func(e map[string]any) { e["font"] = "Nunito" }), `unknown font role "Nunito"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(mutate(t, tc.fn))
			switch {
			case tc.want == "" && err != nil:
				t.Fatal(err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "page 1 (cover)")):
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRotateAndEllipse(t *testing.T) {
	setRect := func(deg any) func(map[string]any) {
		return func(r map[string]any) {
			p := r["pages"].([]any)[0].(map[string]any)
			p["elements"] = append(p["elements"].([]any), map[string]any{"type": "rect", "x": 10, "y": 100, "w": 20, "h": 10, "fill": "accent", "rotate": deg})
		}
	}
	for _, deg := range []float64{-45, -0.5, 0, 12.5, 45} {
		if _, err := Parse(mutate(t, setRect(deg))); err != nil {
			t.Errorf("rotate %g: %v", deg, err)
		}
	}
	for _, deg := range []float64{45.01, -46, 90} {
		_, err := Parse(mutate(t, setRect(deg)))
		if err == nil || !strings.Contains(err.Error(), "rotate") || !strings.Contains(err.Error(), "-45..45") {
			t.Errorf("rotate %g: err = %v", deg, err)
		}
	}
	// A rotated element must still lie inside the page before rotation.
	_, err := Parse(mutate(t, func(r map[string]any) { el(r, 0, 2)["x"] = 100; el(r, 0, 2)["rotate"] = 5 }))
	if err == nil || !strings.Contains(err.Error(), "outside the 148 x 210 mm area") {
		t.Errorf("err = %v", err)
	}
	ellipse := func(w, h, radius any) func(map[string]any) {
		return func(r map[string]any) {
			e := el(r, 1, 0)
			e["shape"], e["w"], e["h"] = "ellipse", w, h
			delete(e, "radius")
			if radius != nil {
				e["radius"] = radius
			}
		}
	}
	for _, wh := range [][2]float64{{56, 56}, {56, 40}} {
		if _, err := Parse(mutate(t, ellipse(wh[0], wh[1], nil))); err != nil {
			t.Errorf("ellipse %v: %v", wh, err)
		}
	}
	if _, err := Parse(mutate(t, ellipse(56, 40, 3))); err == nil || !strings.Contains(err.Error(), "radius is not allowed for an ellipse") {
		t.Errorf("err = %v", err)
	}
}

func TestFontFamilies(t *testing.T) {
	theme := func(r map[string]any) map[string]any { return r["theme"].(map[string]any) }
	tests := []struct {
		name string
		fn   func(root map[string]any)
		want string
	}{
		{"fonts only", func(r map[string]any) {
			delete(theme(r), "font")
			theme(r)["fonts"] = map[string]any{"body": "BeVietnamPro", "display": "TestFixture"}
		}, ""},
		{"font and the same body", func(r map[string]any) { theme(r)["fonts"] = map[string]any{"body": "BeVietnamPro"} }, ""},
		{"display only with font", func(r map[string]any) { theme(r)["fonts"] = map[string]any{"display": "TestFixture"} }, ""},
		{"unknown family", func(r map[string]any) { theme(r)["fonts"] = map[string]any{"display": "Fredoka"} },
			`fonts.display: missing font "Fredoka"`},
		{"unknown role", func(r map[string]any) { theme(r)["fonts"] = map[string]any{"title": "BeVietnamPro"} }, `unknown font role "title"`},
		{"font and body differ", func(r map[string]any) { theme(r)["fonts"] = map[string]any{"body": "TestFixture"} }, "differ"},
		{"no body at all", func(r map[string]any) {
			delete(theme(r), "font")
			theme(r)["fonts"] = map[string]any{"display": "TestFixture"}
		}, "missing font"},
		{"unknown font", func(r map[string]any) { theme(r)["font"] = "Comic Sans" }, `font: missing font "Comic Sans"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(mutate(t, tc.fn))
			switch {
			case tc.want == "" && err != nil:
				t.Fatal(err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), `template "classic"`)):
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
	tt, err := Parse(mutate(t, func(r map[string]any) { theme(r)["fonts"] = map[string]any{"display": "TestFixture"} }))
	if err != nil {
		t.Fatal(err)
	}
	if tt.Family("") != "BeVietnamPro" || tt.Family(RoleBody) != "BeVietnamPro" || tt.Family(RoleDisplay) != "TestFixture" {
		t.Errorf("families: %q %q %q", tt.Family(""), tt.Family(RoleBody), tt.Family(RoleDisplay))
	}
	c, _ := Get("classic")
	if c.Family(RoleDisplay) != "BeVietnamPro" {
		t.Errorf("display falls back to body, got %q", c.Family(RoleDisplay))
	}
}

// T-037 review note: a lower-case reference must produce only the "invalid reference" error, not also a
// misleading aspect-ratio error about the page sizes.
func TestInvalidReferenceReportsOnce(t *testing.T) {
	_, err := Parse(mutate(t, func(r map[string]any) { r["reference"] = "letter"; r["page_sizes"] = []any{"A5", "A4"} }))
	if err == nil || !strings.Contains(err.Error(), `invalid reference "letter"`) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "aspect ratio") || strings.Count(err.Error(), "\n") != 0 {
		t.Errorf("more than the one error: %v", err)
	}
	// Unknown sizes are still reported next to a bad reference.
	_, err = Parse(mutate(t, func(r map[string]any) { r["reference"] = "letter"; r["page_sizes"] = []any{"A3"} }))
	if err == nil || !strings.Contains(err.Error(), `unknown page size "A3"`) {
		t.Errorf("err = %v", err)
	}
}

func TestPageDims(t *testing.T) {
	for size, want := range map[string][2]float64{"A5": {148, 210}, "A4": {210, 297}, "Letter": {215.9, 279.4}} {
		if got, ok := PageDims(size); !ok || got != want {
			t.Errorf("PageDims(%q) = %v %v", size, got, ok)
		}
	}
	if _, ok := PageDims("letter"); ok {
		t.Error("PageDims accepted a lower-case size")
	}
	// The returned array is a copy: callers cannot change the table.
	d, _ := PageDims("A5")
	d[0] = 1
	if d2, _ := PageDims("A5"); d2[0] != 148 {
		t.Error("PageDims exposes the table")
	}
}
