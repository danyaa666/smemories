package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/ratelimit"
)

// Google sign-in: OpenID Connect authorization-code flow with PKCE (D-07). The browser leaves
// through /v1/auth/google/start and comes back to /v1/auth/google/callback; the only state
// between the two is the signed smem_oidc cookie, which also binds the flow to the browser
// that started it (login CSRF). Failures redirect to /login?error=<code> with a fixed code;
// provider errors, codes and tokens are never put in a redirect or a log line.

const (
	oidcCookie        = "smem_oidc"
	oidcTTL           = 10 * time.Minute
	googleRateLimit   = 30
	googleRateWindow  = 15 * time.Minute
	googleHTTPTimeout = 5 * time.Second
	maxReturnToLen    = 200
	minCookieKeyLen   = 32

	// Redirect error codes (the web app localises them).
	errOIDCState  = "oidc_state"
	errOIDCDenied = "oidc_denied"
	errOIDCFailed = "oidc_failed"
	errUnverified = "email_unverified"
)

// GoogleConfig turns Google sign-in on; see Handler.EnableGoogle.
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	Issuer       string // https://accounts.google.com unless testing
	RedirectURL  string // <public base URL>/api/v1/auth/google/callback
	CookieKey    []byte // HMAC key for smem_oidc, at least 32 bytes
	HTTPClient   *http.Client
}

type googleFlow struct {
	cfg    GoogleConfig
	client *http.Client

	startIP    *ratelimit.Limiter
	callbackIP *ratelimit.Limiter

	mu       sync.Mutex
	provider *oidc.Provider // discovered on first use, then cached
}

// EnableGoogle registers the Google routes on this handler. Call it before Routes. Without
// it the two endpoints do not exist (404 not_found).
func (h *Handler) EnableGoogle(cfg GoogleConfig) error {
	switch {
	case cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.Issuer == "" || cfg.RedirectURL == "":
		return errors.New("auth: google config needs client id, secret, issuer and redirect URL")
	case len(cfg.CookieKey) < minCookieKeyLen:
		return fmt.Errorf("auth: google cookie key must be at least %d bytes", minCookieKeyLen)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: googleHTTPTimeout}
	}
	h.google = &googleFlow{
		cfg: cfg, client: client,
		startIP:    ratelimit.New(googleRateLimit, googleRateWindow, h.svc.now),
		callbackIP: ratelimit.New(googleRateLimit, googleRateWindow, h.svc.now),
	}
	return nil
}

// discover fetches the provider's discovery document once and caches it (a failure is not cached).
// ponytail: the lock is held during the 5 s fetch, so callers queue behind an outage; use singleflight if that ever shows.
func (g *googleFlow) discover(ctx context.Context) (*oidc.Provider, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.provider == nil {
		ctx, cancel := g.outbound(ctx)
		defer cancel()
		p, err := oidc.NewProvider(ctx, g.cfg.Issuer)
		if err != nil {
			return nil, err
		}
		g.provider = p
	}
	return g.provider, nil
}

// outbound is a context for one call to the provider: our HTTP client, 5 s at most.
func (g *googleFlow) outbound(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, googleHTTPTimeout)
	return oidc.ClientContext(ctx, g.client), cancel
}

func (g *googleFlow) oauth(p *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID: g.cfg.ClientID, ClientSecret: g.cfg.ClientSecret, Endpoint: p.Endpoint(),
		RedirectURL: g.cfg.RedirectURL, Scopes: []string{oidc.ScopeOpenID, "email", "profile"},
	}
}

// oidcState is the content of the smem_oidc cookie.
type oidcState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	ReturnTo string `json:"r"`
	Expires  int64  `json:"e"` // unix seconds
}

func (g *googleFlow) mac(payload string) []byte {
	m := hmac.New(sha256.New, g.cfg.CookieKey)
	m.Write([]byte(payload))
	return m.Sum(nil)
}

