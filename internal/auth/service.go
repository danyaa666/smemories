// Package auth is email + password authentication: users, argon2id hashing, cookie
// sessions, rate limits and the CSRF origin check (D-07).
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/legal"
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

	resendPerHour      = 3  // per user
	resendRefundsHour  = 5  // per user: resend tries given back while the mail provider is down
	verifyTriesPerHour = 20 // per user
	forgotPerIPHour    = 5
	forgotPerEmailHour = 3
	resetTriesPerHour  = 20 // per IP
	usernameTriesPerHr = 20 // failed username changes per user
)

// Session is a freshly issued or refreshed session; Token is the cookie value.
type Session struct {
	Token   string
	Expires time.Time
}

// Limits are the rate limits from config.
//
// Limits is passed by value on purpose: start-up settings, read once.
type Limits struct {
	RegisterPerHour   int // per IP
	LoginFailsPerPair int // per IP + email, per 15 min
	LoginFailsPerIP   int // per IP, per 15 min
	// LoginFailsPerAccount counts failed logins for one account from any address, per 15 min (T-098).
	LoginFailsPerAccount int
}

// Service holds the auth rules; it knows nothing about HTTP.
type Service struct {
	store     *Store
	sessions  *Sessions
	hasher    *Hasher
	dummyHash string // verified for unknown emails so both paths cost the same
	now       func() time.Time

	limiters  *ratelimit.Factory // also builds the Google ones (google.go)
	register  ratelimit.Limiter
	loginPair ratelimit.Limiter
	loginIP   ratelimit.Limiter
	loginAcct ratelimit.Limiter // per account, any address; reset by a successful password reset

	mail          Mail
	bg            sync.WaitGroup // emails being sent in the background
	bgActive      atomic.Int64   // how many of them are still unfinished (Drain counts what it has to drop)
	codes         *Codes
	resend        ratelimit.Limiter
	resendRefunds ratelimit.Limiter // per user: how many outage-refunded resend tries an hour (verify.go)
	verifyTries   ratelimit.Limiter // per user
	forgotIP      ratelimit.Limiter
	forgotEmail   ratelimit.Limiter
	resetIP       ratelimit.Limiter
	usernameFails ratelimit.Limiter // per user
}

// NewService builds a Service. It hashes one throw-away password at startup for the
// unknown-email timing path.
func NewService(store *Store, sessions *Sessions, hasher *Hasher, limits Limits, mail Mail, codes *Codes, limiters *ratelimit.Factory, now func() time.Time) (*Service, error) {
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
		store: store, sessions: sessions, hasher: hasher, dummyHash: dummy, now: now, mail: mail, codes: codes,
		limiters: limiters,
		// Fail policy (D-23, docs/redis.md): the login lockouts and the code-guessing limits fail closed because
		// they stop guessing; everything else fails open.
		register:  limiters.Open("register", limits.RegisterPerHour, registerWindow),
		loginPair: limiters.Closed("login_pair", limits.LoginFailsPerPair, loginWindow),
		loginIP:   limiters.Closed("login_ip", limits.LoginFailsPerIP, loginWindow),
		loginAcct: limiters.Closed("login_account", limits.LoginFailsPerAccount, loginWindow),

		resend: limiters.Open("verify_resend", resendPerHour, time.Hour),
		// Open like resend: it only caps how long a user can keep hitting a provider that is down.
		resendRefunds: limiters.Open("verify_resend_refund", resendRefundsHour, time.Hour),
		verifyTries:   limiters.Closed("verify_tries", verifyTriesPerHour, time.Hour),
		forgotIP:      limiters.Open("forgot_ip", forgotPerIPHour, time.Hour),
		forgotEmail:   limiters.Open("forgot_email", forgotPerEmailHour, time.Hour),
		resetIP:       limiters.Closed("reset_tries", resetTriesPerHour, time.Hour),
		// Closed: the limit stops probing which names exist (T-081).
		usernameFails: limiters.Closed("username_tries", usernameTriesPerHr, time.Hour),
	}, nil
}

