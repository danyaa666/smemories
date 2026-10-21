// Package metrics holds the process's Prometheus registry and the few metrics the API exports (T-073, docs/observability.md).
// Labels are always from a small fixed set: a route pattern, a known HTTP method, a status class, a limiter name.
// Never add a label that carries a user id, email, token, IP or raw path.
package metrics

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	goredis "github.com/redis/go-redis/v9"
)

// Unmatched is the route label of a request that matched no route (404/405), so scanners cannot create labels.
const Unmatched = "unmatched"

const namespace = "smem"

// Metrics owns a private registry (no global state) and the instruments that need to be called from code.
type Metrics struct {
	reg         *prometheus.Registry
	duration    *prometheus.HistogramVec
	inFlight    prometheus.Gauge
	rateLimited *prometheus.CounterVec
	mailFailed  *prometheus.CounterVec
	publicMedia *prometheus.CounterVec
}

// New returns a registry with the Go runtime and process collectors and the HTTP and rate-limit instruments.
func New() *Metrics {
	m := &Metrics{
		reg: prometheus.NewRegistry(),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: "http", Name: "request_duration_seconds",
			Help:    "Request duration by method, matched route pattern and status class; its _count is the request count.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
		}, []string{"method", "route", "code_class"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "http", Name: "requests_in_flight", Help: "Requests being served now.",
		}),
		rateLimited: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: "ratelimit", Name: "rejected_total", Help: "Requests refused by a rate limiter, by limiter name.",
		}, []string{"limiter"}),
		mailFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "mail_send_failures_total", Help: "Emails that could not be sent after the retries, by class (unavailable, rejected, config, shutdown, other).",
		}, []string{"class"}),
		publicMedia: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "public_media_requests_total", Help: "Public photo link requests by result (ok, not_found, rate_limited). No ids or tokens.",
		}, []string{"result"}),
	}
	for _, r := range []string{"ok", "not_found", "rate_limited"} {
		m.publicMedia.WithLabelValues(r) // present at 0 from the first scrape
	}
	m.reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.duration, m.inFlight, m.rateLimited, m.mailFailed, m.publicMedia)
	return m
}

// Handler serves the text exposition. Mount it only on the internal listener.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}

// InFlight adds delta (+1 / -1) to the in-flight gauge.
func (m *Metrics) InFlight(delta float64) { m.inFlight.Add(delta) }

// ObserveRequest records one finished request. route is the mux pattern ("" = no match); the method is folded
// into a known set because a client can send any string.
func (m *Metrics) ObserveRequest(method, route string, status int, d time.Duration) {
	if route == "" {
		route = Unmatched
	}
	m.duration.WithLabelValues(knownMethod(method), route, statusClass(status)).Observe(d.Seconds())
}

// RateLimited counts one rejection by the limiter called name (a constant chosen in code).
func (m *Metrics) RateLimited(name string) { m.rateLimited.WithLabelValues(name).Inc() }

// MailSendFailed counts one email given up on; class is one of mailer.ClassUnavailable, ClassRejected, ClassOther.
func (m *Metrics) MailSendFailed(class string) { m.mailFailed.WithLabelValues(class).Inc() }

// PublicMedia counts one public photo link request; result is a constant chosen in code (media.PublicOK, PublicNotFound,
// PublicRateLimited). Any other value is counted as not_found so the label set stays closed.
func (m *Metrics) PublicMedia(result string) {
	switch result {
	case "ok", "not_found", "rate_limited":
	default:
		result = "not_found"
	}
	m.publicMedia.WithLabelValues(result).Inc()
}

// Gauge exports fn as a gauge read at scrape time.
func (m *Metrics) Gauge(name, help string, fn func() float64) {
	m.reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Namespace: namespace, Name: name, Help: help}, fn))
}

// DB exports the sql.DB pool statistics as go_sql_* metrics.
func (m *Metrics) DB(db *sql.DB, name string) {
	m.reg.MustRegister(collectors.NewDBStatsCollector(db, name))
}

// Redis exports the go-redis pool statistics as smem_redis_pool_*.
func (m *Metrics) Redis(rdb *goredis.Client) {
	g := func(name, help string, f func(*goredis.PoolStats) float64) {
		m.Gauge("redis_pool_"+name, help, func() float64 { return f(rdb.PoolStats()) })
	}
	c := func(name, help string, f func(*goredis.PoolStats) float64) {
		m.reg.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{Namespace: namespace, Name: "redis_pool_" + name, Help: help},
			func() float64 { return f(rdb.PoolStats()) }))
	}
	g("connections", "Connections in the Redis pool.", func(s *goredis.PoolStats) float64 { return float64(s.TotalConns) })
	g("idle_connections", "Idle connections in the Redis pool.", func(s *goredis.PoolStats) float64 { return float64(s.IdleConns) })
	c("hits_total", "Times a free connection was found.", func(s *goredis.PoolStats) float64 { return float64(s.Hits) })
	c("misses_total", "Times no free connection was found.", func(s *goredis.PoolStats) float64 { return float64(s.Misses) })
	c("timeouts_total", "Times waiting for a connection timed out.", func(s *goredis.PoolStats) float64 { return float64(s.Timeouts) })
}

func knownMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return m
	}
	return "OTHER"
}

func statusClass(code int) string {
	switch {
	case code >= http.StatusInternalServerError:
		return "5xx"
	case code >= http.StatusBadRequest:
		return "4xx"
	case code >= http.StatusMultipleChoices:
		return "3xx"
	case code >= http.StatusOK:
		return "2xx"
	}
	return "1xx"
}
