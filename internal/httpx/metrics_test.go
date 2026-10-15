package httpx

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/metrics"
)

// TestMetricsRouteLabelsArePatterns (T-073 AC4): the scrape carries route patterns, never raw paths, and unknown
// paths and methods fold into bounded labels.
func TestMetricsRouteLabelsArePatterns(t *testing.T) {
	mt := metrics.New()
	h := NewRouterWith(slog.New(slog.DiscardHandler), RouterConfig{Metrics: mt}, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/c/{token}", func(w http.ResponseWriter, r *http.Request) { WriteJSON(w, 200, map[string]string{}) })
		mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("boom") })
	})
	for _, c := range [][2]string{
		{"GET", "/api/c/secret-token-1"}, {"GET", "/api/c/secret-token-2"}, {"GET", "/scanner/wp-login.php"},
		{"FOOBAR", "/api/c/x"}, {"GET", "/boom"},
	} {
		do(h, c[0], c[1], nil)
	}

	srv := httptest.NewServer(mt.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	out := string(b)

	for _, want := range []string{
		`smem_http_request_duration_seconds_count{code_class="2xx",method="GET",route="GET /api/c/{token}"} 2`,
		`smem_http_request_duration_seconds_count{code_class="4xx",method="GET",route="unmatched"} 1`,
		`smem_http_request_duration_seconds_count{code_class="4xx",method="OTHER",route="unmatched"} 1`,
		`smem_http_request_duration_seconds_count{code_class="5xx",method="GET",route="GET /boom"} 1`,
		`smem_http_requests_in_flight 0`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
	for _, bad := range []string{"secret-token", "wp-login", "FOOBAR"} {
		if strings.Contains(out, bad) {
			t.Errorf("scrape leaks %q", bad)
		}
	}
}

// The public mux must not serve /metrics.
func TestPublicRouterHasNoMetricsEndpoint(t *testing.T) {
	h := NewRouterWith(slog.New(slog.DiscardHandler), RouterConfig{Metrics: metrics.New()})
	if rec := do(h, "GET", "/metrics", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("GET /metrics on the public router = %d, want 404", rec.Code)
	}
}