func (g *googleFlow) seal(st oidcState) string {
	b, _ := json.Marshal(st)
	p := base64.RawURLEncoding.EncodeToString(b)
	return p + "." + base64.RawURLEncoding.EncodeToString(g.mac(p))
}

// open returns the state in the request's smem_oidc cookie, or false when it is missing,
// altered or expired.
func (g *googleFlow) open(r *http.Request, now time.Time) (oidcState, bool) {
	c, err := r.Cookie(oidcCookie)
	if err != nil {
		return oidcState{}, false
	}
	p, sig, ok := strings.Cut(c.Value, ".")
	// Compare the encoded form: base64 decoding ignores spare bits, which would accept altered strings.
	if !ok || !hmac.Equal([]byte(sig), []byte(base64.RawURLEncoding.EncodeToString(g.mac(p)))) {
		return oidcState{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(p)
	var st oidcState
	if err != nil || json.Unmarshal(raw, &st) != nil || st.State == "" || st.Nonce == "" || st.Verifier == "" || now.Unix() >= st.Expires {
		return oidcState{}, false
	}
	return st, true
}

func (h *Handler) setOIDCCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: oidcCookie, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: h.cfg.SecureCookie, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) googleStart(w http.ResponseWriter, r *http.Request) {
	g := h.google
	w.Header().Set("Cache-Control", "no-store")
	if ok, retry := g.startIP.Take(h.clientIP(r)); !ok {
		h.fail(w, r, RateLimitedError{retry})
		return
	}
	p, err := g.discover(r.Context())
	if err != nil {
		h.logger.Error("auth: google discovery failed", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
		redirectLogin(w, errOIDCFailed)
		return
	}
	st := oidcState{
		State: oauth2.GenerateVerifier(), Nonce: oauth2.GenerateVerifier(), Verifier: oauth2.GenerateVerifier(),
		ReturnTo: safeReturnTo(r.URL.Query().Get("return_to")), Expires: h.svc.now().Add(oidcTTL).Unix(),
	}
	h.setOIDCCookie(w, g.seal(st), int(oidcTTL/time.Second))
	w.Header().Set("Location", g.oauth(p).AuthCodeURL(st.State, oauth2.S256ChallengeOption(st.Verifier), oidc.Nonce(st.Nonce)))
	w.WriteHeader(http.StatusFound)
}

func (h *Handler) googleCallback(w http.ResponseWriter, r *http.Request) {
	g := h.google
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if ok, retry := g.callbackIP.Take(h.clientIP(r)); !ok {
		h.fail(w, r, RateLimitedError{retry})
		return
	}
	h.setOIDCCookie(w, "", -1) // single use, whatever happens next
	rid := httpx.RequestIDFrom(r.Context())
	reject := func(code string, err error) {
		h.logger.Warn("auth: google sign-in rejected", "request_id", rid, "reason", code, "error", safeErr(err))
		redirectLogin(w, code)
	}

	// The existing session cookie is ignored here; it is only revoked on success.
	st, ok := g.open(r, h.svc.now())
	q := r.URL.Query()
	if !ok || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(st.State)) != 1 {
		reject(errOIDCState, nil)
		return
	}
	if q.Get("error") != "" {
		reject(errOIDCDenied, nil) // the provider's error text is not logged
		return
	}
	id, code, err := g.identity(r.Context(), q.Get("code"), st)
	if err != nil {
		reject(code, err)
		return
	}
	u, sess, err := h.svc.SignInGoogle(r.Context(), r.UserAgent(), cookieToken(r), id)
	if err != nil {
		reject(errOIDCFailed, err)
		return
	}
	h.logger.Info("auth: google sign-in", "request_id", rid, "user_id", u.ID)
	h.setCookie(w, sess)
	w.Header().Set("Location", safeReturnTo(st.ReturnTo))
	w.WriteHeader(http.StatusFound)
}

// identity exchanges the code and returns the verified Google identity; on failure it also
// returns the redirect error code.
func (g *googleFlow) identity(ctx context.Context, code string, st oidcState) (GoogleIdentity, string, error) {
	if code == "" {
		return GoogleIdentity{}, errOIDCFailed, errors.New("no code")
	}
	p, err := g.discover(ctx)
	if err != nil {
		return GoogleIdentity{}, errOIDCFailed, err
	}
	ctx, cancel := g.outbound(ctx)
	defer cancel()
	tok, err := g.oauth(p).Exchange(ctx, code, oauth2.VerifierOption(st.Verifier))
	if err != nil {
		return GoogleIdentity{}, errOIDCFailed, err
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return GoogleIdentity{}, errOIDCFailed, errors.New("no id_token")
	}
	idt, err := p.Verifier(&oidc.Config{ClientID: g.cfg.ClientID}).Verify(ctx, raw) // signature, iss, aud, exp
	if err != nil {
		return GoogleIdentity{}, errOIDCFailed, err
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(st.Nonce)) != 1 {
		return GoogleIdentity{}, errOIDCFailed, errors.New("nonce mismatch")
	}
	var c struct {
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Name          string `json:"name"`
		Locale        string `json:"locale"`
	}
	if err := idt.Claims(&c); err != nil {
		return GoogleIdentity{}, errOIDCFailed, err
	}
	if v, _ := c.EmailVerified.(bool); !v { // the string "true" does not count
		return GoogleIdentity{}, errUnverified, errors.New("email not verified")
	}
	return GoogleIdentity{Subject: idt.Subject, Email: c.Email, Name: c.Name, Locale: c.Locale}, "", nil
}

func redirectLogin(w http.ResponseWriter, code string) {
	w.Header().Set("Location", "/login?error="+url.QueryEscape(code))
	w.WriteHeader(http.StatusFound)
}

// safeErr keeps provider response bodies out of the logs.
func safeErr(err error) string {
	var re *oauth2.RetrieveError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &re):
		return fmt.Sprintf("token endpoint answered %d", re.Response.StatusCode)
	}
	return err.Error()
}

