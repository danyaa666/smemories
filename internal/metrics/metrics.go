// Package metrics holds the process's Prometheus registry and the few metrics the API exports (T-073, docs/observability.md).
// Labels are always from a small fixed set: a route pattern, a known HTTP method, a status class, a limiter name.
// Never add a label that carries a user id, email, token, IP or raw path.
package metrics

import (
	"database/sql"
	"net/http"
	"slices"
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
	reg          *prometheus.Registry
	duration     *prometheus.HistogramVec
	inFlight     prometheus.Gauge
	rateLimited  *prometheus.CounterVec
	mailFailed   *prometheus.CounterVec
	publicMedia  *prometheus.CounterVec
	mediaPanics  prometheus.Counter
	mediaUploads *prometheus.CounterVec
	layoutSaves  *prometheus.CounterVec

	layoutSnapshots *prometheus.CounterVec
	layoutPruned    prometheus.Counter
	classLayoutsCut prometheus.Counter
	layoutRestores  *prometheus.CounterVec
	pageTemplateBad prometheus.Counter
	tplCreated      prometheus.Counter
	tplDeleted      prometheus.Counter
	tplStripped     prometheus.Counter
	tplApplies      *prometheus.CounterVec
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
		mediaPanics: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "media_process_panics_total", Help: "Panics recovered in the image pipeline (decode, scale, encode); each answered 400 ERROR_INVALID_IMAGE.",
		}),
		mediaUploads: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "media_uploads_total", Help: "Owner image uploads (media/create) by purpose (photo, design) and result (ok, quota, invalid, busy, error). No ids or file names.",
		}, []string{"purpose", "result"}),
		layoutSaves: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "layout_saves_total", Help: "Book layout saves (book-layout/save) by result (ok, conflict, invalid, too_large, rate_limited). No ids or document content.",
		}, []string{"result"}),
		layoutSnapshots: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "layout_snapshots_total", Help: "Layout history snapshots written (T-163) by kind (auto, reset, restore). No ids or document content.",
		}, []string{"kind"}),
		layoutPruned: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "layout_snapshots_pruned_total", Help: "Layout history snapshots deleted because a book already held 30 (T-163).",
		}),
		classLayoutsCut: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "class_book_layouts_dropped_total", Help: "Member layouts left out of class-book/get because the 8 MiB cap would have been passed (T-167). Counts only.",
		}),
		layoutRestores: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "layout_restores_total", Help: "Layout restores (book-layout/restore) by result (ok, conflict, invalid, template_changed; T-163). No ids or document content.",
		}, []string{"result"}),
	}
	m.pageTemplateBad = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace, Name: "page_templates_rejected_total", Help: "Shipped page templates left out at start-up because they failed the layout validator (T-172). Should stay 0.",
	})
	m.tplCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace, Name: "page_templates_created_total", Help: "Own page templates created (book-layout/template-create; T-173). No ids or content.",
	})
	m.tplDeleted = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace, Name: "page_templates_deleted_total", Help: "Own page templates deleted (book-layout/template-delete; T-173).",
	})
	m.tplStripped = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace, Name: "page_template_items_stripped_total", Help: "Image items and picture backgrounds left out of own page templates when they were created (T-173).",
	})
	m.tplApplies = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "page_template_applies_total", Help: "Template applies (book-layout/apply-template) by source (shipped, own) and result (ok, conflict, invalid, mismatch; T-173). No ids or content.",
	}, []string{"source", "result"})
	for _, src := range []string{"shipped", "own"} {
		for _, r := range tplApplyResults {
			m.tplApplies.WithLabelValues(src, r) // present at 0 from the first scrape
		}
	}
	for _, k := range layoutSnapshotKinds {
		m.layoutSnapshots.WithLabelValues(k) // present at 0 from the first scrape
	}
	for _, r := range layoutRestoreResults {
		m.layoutRestores.WithLabelValues(r)
	}
	for _, r := range []string{"ok", "conflict", "invalid", "too_large", "rate_limited"} {
		m.layoutSaves.WithLabelValues(r) // present at 0 from the first scrape
	}
	for _, p := range []string{"photo", "design"} {
		for _, r := range []string{"ok", "quota", "invalid", "busy", "error"} {
			m.mediaUploads.WithLabelValues(p, r) // present at 0 from the first scrape
		}
	}
	for _, r := range []string{"ok", "not_found", "rate_limited"} {
		m.publicMedia.WithLabelValues(r) // present at 0 from the first scrape
	}
	m.reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.duration, m.inFlight, m.rateLimited, m.mailFailed, m.publicMedia, m.mediaPanics, m.mediaUploads, m.layoutSaves, m.layoutSnapshots, m.layoutPruned, m.classLayoutsCut, m.layoutRestores, m.pageTemplateBad, m.tplCreated, m.tplDeleted, m.tplStripped, m.tplApplies)
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

