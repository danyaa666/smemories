// Package apperr is the project's typed error: an error carries a Code (a stable ERROR_*
// name plus the HTTP status it maps to) so one mapper in internal/httpx can answer for it.
// The package does not import net/http; the status is a plain int.
package apperr

import (
	"errors"
	"strings"
)

// Code is an error category: Name is the contract string sent to clients, HTTP the status.
// It is a small immutable value, so it is passed by value on purpose.
type Code struct {
	Name string
	HTTP int
}

// NewCode defines a business code, e.g. NewCode("ERROR_INVALID_TITLE", 400).
func NewCode(name string, http int) Code { return Code{Name: name, HTTP: http} }

// The codes every endpoint can return (see docs/api-contract.md section 3).
var (
	Internal     = NewCode("ERROR_INTERNAL", 500)
	Unauthorized = NewCode("ERROR_UNAUTHORIZED", 401)
	Forbidden    = NewCode("ERROR_FORBIDDEN", 403)
	Param        = NewCode("ERROR_PARAM", 400)
	NotFound     = NewCode("ERROR_NOT_FOUND", 404)
	Conflict     = NewCode("ERROR_CONFLICT", 409)
	RateLimited  = NewCode("ERROR_RATE_LIMITED", 429)
	TooLarge     = NewCode("ERROR_TOO_LARGE", 413)
	Unavailable  = NewCode("ERROR_UNAVAILABLE", 503)
	BadGateway   = NewCode("ERROR_BAD_GATEWAY", 502)
)

// Error is an error with a Code. msg is the English text for the client (4xx) or context for
// the log (5xx); err is the wrapped cause, kept for errors.Is / errors.As.
type Error struct {
	code Code
	msg  string
	err  error
}

// Error returns "msg: cause", or whichever of the two exists; the cause appears once.
func (e *Error) Error() string {
	parts := make([]string, 0, 2)
	if e.msg != "" {
		parts = append(parts, e.msg)
	}
	if e.err != nil {
		parts = append(parts, e.err.Error())
	}
	if len(parts) == 0 {
		return e.code.Name
	}
	return strings.Join(parts, ": ")
}

// Unwrap exposes the cause to errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.err }

// New returns an error with the given code and message.
func New(code Code, msg string) error { return &Error{code: code, msg: msg} }

// Wrap adds context msg and a code to err.
func Wrap(code Code, err error, msg string) error { return &Error{code: code, msg: msg, err: err} }

// WithCode sets the code on err without adding context; nil stays nil.
func WithCode(code Code, err error) error {
	if err == nil {
		return nil
	}
	return &Error{code: code, err: err}
}

// CodeOf returns the code of the outermost apperr in err's chain; any other error
// (including context.Canceled) is Internal.
func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.code
	}
	return Internal
}

// Is reports whether err's code is code.
func Is(err error, code Code) bool { return CodeOf(err) == code }

// Message returns the message of the outermost apperr in err's chain ("" if none or unset).
func Message(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.msg
	}
	return ""
}
