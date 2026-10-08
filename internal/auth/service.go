// Package auth is email + password authentication: users, argon2id hashing, cookie
// sessions, rate limits and the CSRF origin check (D-07).
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/ulid"
)

const (
	// SessionTTL is the lifetime of a session; it slides forward once half has elapsed.
	SessionTTL = 30 * 24 * time.Hour
	// HashWait is how long a request waits for a free hashing slot before 503 busy.
	HashWait = 2 * time.Second

	loginWindow    = 15 * time.Minute
	registerWindow = time.Hour
)

var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrUnauthenticated    = errors.New("auth: unauthenticated")
)

// ValidationError carries the stable API error code for a rejected input.
type ValidationError struct{ Code string }

func (e ValidationError) Error() string { return "auth: " + e.Code }

// RateLimitedError says how long the caller must wait.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e RateLimitedError) Error() string { return "auth: rate limited" }

// Session is a freshly issued or refreshed session; Token is the cookie value.
type Session struct {
	Token   string
	Expires time.Time
}

// Limits are the rate limits from config.
type Limits struct {
	RegisterPerHour   int // per IP
	LoginFailsPerPair int // per IP + email, per 15 min
	LoginFailsPerIP   int // per IP, per 15 min
}

// Service holds the auth rules; it knows nothing about HTTP.
type Service struct {
	store     *Store
	hasher    *Hasher
	dummyHash string // verified for unknown emails so both paths cost the same
	now       func() time.Time

	register  *ratelimit.Limiter
	loginPair *ratelimit.Limiter
	loginIP   *ratelimit.Limiter

	mail        Mail
	bg          sync.WaitGroup // emails being sent in the background
	resend      *ratelimit.Limiter
	forgotIP    *ratelimit.Limiter
	forgotEmail *ratelimit.Limiter
}

// NewService builds a Service. It hashes one throw-away password at startup for the
// unknown-email timing path.
func NewService(store *Store, hasher *Hasher, limits Limits, mail Mail, now func() time.Time) (*Service, error) {
	if now == nil {
		now = time.Now
	}
	var junk [16]byte
	_, _ = rand.Read(junk[:])
	dummy, err := hasher.Hash(context.Background(), base64.RawStdEncoding.EncodeToString(junk[:]))
	if err != nil {
		return nil, err
	}
	return &Service{
		store: store, hasher: hasher, dummyHash: dummy, now: now, mail: mail,
		register:  ratelimit.New(limits.RegisterPerHour, registerWindow, now),
		loginPair: ratelimit.New(limits.LoginFailsPerPair, loginWindow, now),
		loginIP:   ratelimit.New(limits.LoginFailsPerIP, loginWindow, now),

		resend:      ratelimit.New(resendPerHour, time.Hour, now),
		forgotIP:    ratelimit.New(forgotPerIPHour, time.Hour, now),
		forgotEmail: ratelimit.New(forgotPerEmailHour, time.Hour, now),
	}, nil
}

func newToken() (token string, hash []byte, err error) {
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(b[:])
	return token, hashToken(token), nil
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// Register validates the input (locale is "en" or "vi"), creates the account and signs it in.
func (s *Service) Register(ctx context.Context, ip, userAgent, email, password, displayName, locale string) (User, Session, error) {
	if locale != "en" && locale != "vi" {
		return User{}, Session{}, ValidationError{codeInvalidLocale}
	}
	email = normalizeEmail(email)
	if !validEmail(email) {
		return User{}, Session{}, ValidationError{codeInvalidEmail}
	}
	password = normalizePassword(password)
	if !validPassword(password, email) {
		return User{}, Session{}, ValidationError{codeWeakPassword}
	}
	name, ok := cleanDisplayName(displayName)
	if !ok {
		return User{}, Session{}, ValidationError{codeInvalidDisplayName}
	}
	// Only inputs that would cost a hash count against the per-IP limit.
	if ok, retry := s.register.Take(ip); !ok {
		return User{}, Session{}, RateLimitedError{retry}
	}
	phc, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return User{}, Session{}, err
	}
	now := s.now().UTC()
	u := User{ID: ulid.New(now), Email: email, DisplayName: name, Locale: locale, CreatedAt: now}
	sess, row, err := s.newSession(now, userAgent)
	if err != nil {
		return User{}, Session{}, err
	}
	if err := s.store.createUserWithSession(ctx, &u, phc, row); err != nil {
		return User{}, Session{}, err
	}
	// The account exists now; a mail failure must not undo that, the user can resend.
	if err := s.sendToken(ctx, u, purposeVerify); err != nil {
		s.mail.Logger.Error("auth: verification email not sent", "user_id", u.ID, "error", err)
	}
	return u, sess, nil
}

