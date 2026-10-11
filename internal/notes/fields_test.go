package notes

import (
	"slices"
	"testing"

	"github.com/danyaa666/smemories/internal/notefields"
)

func TestFieldsFor(t *testing.T) {
	// A template's own set, the default for a book without a template, and the default for an unknown id.
	for _, id := range []string{"classic", "modern", "", "removed-template"} {
		if got := FieldsFor(id); !slices.Equal(got, notefields.Default()) {
			t.Errorf("FieldsFor(%q) = %v, want the default set", id, got)
		}
	}
}
