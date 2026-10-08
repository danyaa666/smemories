package auth

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danyaa666/smemories/internal/httpx"
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
	mux.Handle("POST /v1/auth/verify-email", guard(h.verifyEmail))
	mux.Handle("POST /v1/auth/verify-email/resend", Guard(h.cfg.AllowedOrigins, h.RequireUser(http.HandlerFunc(h.resendVerification))))
	mux.Handle("POST /v1/auth/forgot-password", guard(h.forgotPassword))
	mux.Handle("POST /v1/auth/reset-password", guard(h.resetPassword))
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

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	u, sess, err := h.svc.Register(r.Context(), h.clientIP(r), r.UserAgent(), in.Email, in.Password, in.DisplayName)
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
		Token string `json:"token"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.VerifyEmail(r.Context(), in.Token); err != nil {
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
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.ResetPassword(r.Context(), in.Token, in.Password); err != nil {
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
	switch {
	case errors.As(err, &ve):
		httpx.WriteError(w, r, http.StatusBadRequest, ve.Code, "invalid input: "+ve.Code)
	case errors.Is(err, ErrInvalidToken):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_token", "invalid, expired or used token")
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

// clientIP is RemoteAddr's host, or with TrustProxy the last X-Forwarded-For hop (the one
// our own proxy appended; earlier hops are client-controlled).
func (h *Handler) clientIP(r *http.Request) string {
	if h.cfg.TrustProxy {
		if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
			hops := strings.Split(xff[len(xff)-1], ",")
			if ip := strings.TrimSpace(hops[len(hops)-1]); net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
