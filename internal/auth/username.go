package auth

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Usernames (T-081, D-28, docs/usernames.md): a unique public handle, not a login name.
const (
	minUsernameLen = 3
	maxUsernameLen = 30
	// usernameCooldown is how long a user must wait after a user-made change.
	usernameCooldown = 7 * 24 * time.Hour
	defaultPrefix    = "user-"
)

// defaultSuffixLens are the slices of the public id tried for the default name: the last 8, 10 and 14 characters after "user-",
// then (0) the whole 26-character id without the prefix, which still fits the 30-character rule.
var defaultSuffixLens = [...]int{8, 10, 14, 0}

// reservedUsernames cannot be claimed (the "user-" prefix is handled separately in reservedUsername).
var reservedUsernames = map[string]bool{
	"admin": true, "administrator": true, "support": true, "help": true, "root": true, "system": true, "smemories": true,
	"api": true, "www": true, "mail": true, "team": true, "staff": true, "moderator": true, "official": true,
	"null": true, "undefined": true,
}

// defaultUsername is the generated name for publicID using the n-th slice length of defaultSuffixLens.
func defaultUsername(publicID string, attempt int) string {
	id := asciiLower(publicID)
	n := defaultSuffixLens[attempt]
	if n == 0 {
		return id
	}
	return defaultPrefix + id[max(len(id)-n, 0):]
}

// normalizeUsername trims and lower-cases ASCII letters only: a non-ASCII look-alike (the Kelvin sign lower-cases to "k")
// must stay invalid instead of turning into a valid name.
func normalizeUsername(s string) string { return asciiLower(strings.TrimSpace(s)) }

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

func isAlnum(c byte) bool { return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' }

// validUsername reports whether an already normalised name has 3-30 characters of a-z 0-9 . _ -, starts and ends with a
// letter or digit, and never has two special characters in a row.
func validUsername(s string) bool {
	if len(s) < minUsernameLen || len(s) > maxUsernameLen || !isAlnum(s[0]) || !isAlnum(s[len(s)-1]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		switch c := s[i]; {
		case isAlnum(c):
		case c == '.' || c == '_' || c == '-':
			if !isAlnum(s[i-1]) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// reservedUsername reports whether name (valid, normalised) is off limits to the user with publicID: the fixed list, and any
// "user-" name that is not one of that user's own generated defaults.
func reservedUsername(name, publicID string) bool {
	if reservedUsernames[name] {
		return true
	}
	if !strings.HasPrefix(name, defaultPrefix) {
		return false
	}
	for i := range defaultSuffixLens {
		if name == defaultUsername(publicID, i) {
			return false
		}
	}
	return true
}

// expose fills UsernameNextChangeAt from the service clock: nil when a change is allowed now.
// It returns a copy, so the caller's user (for example the one in the request context) is never changed.
func (s *Service) expose(u *User) *User {
	c := *u
	c.UsernameNextChangeAt = nil
	if next := c.UsernameChangedAt + usernameCooldown.Milliseconds(); c.UsernameChangedAt > 0 && next > s.now().UnixMilli() {
		c.UsernameNextChangeAt = &next
	}
	return &c
}

// UpdateUsername changes u's public username (normalised: trimmed, lower-cased) at most once per usernameCooldown and returns
// the updated user. Setting the current name again succeeds and does not start the cooldown. Taken, invalid and reserved
// names count against usernameTriesPerHr per user (ERROR_RATE_LIMITED), so the endpoint cannot probe which names exist;
// successes, no-ops and cooldown answers are refunded. The cooldown check and the write are one conditional UPDATE.
func (s *Service) UpdateUsername(ctx context.Context, u *User, raw string) (*User, error) {
	if ok, retry, err := s.usernameFails.Take(ctx, u.ID); err != nil {
		return nil, err
	} else if !ok {
		return nil, RateLimitedError{retry}
	}
	failed := true
	defer func() {
		if !failed {
			_ = s.usernameFails.Refund(ctx, u.ID)
		}
	}()

	name := normalizeUsername(raw)
	switch {
	case !validUsername(name):
		return nil, errInvalidUsername
	case name == u.Username:
		failed = false
		return s.expose(u), nil
	case reservedUsername(name, u.ID):
		return nil, errUsernameReserved
	}
	now := s.now().UTC()
	nowMs := now.UnixMilli()
	changed, err := s.store.changeUsername(ctx, u.InternalID, name, nowMs, nowMs-usernameCooldown.Milliseconds())
	if errors.Is(err, ErrUsernameTaken) {
		return nil, err // counts as a failed attempt
	}
	if err != nil {
		failed = false // our fault, not the caller's
		return nil, err
	}
	failed = false
	if !changed { // cooldown still running, or the account is gone: read the row to tell which
		cur, err := s.store.userByID(ctx, u.InternalID)
		if errors.Is(err, errNotFound) {
			return nil, ErrUnauthenticated
		}
		if err != nil {
			return nil, err
		}
		next := time.UnixMilli(cur.UsernameChangedAt + usernameCooldown.Milliseconds())
		return nil, UsernameCooldownError{Next: next.UTC(), RetryAfter: max(next.Sub(now), time.Second)}
	}
	upd := *u
	upd.Username, upd.UsernameChangedAt = name, nowMs
	return s.expose(&upd), nil
}
