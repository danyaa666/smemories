package notes

import (
	"context"
	"net/http"
)

// Verifier decides whether a submission comes from a person (a CAPTCHA, Q-005). It runs after the body was read
// (so it sees headers only) and before any photo is processed; a non-nil error rejects the submission with 403 verification_failed.
type Verifier interface {
	Verify(ctx context.Context, r *http.Request) error
}

// allowAll is the default Verifier: no check.
type allowAll struct{}

func (allowAll) Verify(context.Context, *http.Request) error { return nil }
