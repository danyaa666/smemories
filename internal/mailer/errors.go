package mailer

// sendError is a classified failure. Its text is the class and a short reason chosen by the provider file (an error code, never
// the provider's message, which may contain the recipient address); the cause stays reachable for errors.Is / errors.As, so a
// context deadline still matches context.DeadlineExceeded.
type sendError struct {
	kind   error
	reason string
	cause  error
}

func (e *sendError) Error() string { return e.kind.Error() + " (" + e.reason + ")" }

func (e *sendError) Unwrap() []error { return []error{e.kind, e.cause} }

// classify wraps cause as kind (ErrUnavailable or ErrRejected) with a short reason.
func classify(kind error, reason string, cause error) error {
	return &sendError{kind: kind, reason: reason, cause: cause}
}
