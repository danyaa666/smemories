package notes

import (
	"github.com/danyaa666/smemories/internal/notefields"
	"github.com/danyaa666/smemories/internal/templates"
)

// FieldsFor returns the fields the note form of a book asks, in order: those of its template (Go or html),
// or the default set when the book has no template yet or names one that no longer exists. It is the single
// place the form is decided. A note is checked against the set in force when it is submitted.
func FieldsFor(templateID string) []notefields.FieldRef {
	if refs, ok := templates.NoteFields(templateID); ok {
		return refs
	}
	return notefields.Default()
}
