package assets

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

var idRE = regexp.MustCompile(`^(sticker|shape|frame)\.[a-z0-9]+(-[a-z0-9]+)*$`)

func TestRegistry(t *testing.T) {
	all := All()
	if len(all) < 48 {
		t.Fatalf("registry has %d assets, want at least 48", len(all))
	}
	seen := map[string]bool{}
	var authored, curated int
	for _, a := range all {
		if !idRE.MatchString(a.ID) || !strings.HasPrefix(a.ID, a.Kind+".") {
			t.Errorf("%q: id must be kind.name with kind %q", a.ID, a.Kind)
		}
		if seen[a.ID] {
			t.Errorf("%q: duplicate id", a.ID)
		}
		seen[a.ID] = true
		if a.LabelEN == "" || a.LabelVI == "" || len(a.Tags) == 0 || a.Width <= 0 || a.Height <= 0 {
			t.Errorf("%q: labels, tags and size are required", a.ID)
		}
		switch a.Licence {
		case "authored":
			authored++
		case "CC0", "OFL":
			curated++
		default:
			t.Errorf("%q: licence %q is not CC0, OFL or authored", a.ID, a.Licence)
		}
	}
	if authored < 24 || curated < 24 {
		t.Errorf("authored %d, curated %d: want at least 24 of each", authored, curated)
	}
}

func TestGetAndHas(t *testing.T) {
	first := All()[0]
	if a, ok := Get(first.ID); !ok || a.ID != first.ID {
		t.Fatalf("Get(%q) = %v, %v", first.ID, a, ok)
	}
	for _, id := range []string{"", "sticker.", "sticker.nope", "SHAPE.CIRCLE", "<script>"} {
		if Has(id) {
			t.Errorf("Has(%q) = true, want false", id)
		}
	}
}

// The web app reads web/src/book/assets/registry.json, the server reads its copy; they must be the same bytes.
func TestRegistryMatchesWeb(t *testing.T) {
	web, err := os.ReadFile("../../web/src/book/assets/registry.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(web, raw) {
		t.Fatal("internal/assets/registry.json differs from web/src/book/assets/registry.json: run scripts/sync-assets.sh")
	}
}
