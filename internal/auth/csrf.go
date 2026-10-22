package auth

import (
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/danyaa666/smemories/internal/httpx"
)

// CookieName is the session cookie in dev (plain http, so no Secure and no __Host- prefix).
// HostCookieName is the one used everywhere else: the __Host- prefix makes browsers accept it only
// with Secure, Path=/ and no Domain, so a sibling subdomain cannot plant or shadow it.
const (
	CookieName     = "smem_session"
	HostCookieName = "__Host-smem_session"
)

// cookieName is the session cookie's name for a handler that sets Secure cookies or not.
func cookieName(secure bool) string {
	if secure {
		return HostCookieName
	}
	return CookieName
}

// hasSessionCookie reports whether the request carries a session cookie under either name.
func hasSessionCookie(r *http.Request) bool {
	_, e1 := r.Cookie(CookieName)
	_, e2 := r.Cookie(HostCookieName)
	return e1 == nil || e2 == nil
}

// Guard protects every state-changing request (POST, PUT, PATCH, DELETE):
//   - one that carries the session cookie must come from an allowed origin (Origin, or
//     Referer when Origin is absent), else 403 ERROR_CSRF_ORIGIN_MISMATCH;
//   - one with a body must be application/json, else 415 ERROR_UNSUPPORTED_MEDIA_TYPE.
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
			if hasSessionCookie(r) && !set[requestOrigin(r)] {
				httpx.WriteError(w, r, http.StatusForbidden, "ERROR_CSRF_ORIGIN_MISMATCH", "origin not allowed")
				return
			}
			if r.ContentLength != 0 && !hasMediaType(r.Header.Get("Content-Type"), bodyType) {
				httpx.WriteError(w, r, http.StatusUnsupportedMediaType, "ERROR_UNSUPPORTED_MEDIA_TYPE", "Content-Type must be "+bodyType)
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
