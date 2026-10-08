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

// Dep is a further dependency /readyz checks besides the database, e.g. Dep{"redis", rc.Ping}.
type Dep struct {
	Name string
	Ping func(ctx context.Context) error
}

// ReadyTimeout bounds all the pings behind one /readyz request together.
const ReadyTimeout = time.Second

// Ready registers GET /readyz: 200 {"status":"ready"} while p and every dep answer within ReadyTimeout,
// otherwise 503 not_ready. The driver error is logged, never put in the body.
func Ready(p Pinger, logger *slog.Logger, deps ...Dep) func(*http.ServeMux) {
	deps = append([]Dep{{"database", p.PingContext}}, deps...)
	return func(mux *http.ServeMux) {
		mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), ReadyTimeout)
			defer cancel()
			for _, d := range deps {
				if err := d.Ping(ctx); err != nil {
					logger.Warn("readyz: "+d.Name+" ping failed", "request_id", RequestIDFrom(r.Context()), "error", err)
					WriteError(w, r, http.StatusServiceUnavailable, "not_ready", "dependency not ready")
					return
				}
			}
			WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		})
	}
}