// safeReturnTo accepts only a same-site relative path: one leading "/" (not "//" or "/\"),
// no control or format characters, at most 200 bytes. Anything else becomes "/".
func safeReturnTo(s string) string {
	if s == "" || len(s) > maxReturnToLen || s[0] != '/' || (len(s) > 1 && (s[1] == '/' || s[1] == '\\')) {
		return "/"
	}
	for _, r := range s {
		if unicode.In(r, unicode.Cc, unicode.Cf) || r == unicode.ReplacementChar {
			return "/"
		}
	}
	return s
}

// GoogleIdentity is what a verified Google ID token says about the user.
type GoogleIdentity struct {
	Subject string
	Email   string
	Name    string
	Locale  string
}

// SignInGoogle links or creates the account for a verified Google identity and signs it in.
// oldToken (the session cookie the browser sent, if any) is revoked.
//
// Rules: a known subject signs in; else a verified local account with the same email gets
// the identity linked; else an unverified local account (possibly squatted by someone who
// does not own the address) loses its password and sessions, is marked verified and linked;
// else a social-only account is created.
func (s *Service) SignInGoogle(ctx context.Context, userAgent, oldToken string, id GoogleIdentity) (User, Session, error) {
	id.Email = normalizeEmail(id.Email)
	if !validEmail(id.Email) || id.Subject == "" || len(id.Subject) > 255 {
		return User{}, Session{}, errors.New("auth: google identity without usable subject or email")
	}
	now := s.now().UTC()
	name, ok := cleanDisplayName(id.Name)
	if !ok {
		local, _, _ := strings.Cut(id.Email, "@")
		if name, ok = cleanDisplayName(local); !ok {
			name = "Student"
		}
	}
	locale := "en"
	if strings.HasPrefix(strings.ToLower(id.Locale), "vi") {
		locale = "vi"
	}
	u, err := s.store.googleUser(ctx, id, User{DisplayName: name, Locale: locale}, now)
	if err != nil {
		return User{}, Session{}, err
	}
	return s.issue(ctx, u, userAgent, oldToken)
}
