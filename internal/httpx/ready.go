package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Pinger is what /readyz needs from the database (*sql.DB satisfies it).
type Pinger interface {
	PingContext(ctx context.Context) error
}

// ReadyTimeout bounds the ping behind one /readyz request.
const ReadyTimeout = time.Second

// Ready registers GET /readyz: 200 {"status":"ready"} while p answers within ReadyTimeout,
// otherwise 503 not_ready. The driver error is logged, never put in the body.
func Ready(p Pinger, logger *slog.Logger) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), ReadyTimeout)
			defer cancel()
			if err := p.PingContext(ctx); err != nil {
				logger.Warn("readyz: database ping failed", "request_id", RequestIDFrom(r.Context()), "error", err)
				WriteError(w, r, http.StatusServiceUnavailable, "not_ready", "dependency not ready")
				return
			}
			WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		})
	}
}
