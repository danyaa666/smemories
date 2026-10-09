package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/redis"
)

// HandlerConfig carries the HTTP-level settings.
type HandlerConfig struct {
	AllowedOrigins []string // for Guard
	SecureCookie   bool     // false only when SMEM_ENV=dev (plain http://localhost)
	TrustProxy     bool     // SMEM_TRUST_PROXY: client IP = last X-Forwarded-For hop
}

// Handler serves /v1/auth/* and /v1/me.
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
	guard := func(f http.HandlerFunc) http.Handler { return Guard(h.cfg.AllowedOrigins, f) }
	mux.Handle("POST /v1/auth/register", guard(h.register))
	mux.Handle("POST /v1/auth/login", guard(h.login))
	mux.Handle("POST /v1/auth/logout", guard(h.logout))
	mux.Handle("POST /v1/auth/verify-email", Guard(h.cfg.AllowedOrigins, h.RequireUser(http.HandlerFunc(h.verifyEmail))))
	mux.Handle("POST /v1/auth/verify-email/resend", Guard(h.cfg.AllowedOrigins, h.RequireUser(http.HandlerFunc(h.resendVerification))))
	mux.Handle("POST /v1/auth/forgot-password", guard(h.forgotPassword))
	mux.Handle("POST /v1/auth/reset-password", guard(h.resetPassword))
	if h.google != nil {
		mux.HandleFunc("GET /v1/auth/google/start", h.googleStart)
		mux.HandleFunc("GET /v1/auth/google/callback", h.googleCallback)
	}
	mux.Handle("GET /v1/me", h.RequireUser(http.HandlerFunc(h.me)))
}

type ctxKey struct{}

// UserFrom returns the signed-in user stored by RequireUser.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
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
			h.setCookie(w, *refreshed)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
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
	httpx.WriteJSON(w, http.StatusCreated, map[string]User{"user": u})
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
	httpx.WriteJSON(w, http.StatusOK, map[string]User{"user": u})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Logout(r.Context(), cookieToken(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	h.cookie(w, "", -1, time.Unix(0, 0))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, _ := UserFrom(r.Context())
	if err := h.svc.VerifyEmail(r.Context(), u, in.Code); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFrom(r.Context())
	already, err := h.svc.ResendVerification(r.Context(), u)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if already {
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"already_verified": true})
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, struct{}{})
}

func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), h.clientIP(r), in.Email); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, struct{}{})
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
	if err := h.svc.ResetPassword(r.Context(), h.clientIP(r), in.Email, in.Code, in.Password); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFrom(r.Context())
	httpx.WriteJSON(w, http.StatusOK, map[string]User{"user": u})
}

// decode reads exactly one JSON object from the body; on failure it writes the error
// response itself (413 payload_too_large or 400 invalid_body).
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	return httpx.DecodeJSON(w, r, v, false)
}

// fail maps a service error to the shared envelope; unexpected errors are logged (never
// with credentials) and answer 500 internal_error.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ve ValidationError
	var rl RateLimitedError
	var sse *SessionStoreError
	switch {
	case errors.As(err, &ve):
		httpx.WriteError(w, r, http.StatusBadRequest, ve.Code, "invalid input: "+ve.Code)
	case errors.Is(err, ErrInvalidCode):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_code", "invalid code")
	case errors.Is(err, ErrCodeExpired):
		httpx.WriteError(w, r, http.StatusBadRequest, "code_expired", "code expired, request a new one")
	case errors.Is(err, ErrCodeLocked):
		httpx.WriteError(w, r, http.StatusBadRequest, "code_locked", "too many wrong attempts, request a new code")
	case errors.As(err, &sse): // already logged (once per interval) by the session store
		w.Header().Set("Retry-After", "5")
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "session_store_unavailable", "session store unavailable, retry shortly")
	case errors.Is(err, redis.ErrUnavailable):
		h.logger.Error("auth: code store unavailable", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
		w.Header().Set("Retry-After", "5")
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "code_store_unavailable", "code store unavailable, retry shortly")
	case errors.Is(err, ErrEmailTaken):
		httpx.WriteError(w, r, http.StatusConflict, "email_taken", "email already registered")
	case errors.Is(err, ErrInvalidCredentials):
		httpx.WriteError(w, r, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
	case errors.Is(err, ErrUnauthenticated):
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthenticated", "sign in required")
	case errors.As(err, &rl):
		secs := int((rl.RetryAfter + time.Second - 1) / time.Second)
		w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
		httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limited", "too many attempts")
	case errors.Is(err, ErrBusy):
		w.Header().Set("Retry-After", "2")
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "busy", "server busy, retry shortly")
	default:
		if !errors.Is(err, context.Canceled) {
			h.logger.Error("auth: request failed", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func cookieToken(r *http.Request) string {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func (h *Handler) setCookie(w http.ResponseWriter, s Session) {
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
