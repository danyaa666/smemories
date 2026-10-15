package auth

import (
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Business codes of the auth endpoints (docs/api-contract.md section 3); the global ones live in apperr.
// The names are ERROR_ plus the old snake_case code, so the locale keys the web uses did not change.
var (
	CodeInvalidCredentials = apperr.NewCode("ERROR_INVALID_CREDENTIALS", 401)
	CodeEmailTaken         = apperr.NewCode("ERROR_EMAIL_TAKEN", 409)
	CodeInvalidEmail       = apperr.NewCode("ERROR_INVALID_EMAIL", 400)
	CodeWeakPassword       = apperr.NewCode("ERROR_WEAK_PASSWORD", 400)
	CodeInvalidDisplayName = apperr.NewCode("ERROR_INVALID_DISPLAY_NAME", 400)
	CodeInvalidLocale      = apperr.NewCode("ERROR_INVALID_LOCALE", 400)
	CodeInvalidCode        = apperr.NewCode("ERROR_INVALID_CODE", 400)
	CodeCodeExpired        = apperr.NewCode("ERROR_CODE_EXPIRED", 400)
	CodeCodeLocked         = apperr.NewCode("ERROR_CODE_LOCKED", 400)
	CodeBusy               = apperr.NewCode("ERROR_BUSY", 503)
	// The Redis-backed stores fail closed: 503 with their own name, so the client can tell which one is down.
	CodeLimiterUnavailable      = apperr.NewCode("ERROR_LIMITER_UNAVAILABLE", 503)
	CodeSessionStoreUnavailable = apperr.NewCode("ERROR_SESSION_STORE_UNAVAILABLE", 503)
	CodeCodeStoreUnavailable    = apperr.NewCode("ERROR_CODE_STORE_UNAVAILABLE", 503)
)

// What the service and the stores answer for the cases a caller must tell apart.
var (
	ErrInvalidCredentials = apperr.New(CodeInvalidCredentials, "invalid email or password")
	ErrUnauthenticated    = apperr.New(apperr.Unauthorized, "sign in required")
	ErrEmailTaken         = apperr.New(CodeEmailTaken, "email already registered")
	// ErrBusy: every hashing slot stayed taken for the whole wait (retry shortly).
	ErrBusy = apperr.New(CodeBusy, "server busy, retry shortly")

	errInvalidEmail       = apperr.New(CodeInvalidEmail, "email is not valid")
	errWeakPassword       = apperr.New(CodeWeakPassword, "password must be 10-128 characters and differ from the email")
	errInvalidDisplayName = apperr.New(CodeInvalidDisplayName, "display name must be 1-100 characters without control characters")
	errInvalidLocale      = apperr.New(CodeInvalidLocale, "locale must be en or vi")
)

// RateLimitedError says how long the caller must wait; the handler turns it into 429 with Retry-After.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e RateLimitedError) Error() string { return "auth: rate limited" }