func (s *Service) newSession(now time.Time, userAgent string) (Session, session, error) {
	token, hash, err := newToken()
	if err != nil {
		return Session{}, session{}, err
	}
	exp := now.Add(SessionTTL)
	return Session{Token: token, Expires: exp}, session{tokenHash: hash, now: now, expires: exp, userAgent: userAgent}, nil
}

// Login checks the credentials and issues a new session. Wrong password, unknown email and
// an account without a password all give ErrInvalidCredentials after the same hashing work.
// oldToken (the cookie the browser sent, if any) is revoked so a session is never reused.
func (s *Service) Login(ctx context.Context, ip, userAgent, email, password, oldToken string) (User, Session, error) {
	email = normalizeEmail(email)
	emailKey := email
	if len(emailKey) > maxEmailLen {
		emailKey = "?" // do not let an attacker choose arbitrarily large map keys
	}
	pairKey := ip + "|" + emailKey
	// Every attempt is counted up front and refunded on success (or when we are too busy to
	// judge it), so parallel guesses cannot slip past the limit.
	if ok, retry := s.loginPair.Take(pairKey); !ok {
		return User{}, Session{}, RateLimitedError{retry}
	}
	if ok, retry := s.loginIP.Take(ip); !ok {
		s.loginPair.Refund(pairKey)
		return User{}, Session{}, RateLimitedError{retry}
	}
	refund := func() { s.loginPair.Refund(pairKey); s.loginIP.Refund(ip) }

	password = normalizePassword(password)
	if n := utf8.RuneCountInString(password); n == 0 || n > maxPasswordLen { // never hash oversize input
		return User{}, Session{}, ErrInvalidCredentials
	}
	u, phc, err := s.store.userByEmail(ctx, email)
	switch {
	case errors.Is(err, errNotFound) || (err == nil && phc == ""):
		_, err = s.hasher.Verify(ctx, password, s.dummyHash)
		if err == nil {
			return User{}, Session{}, ErrInvalidCredentials
		}
	case err == nil:
		var ok bool
		if ok, err = s.hasher.Verify(ctx, password, phc); err == nil {
			if !ok {
				return User{}, Session{}, ErrInvalidCredentials
			}
			refund()
			return s.issue(ctx, u, userAgent, oldToken)
		}
	}
	if errors.Is(err, ErrBusy) || errors.Is(err, context.Canceled) {
		refund()
	}
	return User{}, Session{}, err
}

func (s *Service) issue(ctx context.Context, u User, userAgent, oldToken string) (User, Session, error) {
	now := s.now().UTC()
	sess, row, err := s.newSession(now, userAgent)
	if err != nil {
		return User{}, Session{}, err
	}
	row.userID = u.InternalID
	if err := s.store.createSession(ctx, row); err != nil {
		return User{}, Session{}, err
	}
	if oldToken != "" {
		_ = s.store.deleteSession(ctx, hashToken(oldToken)) // best effort
	}
	_ = s.store.deleteExpiredSessions(ctx, u.InternalID, now) // best effort
	return u, sess, nil
}

// Authenticate resolves a session cookie value to its user. When more than half the TTL
// has elapsed the expiry is pushed out and refreshed is non-nil (the caller re-sends the cookie).
func (s *Service) Authenticate(ctx context.Context, token string) (u User, refreshed *Session, err error) {
	if token == "" {
		return User{}, nil, ErrUnauthenticated
	}
	hash := hashToken(token)
	now := s.now().UTC()
	u, exp, err := s.store.userBySession(ctx, hash, now)
	if errors.Is(err, errNotFound) {
		return User{}, nil, ErrUnauthenticated
	}
	if err != nil {
		return User{}, nil, err
	}
	if exp.Sub(now) < SessionTTL/2 {
		newExp := now.Add(SessionTTL)
		if err := s.store.extendSession(ctx, hash, now, newExp); err != nil {
			return User{}, nil, err
		}
		refreshed = &Session{Token: token, Expires: newExp}
	}
	return u, refreshed, nil
}

// Logout deletes the session; an unknown or empty token is not an error.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.deleteSession(ctx, hashToken(token))
}
