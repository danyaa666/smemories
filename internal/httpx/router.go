package httpx

import (
	"cmp"
	"log/slog"
	"net/http"
	"time"
)

// NewRouter returns the root handler: the middleware chain around a ServeMux that
// serves /healthz plus whatever the domain packages register in routes. Unknown paths
// and wrong methods answer with the shared error envelope.
//
// Chain (outermost first): request id, access log, security headers, panic recovery,
// request timeout, body cap, client IP, mux.
func NewRouter(logger *slog.Logger, routes ...func(*http.ServeMux)) http.Handler {
	return NewRouterWith(logger, RouterConfig{}, routes...)
}

// RouterConfig tunes the chain; the zero value means DefaultRequestTimeout and no trusted proxy.
type RouterConfig struct {
	RequestTimeout time.Duration
	TrustProxy     bool // WithClientIP reads the last X-Forwarded-For hop
}

// NewRouterWith is NewRouter with an explicit RouterConfig.
func NewRouterWith(logger *slog.Logger, cfg RouterConfig, routes ...func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	for _, register := range routes {
		register(mux)
	}

	h := WithClientIP(cfg.TrustProxy)(envelopeNotFound(mux))
	h = BodyLimit(DefaultMaxBody, h)
	h = Timeout(cmp.Or(cfg.RequestTimeout, DefaultRequestTimeout))(h)
	h = Recover(logger, h)
	h = SecurityHeaders(h)
	h = AccessLog(logger, h)
	return RequestID(withLogger(logger, h))
}

// envelopeNotFound dispatches to mux, but replaces the mux's plain-text 404 and 405
// with envelope responses (keeping the Allow header the mux computed for 405).
func envelopeNotFound(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, pattern := mux.Handler(r)
		setRoute(r.Context(), pattern)
		if pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		// No route matched: run the mux's own fallback against a probe to learn
		// whether it is a 404, a 405 (+Allow) or a redirect.
		p := &probe{header: http.Header{}}
		h.ServeHTTP(p, r)
		switch p.code {
		case http.StatusNotFound:
			WriteError(w, r, http.StatusNotFound, "not_found", "route not found")
		case http.StatusMethodNotAllowed:
			w.Header().Set("Allow", p.header.Get("Allow"))
			WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		default:
			for k, v := range p.header {
				w.Header()[k] = v
			}
			w.WriteHeader(p.code)
		}
	})
}

type probe struct {
	header http.Header
	code   int
}

func (p *probe) Header() http.Header         { return p.header }
func (p *probe) Write(b []byte) (int, error) { return len(b), nil }
func (p *probe) WriteHeader(code int)        { p.code = code }