// accountKey is the limiter key of an account: the hash of its normalised email, with an oversize
// address collapsed first so an attacker cannot choose arbitrarily large map keys.
func accountKey(email string) string {
	if len(email) > maxEmailLen {
		email = "?"
	}
	return hashKey(email)
}

// hashKey is the hex SHA-256 of a value that must not appear in a Redis key (an email address). Callers
// normalise the value first, so the key is case-insensitive like the address.
func hashKey(v string) string {
	h := sha256.Sum256([]byte(v))
	return hex.EncodeToString(h[:])
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

// Register validates the input (locale is "en" or "vi"; acceptTerms is legal.CurrentVersion), creates the account with the
// acceptance stored and signs it in.
func (s *Service) Register(ctx context.Context, ip, userAgent, email, password, displayName, locale, acceptTerms string) (*User, *Session, error) {
	if acceptTerms != legal.CurrentVersion {
		return nil, nil, errTermsAtRegister
	}
	if locale != "en" && locale != "vi" {
		return nil, nil, errInvalidLocale
	}
	email = normalizeEmail(email)
	if !validEmail(email) {
		return nil, nil, errInvalidEmail
	}
	password = normalizePassword(password)
	if !validPassword(password, email) {
		return nil, nil, errWeakPassword
	}
	name, ok := cleanDisplayName(displayName)
	if !ok {
		return nil, nil, errInvalidDisplayName
	}
	// Only inputs that would cost a hash count against the per-IP limit. Fails open: a registration is not a guess.
	if ok, retry, _ := s.register.Take(ctx, ip); !ok {
		return nil, nil, RateLimitedError{retry}
	}
	phc, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return nil, nil, err
	}
	now := s.now().UTC()
	u := &User{ID: ulid.New(now), Email: email, DisplayName: name, Locale: locale, CreatedAt: now.UnixMilli(), TermsVersion: acceptTerms}
	sess, row, err := s.newSession(now, userAgent)
	if err != nil {
		return nil, nil, err
	}
	err = s.store.createUser(ctx, u, phc, func(id uint64) error {
		row.userID = id
		return s.sessions.Create(ctx, row)
	})
	if err != nil {
		return nil, nil, err
	}
	// The account exists now; the email goes out in the background (the request never waits for the mail provider) and a
	// failure must not undo the account: the user can resend.
	s.sendCodeLater(ctx, u, purposeVerify, "auth: verification email")
	return s.expose(u), sess, nil
}

func (s *Service) newSession(now time.Time, userAgent string) (*Session, session, error) {
	token, hash, err := newToken()
	if err != nil {
		return nil, session{}, err
	}
	exp := now.Add(SessionTTL)
	return &Session{Token: token, Expires: exp}, session{tokenHash: hash, now: now, expires: exp, userAgent: userAgent}, nil
}

// Login checks the credentials and issues a new session. Wrong password, unknown email and
// an account without a password all give ErrInvalidCredentials after the same hashing work.
// oldToken (the cookie the browser sent, if any) is revoked so a session is never reused.
func (s *Service) Login(ctx context.Context, ip, userAgent, email, password, oldToken string) (*User, *Session, error) {
	email = normalizeEmail(email)
	acctKey := accountKey(email)
	pairKey := ip + "|" + acctKey // no email address in a Redis key
	// Every attempt is counted up front and refunded on success (or when we are too busy to
	// judge it), so parallel guesses cannot slip past the limit. All three limiters fail closed (503
	// limiter_unavailable when Redis is down): they are the lockouts that stop password guessing.
	ok, retry, err := s.loginPair.Take(ctx, pairKey)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, RateLimitedError{retry}
	}
	ok, retry, err = s.loginIP.Take(ctx, ip)
	if err != nil || !ok {
		_ = s.loginPair.Refund(ctx, pairKey)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, RateLimitedError{retry}
	}
	ok, retry, err = s.loginAcct.Take(ctx, acctKey)
	if err != nil || !ok {
		_ = s.loginPair.Refund(ctx, pairKey)
		_ = s.loginIP.Refund(ctx, ip)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, RateLimitedError{retry}
	}
	refund := func() {
		_ = s.loginPair.Refund(ctx, pairKey)
		_ = s.loginIP.Refund(ctx, ip)
		_ = s.loginAcct.Refund(ctx, acctKey)
	}

	password = normalizePassword(password)
	if n := utf8.RuneCountInString(password); n == 0 || n > maxPasswordLen { // never hash oversize input
		return nil, nil, ErrInvalidCredentials
	}
	u, phc, err := s.store.userByEmail(ctx, email)
	switch {
	case errors.Is(err, errNotFound) || (err == nil && phc == ""):
		_, err = s.hasher.Verify(ctx, password, s.dummyHash)
		if err == nil {
			return nil, nil, ErrInvalidCredentials
		}
	case err == nil:
		var ok bool
		if ok, err = s.hasher.Verify(ctx, password, phc); err == nil {
			if !ok {
				return nil, nil, ErrInvalidCredentials
			}
			refund()
			return s.issue(ctx, u, userAgent, oldToken)
		}
	}
	if errors.Is(err, ErrBusy) || errors.Is(err, context.Canceled) {
		refund()
	}
	return nil, nil, err
}

