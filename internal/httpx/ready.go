package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
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

// drainDep is the Name of the Draining dep; its failure is logged once, not per probe (a load balancer probes every few seconds).
const drainDep = "shutdown"

// Draining is a Dep that fails once ctx is cancelled (the shutdown signal), so /readyz answers 503 for the whole drain
// and the load balancer takes the task out of rotation before the listener closes.
func Draining(ctx context.Context) Dep {
	return Dep{drainDep, func(context.Context) error { return ctx.Err() }}
}

// ReadyTimeout bounds all the pings behind one /readyz request together.
const ReadyTimeout = time.Second

// ReadyCacheTTL is how long a /readyz check result is reused, so probing /readyz cannot be used to hammer the database and Redis.
const ReadyCacheTTL = 2 * time.Second

// Ready registers GET /readyz: 200 {"status":"ready"} while p and every dep answer within ReadyTimeout,
// otherwise 503 ERROR_NOT_READY. The driver error is logged, never put in the body.
// The result of the database and dependency pings is cached for ReadyCacheTTL and concurrent probes share one check;
// the Draining dep is evaluated on every request, before the cache, so a drain is visible at once.
func Ready(p Pinger, logger *slog.Logger, deps ...Dep) func(*http.ServeMux) {
	return ready(time.Now, p, logger, deps...)
}

func ready(now func() time.Time, p Pinger, logger *slog.Logger, deps ...Dep) func(*http.ServeMux) {
	var drain, slow []Dep
	for _, d := range deps {
		if d.Name == drainDep {
			drain = append(drain, d)
		} else {
			slow = append(slow, d)
		}
	}
	slow = append([]Dep{{"database", p.PingContext}}, slow...)

	var (
		drainLogged atomic.Bool
		mu          sync.Mutex // held during a check, so concurrent probes wait for it and share its result
		checkedAt   time.Time
		cachedOK    bool
	)
	check := func(r *http.Request) bool {
		mu.Lock()
		defer mu.Unlock()
		if !checkedAt.IsZero() && now().Sub(checkedAt) < ReadyCacheTTL {
			return cachedOK
		}
		// The check outlives a client that hangs up, otherwise its cancellation would be cached as "not ready".
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), ReadyTimeout)
		defer cancel()
		cachedOK = true
		for _, d := range slow {
			if err := d.Ping(ctx); err != nil {
				logger.Warn("readyz: "+d.Name+" ping failed", "request_id", RequestIDFrom(r.Context()), "error", err)
				cachedOK = false
				break
			}
		}
		checkedAt = now()
		return cachedOK
	}
	return func(mux *http.ServeMux) {
		mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
			for _, d := range drain {
				if err := d.Ping(r.Context()); err != nil {
					if drainLogged.CompareAndSwap(false, true) {
						logger.Warn("readyz: "+d.Name+" ping failed", "request_id", RequestIDFrom(r.Context()), "error", err)
					}
					WriteError(w, r, http.StatusServiceUnavailable, "ERROR_NOT_READY", "dependency not ready")
					return
				}
			}
			if !check(r) {
				WriteError(w, r, http.StatusServiceUnavailable, "ERROR_NOT_READY", "dependency not ready")
				return
			}
			WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		})
	}
}
