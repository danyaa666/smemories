package notes

import (
	"context"

	"github.com/danyaa666/smemories/internal/notefields"
)

// FieldsFor returns the fields the note form of a yearbook asks, in order. It is the single place the form
// is decided: every yearbook gets the default set today; T-044 makes it return the fields of the book's
// template. A note is checked against the set in force when it is submitted.
func FieldsFor(_ context.Context, _ uint64) ([]notefields.FieldRef, error) {
	return notefields.Default(), nil
}
