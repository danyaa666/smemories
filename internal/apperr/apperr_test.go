package apperr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
)

func TestPredefinedCodes(t *testing.T) {
	for _, c := range []struct {
		code Code
		name string
		http int
	}{
		{Internal, "ERROR_INTERNAL", 500},
		{Unauthorized, "ERROR_UNAUTHORIZED", 401},
		{Forbidden, "ERROR_FORBIDDEN", 403},
		{Param, "ERROR_PARAM", 400},
		{NotFound, "ERROR_NOT_FOUND", 404},
		{Conflict, "ERROR_CONFLICT", 409},
		{RateLimited, "ERROR_RATE_LIMITED", 429},
		{TooLarge, "ERROR_TOO_LARGE", 413},
		{Unavailable, "ERROR_UNAVAILABLE", 503},
		{BadGateway, "ERROR_BAD_GATEWAY", 502},
	} {
		if c.code.Name != c.name || c.code.HTTP != c.http {
			t.Errorf("%s = %+v, want HTTP %d", c.name, c.code, c.http)
		}
	}
	if got := NewCode("ERROR_X", 418); got.Name != "ERROR_X" || got.HTTP != 418 {
		t.Errorf("NewCode = %+v", got)
	}
}

func TestErrorText(t *testing.T) {
	cause := io.ErrUnexpectedEOF
	for _, c := range []struct {
		name string
		err  error
		want string
	}{
		{"New", New(NotFound, "yearbook not found"), "yearbook not found"},
		{"Wrap", Wrap(Internal, cause, "insert yearbook"), "insert yearbook: unexpected EOF"},
		{"WithCode keeps the cause text once", WithCode(Conflict, cause), "unexpected EOF"},
		{"nested", Wrap(Internal, Wrap(Conflict, cause, "inner"), "outer"), "outer: inner: unexpected EOF"},
		{"empty message and cause", New(Param, ""), "ERROR_PARAM"},
	} {
		if got := c.err.Error(); got != c.want {
			t.Errorf("%s: Error() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWithCodeNil(t *testing.T) {
	if WithCode(Param, nil) != nil {
		t.Error("WithCode(nil) must stay nil")
	}
}

func TestCodeOfAndIs(t *testing.T) {
	biz := NewCode("ERROR_INVALID_TITLE", 400)
	for _, c := range []struct {
		name string
		err  error
		want Code
	}{
		{"nil", nil, Internal},
		{"plain error", errors.New("boom"), Internal},
		{"context.Canceled", context.Canceled, Internal},
		{"New", New(NotFound, "x"), NotFound},
		{"Wrap", Wrap(Conflict, io.EOF, "x"), Conflict},
		{"WithCode", WithCode(biz, io.EOF), biz},
		{"outermost wins", Wrap(Internal, New(NotFound, "x"), "y"), Internal},
		{"through fmt %w", fmt.Errorf("ctx: %w", New(Forbidden, "x")), Forbidden},
		{"wrapped context.Canceled", Wrap(Internal, context.Canceled, "x"), Internal},
	} {
		if got := CodeOf(c.err); got != c.want {
			t.Errorf("%s: CodeOf = %+v, want %+v", c.name, got, c.want)
		}
		if !Is(c.err, c.want) {
			t.Errorf("%s: Is = false", c.name)
		}
	}
	if Is(New(NotFound, "x"), Conflict) {
		t.Error("Is matched the wrong code")
	}
}

func TestUnwrap(t *testing.T) {
	err := Wrap(Internal, io.ErrUnexpectedEOF, "read")
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Error("errors.Is must reach the cause")
	}
	var e *Error
	if !errors.As(err, &e) || e.code != Internal {
		t.Error("errors.As must find *Error")
	}
	if New(Param, "x").(*Error).Unwrap() != nil {
		t.Error("New has no cause")
	}
}

func TestMessage(t *testing.T) {
	if got := Message(Wrap(Param, io.EOF, "bad title")); got != "bad title" {
		t.Errorf("Message = %q", got)
	}
	if Message(io.EOF) != "" || Message(WithCode(Param, io.EOF)) != "" || Message(nil) != "" {
		t.Error("Message must be empty without an apperr message")
	}
}