// MediaUpload counts one owner upload; purpose and result are constants chosen in code (media.Purpose*, media.Upload* results).
// Any other value is counted as photo / error so the label set stays closed.
func (m *Metrics) MediaUpload(purpose, result string) {
	if purpose != "design" {
		purpose = "photo"
	}
	switch result {
	case "ok", "quota", "invalid", "busy":
	default:
		result = "error"
	}
	m.mediaUploads.WithLabelValues(purpose, result).Inc()
}

// LayoutSave counts one book-layout/save; result is a constant chosen in code (booklayout.Result*). Any other value is counted as invalid
// so the label set stays closed.
func (m *Metrics) LayoutSave(result string) {
	switch result {
	case "ok", "conflict", "invalid", "too_large", "rate_limited":
	default:
		result = "invalid"
	}
	m.layoutSaves.WithLabelValues(result).Inc()
}

var (
	layoutSnapshotKinds  = []string{"auto", "reset", "restore", "apply"}
	tplApplyResults      = []string{"ok", "conflict", "invalid", "mismatch"}
	layoutRestoreResults = []string{"ok", "conflict", "invalid", "template_changed"}
)

// LayoutSnapshot counts one layout history snapshot of the kind (booklayout.Kind*); another value is counted as auto, so the label set stays closed.
func (m *Metrics) LayoutSnapshot(kind string) {
	if !slices.Contains(layoutSnapshotKinds, kind) {
		kind = "auto"
	}
	m.layoutSnapshots.WithLabelValues(kind).Inc()
}

// LayoutSnapshotsPruned counts n snapshots deleted by the keep-30 rule.
func (m *Metrics) LayoutSnapshotsPruned(n int) { m.layoutPruned.Add(float64(n)) }

// ClassLayoutsDropped counts n member layouts left out of one class book by the size cap.
func (m *Metrics) ClassLayoutsDropped(n int) { m.classLayoutsCut.Add(float64(n)) }

// LayoutRestore counts one book-layout/restore by result (booklayout.Restore*); another value is counted as invalid.
func (m *Metrics) LayoutRestore(result string) {
	if !slices.Contains(layoutRestoreResults, result) {
		result = "invalid"
	}
	m.layoutRestores.WithLabelValues(result).Inc()
}

// PageTemplateCreated, PageTemplateDeleted and PageTemplateApply count the own-template routes (T-173); stripped is the number of image
// items and picture backgrounds left out of the new template. source and result are constants chosen in code (booklayout.Source*, Apply*);
// another source is counted as own and another result as invalid, so the label set stays closed.
func (m *Metrics) PageTemplateCreated(stripped int) {
	m.tplCreated.Inc()
	m.tplStripped.Add(float64(stripped))
}

func (m *Metrics) PageTemplateDeleted() { m.tplDeleted.Inc() }

func (m *Metrics) PageTemplateApply(source, result string) {
	if source != "shipped" {
		source = "own"
	}
	if !slices.Contains(tplApplyResults, result) {
		result = "invalid"
	}
	m.tplApplies.WithLabelValues(source, result).Inc()
}

// PageTemplateRejected counts one shipped page template that was left out at start-up.
func (m *Metrics) PageTemplateRejected() { m.pageTemplateBad.Inc() }

// MediaProcessPanic counts one panic recovered in the image pipeline (T-104).
func (m *Metrics) MediaProcessPanic() { m.mediaPanics.Inc() }

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
