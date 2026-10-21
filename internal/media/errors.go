package media

import (
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Business codes of the media endpoints (docs/api-contract.md section 3); the global ones live in apperr.
// The names are the ones the web already localises (ERROR_ plus the old snake_case code).
var (
	CodeUnsupportedMediaType = apperr.NewCode("ERROR_UNSUPPORTED_MEDIA_TYPE", 415)
	CodeInvalidImage         = apperr.NewCode("ERROR_INVALID_IMAGE", 400)
	CodeQuotaExceeded        = apperr.NewCode("ERROR_QUOTA_EXCEEDED", 409)
	CodeBusy                 = apperr.NewCode("ERROR_BUSY", 503)
	// CodeStorage is the object store failing: 502, the BadGateway family, with its own name for the client.
	CodeStorage = apperr.NewCode("ERROR_STORAGE_ERROR", 502)
)

// What the stores, the image pipeline and the service answer for the cases a caller must tell apart.
var (
	// ErrNotFound covers media or a yearbook that does not exist and one owned by someone else.
	ErrNotFound = apperr.New(apperr.NotFound, "not found")
	// ErrQuota: the yearbook or the user is at a storage limit.
	ErrQuota = apperr.New(CodeQuotaExceeded, "storage quota reached")
	// ErrUnsupported: the content is not a JPEG, PNG or WebP image (decided by sniffing, never by name or Content-Type).
	ErrUnsupported = apperr.New(CodeUnsupportedMediaType, "only JPEG, PNG and WebP images are accepted")
	// ErrInvalidImage: the content looks like a supported image but is corrupt or too large to process safely.
	ErrInvalidImage = apperr.New(CodeInvalidImage, "the image is corrupt or too large")
	// ErrBusy: every image-processing slot stayed taken for the whole wait (retry shortly).
	ErrBusy = apperr.New(CodeBusy, "too many uploads in progress, retry shortly")
)

// storageFailed wraps a failure of the object store (502 ERROR_STORAGE_ERROR).
func storageFailed(err error) error { return apperr.Wrap(CodeStorage, err, "object storage failed") }

// dbError wraps an unexpected database failure with what was being done; it is ERROR_INTERNAL and only logged.
func dbError(err error, what string) error { return apperr.Wrap(apperr.Internal, err, what) }

// RateLimitedError tells the client when to come back (the Retry-After header); the handler reads it with errors.As,
// the code and message are those of Err.
type RateLimitedError struct {
	Err        error
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string { return e.Err.Error() }
func (e *RateLimitedError) Unwrap() error { return e.Err }

func tooManyUploads(wait time.Duration) error {
	return &RateLimitedError{Err: apperr.New(apperr.RateLimited, "too many uploads, try again later"), RetryAfter: wait}
}

func tooManyPublic(wait time.Duration) error {
	return &RateLimitedError{Err: apperr.New(apperr.RateLimited, "too many requests, try again later"), RetryAfter: wait}
}
