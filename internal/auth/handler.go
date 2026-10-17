package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/redis"
)

// HandlerConfig carries the HTTP-level settings.
//
// HandlerConfig is passed by value on purpose: start-up settings, read once.
type HandlerConfig struct {
	AllowedOrigins []string // for Guard
	SecureCookie   bool     // false only when SMEM_ENV=dev (plain http://localhost)
	TrustProxy     bool     // SMEM_TRUST_PROXY: client IP = last X-Forwarded-For hop
}

// Handler serves /api/auth/* and /api/user/get-me (docs/api-contract.md): it decodes, calls the Service and writes the
// envelope. Rules live in the Service.
type Handler struct {
	svc    *Service
	cfg    HandlerConfig
	logger *slog.Logger
	google *googleFlow // nil unless EnableGoogle was called
}

func NewHandler(svc *Service, cfg HandlerConfig, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, cfg: cfg, logger: logger}
}

// Routes registers the auth endpoints on mux (pass to httpx.NewRouter).
func (h *Handler) Routes(mux *http.ServeMux) {
	open := func(f http.HandlerFunc) http.Handler { return Guard(h.cfg.AllowedOrigins, f) }
	signedIn := func(f http.HandlerFunc) http.Handler { return Guard(h.cfg.AllowedOrigins, h.RequireUser(f)) }
	mux.Handle("POST /api/auth/register", open(h.register))
	mux.Handle("POST /api/auth/login", open(h.login))
	mux.Handle("POST /api/auth/logout", open(h.logout))
	mux.Handle("POST /api/auth/verify-email", signedIn(h.verifyEmail))
	mux.Handle("POST /api/auth/resend-verification", signedIn(h.resendVerification))
	mux.Handle("POST /api/auth/forgot-password", open(h.forgotPassword))
	mux.Handle("POST /api/auth/reset-password", open(h.resetPassword))
	if h.google != nil {
		mux.HandleFunc("GET /api/auth/google-start", h.googleStart)
		mux.HandleFunc("GET /api/auth/google-callback", h.googleCallback)
	}
	mux.Handle("GET /api/user/get-me", h.RequireUser(http.HandlerFunc(h.me)))
	mux.Handle("POST /api/user/update-username", signedIn(h.updateUsername))
}

type ctxKey struct{}

// UserFrom returns the signed-in user stored by RequireUser.
func UserFrom(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(ctxKey{}).(*User)
	return u, ok
}

// WithUser returns ctx carrying u for UserFrom; RequireUser uses it, and so can a test that calls a handler directly.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// MustUser is UserFrom for handlers: without a signed-in user (a route mounted without RequireUser) it answers
// ERROR_UNAUTHORIZED and returns false, so a handler never runs with a nil user.
func MustUser(w http.ResponseWriter, r *http.Request) (*User, bool) {
	u, ok := UserFrom(r.Context())
	if !ok || u == nil {
		httpx.Fail(w, r, ErrUnauthenticated)
		return nil, false
	}
	return u, true
}

// RequireUser answers 401 unauthenticated unless the request carries a live session;
// otherwise it stores the user for UserFrom, slides the session expiry when due, and
// calls next. Other packages wrap their protected routes with it.
func (h *Handler) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, refreshed, err := h.svc.Authenticate(r.Context(), cookieToken(r))
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if refreshed != nil {
			h.setCookie(w, refreshed)
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}

