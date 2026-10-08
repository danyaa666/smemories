package auth

import (
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/danyaa666/smemories/internal/httpx"
)

// CookieName is the session cookie.
const CookieName = "smem_session"

// Guard protects every state-changing request (POST, PUT, PATCH, DELETE):
//   - one that carries the session cookie must come from an allowed origin (Origin, or
//     Referer when Origin is absent), else 403 csrf_origin_mismatch;
//   - one with a body must be application/json, else 415 unsupported_media_type.
//
// allowed holds lower-case origins such as "https://app.example.com" (config
// SMEM_ALLOWED_ORIGINS). Wrap every mutating route with it.
func Guard(allowed []string, next http.Handler) http.Handler {
	return guard(allowed, "application/json", next)
}

// GuardMultipart is Guard for upload routes: the body must be multipart/form-data instead of JSON.
func GuardMultipart(allowed []string, next http.Handler) http.Handler {
	return guard(allowed, "multipart/form-data", next)
}

func guard(allowed []string, bodyType string, next http.Handler) http.Handler {
	set := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		set[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if _, err := r.Cookie(CookieName); err == nil && !set[requestOrigin(r)] {
				httpx.WriteError(w, r, http.StatusForbidden, "csrf_origin_mismatch", "origin not allowed")
				return
			}
			if r.ContentLength != 0 && !hasMediaType(r.Header.Get("Content-Type"), bodyType) {
				httpx.WriteError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be "+bodyType)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requestOrigin returns "scheme://host" from Origin, else from Referer, lower-cased;
// "" when neither is usable (including the opaque Origin "null").
func requestOrigin(r *http.Request) string {
	raw := r.Header.Get("Origin")
	if raw == "" {
		raw = r.Header.Get("Referer")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

func hasMediaType(contentType, want string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	return err == nil && mt == want
}
