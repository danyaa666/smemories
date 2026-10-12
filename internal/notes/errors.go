package notes

import (
	"strconv"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Business codes of the notes endpoints (docs/api-contract.md section 3); the global ones live in apperr.
var (
	CodeEmailNotVerified     = apperr.NewCode("ERROR_EMAIL_NOT_VERIFIED", 403)
	CodeInvalidLabel         = apperr.NewCode("ERROR_INVALID_LABEL", 400)
	CodeInvalidDeadline      = apperr.NewCode("ERROR_INVALID_DEADLINE", 400)
	CodeLimitReached         = apperr.NewCode("ERROR_LIMIT_REACHED", 409)
	CodeCollectionFull       = apperr.NewCode("ERROR_COLLECTION_FULL", 409)
	CodeCollectionClosed     = apperr.NewCode("ERROR_COLLECTION_CLOSED", 410)
	CodeVerificationFailed   = apperr.NewCode("ERROR_VERIFICATION_FAILED", 403)
	CodeMissingAnswer        = apperr.NewCode("ERROR_MISSING_ANSWER", 400)
	CodeInvalidAnswer        = apperr.NewCode("ERROR_INVALID_ANSWER", 400)
	CodeTooManyPhotos        = apperr.NewCode("ERROR_TOO_MANY_PHOTOS", 400)
	CodeInvalidImage         = apperr.NewCode("ERROR_INVALID_IMAGE", 400)
	CodeUnsupportedMediaType = apperr.NewCode("ERROR_UNSUPPORTED_MEDIA_TYPE", 415)
	CodeQuotaExceeded        = apperr.NewCode("ERROR_QUOTA_EXCEEDED", 409)
	CodeBusy                 = apperr.NewCode("ERROR_BUSY", 503)
	CodeStorage              = apperr.NewCode("ERROR_STORAGE_ERROR", 502)
)

// What the stores and the service answer for the cases a caller must tell apart.
var (
	// ErrNotFound covers a missing collection or yearbook and one owned by someone else: callers cannot tell them apart.
	ErrNotFound     = apperr.New(apperr.NotFound, "not found")
	ErrLimitReached = apperr.New(CodeLimitReached, "at most "+strconv.Itoa(maxActive)+" active links per yearbook")
	// ErrFull: the collection already holds maxNotes notes.
	ErrFull = apperr.New(CodeCollectionFull, "this link has reached its limit of notes")
	// ErrClosed: the collection's deadline has passed.
	ErrClosed = apperr.New(CodeCollectionClosed, "this link no longer accepts notes")
)

// RetryAfterError tells the client when to come back (the Retry-After header); the handler reads it with errors.As, the
// code and message are those of Err.
type RetryAfterError struct {
	Err   error
	After time.Duration
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error { return e.Err }

func tooMany(wait time.Duration) error {
	return &RetryAfterError{Err: apperr.New(apperr.RateLimited, "too many requests"), After: wait}
}
