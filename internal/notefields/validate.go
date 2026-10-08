package notefields

import (
	"errors"
	"fmt"
	"sort"

	"github.com/danyaa666/smemories/internal/textx"
)

// Stable error codes, safe to expose in API responses.
const (
	CodeUnknownField  = "unknown_field"
	CodeMissingAnswer = "missing_answer"
	CodeInvalidAnswer = "invalid_answer"
)

var (
	ErrUnknownField = errors.New("unknown field")
	ErrMissing      = errors.New("missing answer")
	ErrInvalid      = errors.New("invalid answer")
)

// FieldError names the field that failed and why. errors.Is matches ErrUnknownField, ErrMissing
// and ErrInvalid by Code.
type FieldError struct{ ID, Code string }

func (e *FieldError) Error() string { return e.Code + ": " + e.ID }

func (e *FieldError) Unwrap() error {
	switch e.Code {
	case CodeUnknownField:
		return ErrUnknownField
	case CodeMissingAnswer:
		return ErrMissing
	}
	return ErrInvalid
}

// resolve maps refs to catalogue fields. A ref outside the catalogue, or listed twice, is a template
// bug, not a user error.
func resolve(refs []FieldRef) ([]Field, error) {
	out := make([]Field, 0, len(refs))
	seen := make(map[string]bool, len(refs))
	for _, r := range refs {
		f, ok := byID[r.ID]
		if !ok {
			return nil, &FieldError{r.ID, CodeUnknownField}
		}
		if seen[r.ID] {
			return nil, fmt.Errorf("notefields: field %q listed twice", r.ID)
		}
		seen[r.ID] = true
		out = append(out, f)
	}
	return out, nil
}

// Validate checks answers (field id to raw text) against refs and returns the cleaned answers without
// empty optional ones. The returned error is a *FieldError for the first problem found: unknown
// answer ids first (in id order), then refs in order.
func Validate(refs []FieldRef, answers map[string]string) (map[string]string, error) {
	fs, err := resolve(refs)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(refs))
	for _, r := range refs {
		allowed[r.ID] = true
	}
	var extra []string
	for id := range answers {
		if !allowed[id] {
			extra = append(extra, id)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return nil, &FieldError{extra[0], CodeUnknownField}
	}
	clean := make(map[string]string, len(refs))
	for i, f := range fs {
		raw, ok := answers[f.ID]
		if !ok {
			if refs[i].Required {
				return nil, &FieldError{f.ID, CodeMissingAnswer}
			}
			continue
		}
		v, ok := cleanValue(f, raw)
		if !ok {
			return nil, &FieldError{f.ID, CodeInvalidAnswer}
		}
		if v == "" {
			if refs[i].Required {
				return nil, &FieldError{f.ID, CodeMissingAnswer}
			}
			continue
		}
		clean[f.ID] = v
	}
	return clean, nil
}

func cleanValue(f Field, raw string) (string, bool) {
	// Bounds the work: no valid value is longer than 4 bytes per character.
	if len(raw) > 4*f.MaxLength {
		return "", false
	}
	if f.Kind == LongText {
		return textx.CleanMultiline(raw, 0, f.MaxLength)
	}
	return textx.Clean(raw, 0, f.MaxLength)
}