type credentials struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type registerRequest struct {
	credentials
	Locale any `json:"locale"` // absent or null: "en"; anything but a string is invalid_locale
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var in registerRequest
	if !decode(w, r, &in) {
		return
	}
	locale := "en"
	if in.Locale != nil {
		locale, _ = in.Locale.(string) // a non-string leaves "", which Register rejects
	}
	u, sess, err := h.svc.Register(r.Context(), h.clientIP(r), r.UserAgent(), in.Email, in.Password, in.DisplayName, locale)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.setCookie(w, sess)
	httpx.OK(w, r, u)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	u, sess, err := h.svc.Login(r.Context(), h.clientIP(r), r.UserAgent(), in.Email, in.Password, cookieToken(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.setCookie(w, sess)
	httpx.OK(w, r, u)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Logout(r.Context(), cookieToken(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	h.cookie(w, "", -1, time.Unix(0, 0))
	httpx.OK(w, r, nil)
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, ok := MustUser(w, r)
	if !ok {
		return
	}
	h.respond(w, r, nil, h.svc.VerifyEmail(r.Context(), u, in.Code))
}

// resendResponse is the data of resend-verification.
type resendResponse struct {
	AlreadyVerified bool `json:"already_verified"`
}

func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	u, ok := MustUser(w, r)
	if !ok {
		return
	}
	already, err := h.svc.ResendVerification(r.Context(), u)
	h.respond(w, r, resendResponse{AlreadyVerified: already}, err)
}

func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	h.respond(w, r, nil, h.svc.ForgotPassword(r.Context(), h.clientIP(r), in.Email))
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	h.respond(w, r, nil, h.svc.ResetPassword(r.Context(), h.clientIP(r), in.Email, in.Code, in.Password))
}

// me answers with the user itself; a field added to User (like the username of T-081) shows up here with no other change.
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	u, ok := MustUser(w, r)
	if !ok {
		return
	}
	httpx.OK(w, r, u)
}

func (h *Handler) updateUsername(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, ok := MustUser(w, r)
	if !ok {
		return
	}
	nu, err := h.svc.UpdateUsername(r.Context(), u, in.Username)
	h.respond(w, r, nu, err)
}

// respond answers data, or err when there is one.
func (h *Handler) respond(w http.ResponseWriter, r *http.Request, data any, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.OK(w, r, data)
}

// decode reads exactly one JSON object from the body; on failure it writes the error
// response itself (413 ERROR_TOO_LARGE or 400 ERROR_PARAM).
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	return httpx.DecodeJSON(w, r, v)
}

// fail maps an error to the v2 envelope. Rate limits and the Redis-backed stores that fail closed get their code and a
// Retry-After; unexpected errors are logged by httpx.Fail (never with credentials) and answer ERROR_INTERNAL.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var rl RateLimitedError
	var sse *SessionStoreError
	var cd UsernameCooldownError
	rid := httpx.RequestIDFrom(r.Context())
	switch {
	case errors.As(err, &rl):
		w.Header().Set("Retry-After", strconv.Itoa(max(int((rl.RetryAfter+time.Second-1)/time.Second), 1)))
		err = apperr.New(apperr.RateLimited, "too many attempts")
	case errors.As(err, &cd):
		w.Header().Set("Retry-After", strconv.Itoa(max(int((cd.RetryAfter+time.Second-1)/time.Second), 1)))
		err = apperr.New(CodeUsernameChangeTooSoon, "username can be changed again at "+cd.Next.Format(time.RFC3339))
	case errors.Is(err, ratelimit.ErrUnavailable): // a fail-closed limiter could not count (already logged, throttled)
		w.Header().Set("Retry-After", retryStore)
		err = apperr.New(CodeLimiterUnavailable, "rate limiter unavailable, retry shortly")
	case errors.As(err, &sse): // already logged (once per interval) by the session store
		w.Header().Set("Retry-After", retryStore)
		err = apperr.New(CodeSessionStoreUnavailable, "session store unavailable, retry shortly")
	case errors.Is(err, redis.ErrUnavailable):
		h.logger.Error("auth: code store unavailable", "request_id", rid, "error", err)
		w.Header().Set("Retry-After", retryStore)
		err = apperr.New(CodeCodeStoreUnavailable, "code store unavailable, retry shortly")
	case errors.Is(err, ErrBusy):
		w.Header().Set("Retry-After", retryBusy)
	case errors.Is(err, mailer.ErrUnavailable): // the provider is throttling or down: try again in a little while
		w.Header().Set("Retry-After", retryMail)
		err = apperr.New(CodeMailUnavailable, "the email service is busy, retry shortly")
	case errors.Is(err, mailer.ErrRejected): // the address is the caller's own, so saying so leaks nothing
		err = apperr.New(CodeEmailUndeliverable, "the email address cannot receive mail")
	}
	httpx.Fail(w, r, err)
}

// Retry-After seconds for the 503 answers.
const (
	retryStore = "5"
	retryBusy  = "2"
	retryMail  = "30"
)

func cookieToken(r *http.Request) string {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func (h *Handler) setCookie(w http.ResponseWriter, s *Session) {
	h.cookie(w, s.Token, int(SessionTTL/time.Second), s.Expires)
}

func (h *Handler) cookie(w http.ResponseWriter, value string, maxAge int, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: value, Path: "/", MaxAge: maxAge, Expires: expires,
		HttpOnly: true, Secure: h.cfg.SecureCookie, SameSite: http.SameSiteLaxMode,
	})
}

// ClientIP lets other packages key their own rate limits the same way auth does.
func (h *Handler) ClientIP(r *http.Request) string { return h.clientIP(r) }

// clientIP is httpx.ResolveClientIP with this handler's TrustProxy setting.
func (h *Handler) clientIP(r *http.Request) string {
	return httpx.ResolveClientIP(r, h.cfg.TrustProxy)
}
