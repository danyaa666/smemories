package fonts

import (
	"slices"
	"strings"
	"testing"
)

// Every registered family must draw all Vietnamese letters with every tone mark, upper and lower case, in
// each of its faces. Register enforces it too; this test is the safety net for the whole registry.
func TestRegistryCoversVietnamese(t *testing.T) {
	if !slices.Contains(Names(), "BeVietnamPro") {
		t.Fatalf("BeVietnamPro is not registered: %v", Names())
	}
	for _, name := range Names() {
		f, _ := Lookup(name)
		for kind, face := range map[string][]byte{"regular": f.Regular, "bold": f.Bold} {
			if face == nil {
				continue
			}
			if err := CheckVietnamese(face); err != nil {
				t.Errorf("%s %s: %v", name, kind, err)
			}
		}
	}
}

func TestCheckVietnameseRejects(t *testing.T) {
	// The emoji font has no Latin letters at all; garbage is not a font.
	if err := CheckVietnamese(Emoji); err == nil || !strings.Contains(err.Error(), "no glyph") {
		t.Errorf("emoji font: %v", err)
	}
	if err := CheckVietnamese([]byte("garbage")); err == nil || !strings.Contains(err.Error(), "TTF") {
		t.Errorf("garbage: %v", err)
	}
}

func TestRegisterRefusesUnusableFamilies(t *testing.T) {
	before := len(Names())
	for name, f := range map[string]Family{
		"":             {Regular: Regular},
		"NoFaces":      {},
		"NoVietnamese": {Regular: Emoji},
		"BadBold":      {Regular: Regular, Bold: []byte("x")},
		"BeVietnamPro": {Regular: Regular}, // twice
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Register(%q) did not panic", name)
				}
			}()
			Register(name, f)
		}()
	}
	if len(Names()) != before {
		t.Errorf("a refused family was registered: %v", Names())
	}
}

func TestFace(t *testing.T) {
	both := Family{Regular: []byte("r"), Bold: []byte("b")}
	onlyRegular, onlyBold := Family{Regular: []byte("r")}, Family{Bold: []byte("b")}
	for _, tc := range []struct {
		name     string
		f        Family
		bold     bool
		want     string
		wantBold bool
	}{
		{"both regular", both, false, "r", false},
		{"both bold", both, true, "b", true},
		{"regular only, bold asked", onlyRegular, true, "r", false},
		{"bold only, regular asked", onlyBold, false, "b", true},
	} {
		got, isBold := tc.f.Face(tc.bold)
		if string(got) != tc.want || isBold != tc.wantBold {
			t.Errorf("%s: %q %v", tc.name, got, isBold)
		}
	}
}