func (s *Service) issue(ctx context.Context, u *User, userAgent, oldToken string) (*User, *Session, error) {
	now := s.now().UTC()
	sess, row, err := s.newSession(now, userAgent)
	if err != nil {
		return nil, nil, err
	}
	row.userID, row.epoch = u.InternalID, u.AuthEpoch
	if err := s.sessions.Create(ctx, row); err != nil { // also sweeps the user's expired sessions
		return nil, nil, err
	}
	if oldToken != "" {
		_ = s.sessions.Delete(ctx, hashToken(oldToken)) // best effort
	}
	return s.expose(u), sess, nil
}

// Authenticate resolves a session cookie value to its user. When more than half the TTL
// has elapsed the expiry is pushed out and refreshed is non-nil (the caller re-sends the cookie).
// A session store failure is a *SessionStoreError, never "anonymous".
func (s *Service) Authenticate(ctx context.Context, token string) (u *User, refreshed *Session, err error) {
	if token == "" {
		return nil, nil, ErrUnauthenticated
	}
	hash := hashToken(token)
	now := s.now().UTC()
	uid, epoch, lastSeen, err := s.sessions.Lookup(ctx, hash)
	if errors.Is(err, errNotFound) {
		return nil, nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, nil, err
	}
	// Redis drops the key at its expiry; the check on the time we stored also makes the rule the same under a test clock.
	exp := lastSeen.Add(SessionTTL)
	if !exp.After(now) {
		return nil, nil, ErrUnauthenticated
	}
	u, err = s.store.userByID(ctx, uid)
	if errors.Is(err, errNotFound) {
		return nil, nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, nil, err
	}
	if u.AuthEpoch != epoch { // issued before a password reset or Google merge: revoked
		_ = s.sessions.Delete(ctx, hash) // best effort, the refusal does not depend on it
		return nil, nil, ErrUnauthenticated
	}
	if exp.Sub(now) < SessionTTL/2 {
		alive, err := s.sessions.Extend(ctx, hash, uid, now, SessionTTL)
		if err != nil {
			return nil, nil, err
		}
		if !alive { // logged out (or reset) while we were looking
			return nil, nil, ErrUnauthenticated
		}
		refreshed = &Session{Token: token, Expires: now.Add(SessionTTL)}
	}
	return s.expose(u), refreshed, nil
}

// AcceptTerms stores that u accepted version, which must be the current one (a stale page sends an old one: ERROR_PARAM).
// It returns u with the new acceptance.
func (s *Service) AcceptTerms(ctx context.Context, u *User, version string) (*User, error) {
	if version != legal.CurrentVersion {
		return nil, apperr.New(apperr.Param, "version is not the current Terms version")
	}
	if err := s.store.acceptTerms(ctx, u.InternalID, version, s.now().UnixMilli()); err != nil {
		return nil, err
	}
	c := *u
	c.TermsVersion = version
	return &c, nil
}

// Logout deletes the session; an unknown or empty token is not an error.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.Delete(ctx, hashToken(token))
}
