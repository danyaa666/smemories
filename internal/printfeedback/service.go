// Package printfeedback counts the answers to "Does the PDF look right?" on the print screen (T-061), per browser family,
// so a failing phone or browser shows up early. Only counters are stored: no user, book, IP or user agent.
package printfeedback

import (
	"context"
	"slices"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/ratelimit"
)

// The accepted values (VARCHAR columns, no ENUM). The families are the print guides of the web app: Chrome and Edge on a
// computer share one, so do they here.
var (
	Verdicts = []string{"yes", "no"}
	Families = []string{"chromium_desktop", "safari_desktop", "firefox", "chrome_android", "safari_ios", "other"}
)

const (
	perIPHour  = 30 // a person tries a few times at most; a script is cut off
	retryFloor = time.Second
)

// store is what the Service needs; the MySQL Store implements it.
type store interface {
	increment(ctx context.Context, family, verdict string, nowMs int64) error
}

// Service validates an answer, limits the caller and bumps the counter.
type Service struct {
	store store
	ip    ratelimit.Limiter
	now   func() time.Time
}

// NewService builds the service; the limiter fails open (a counter is not worth a 503). now may be nil (time.Now).
func NewService(st *Store, limiters *ratelimit.Factory, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: st, ip: limiters.Open("print_feedback_ip", perIPHour, time.Hour), now: now}
}

// RetryAfterError is the 429: the handler turns After into the Retry-After header.
type RetryAfterError struct {
	Err   error
	After time.Duration
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error { return e.Err }

// Create records one answer. Invalid values are ERROR_PARAM; the per-IP limit is a *RetryAfterError.
func (s *Service) Create(ctx context.Context, ip, verdict, family string) error {
	if !slices.Contains(Verdicts, verdict) {
		return apperr.New(apperr.Param, "verdict must be yes or no")
	}
	if !slices.Contains(Families, family) {
		return apperr.New(apperr.Param, "browser_family is not a known value")
	}
	if ok, wait, _ := s.ip.Take(ctx, ip); !ok {
		return &RetryAfterError{Err: apperr.New(apperr.RateLimited, "too many requests"), After: max(wait, retryFloor)}
	}
	return s.store.increment(ctx, family, verdict, s.now().UnixMilli())
}
