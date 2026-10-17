package metrics

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	goredis "github.com/redis/go-redis/v9"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestPoolsGaugesAndRateLimitAreExported(t *testing.T) {
	m := New()
	db, err := sql.Open("mysql", "u:p@tcp(127.0.0.1:1)/x") // lazy: never connects
	if err != nil {
		t.Fatal(err)
	}
	m.DB(db, "smemories")
	rdb := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"})
	defer func() { _ = rdb.Close() }()
	m.Redis(rdb)
	used := 3.0
	m.Gauge("media_processing_slots_in_use", "test", func() float64 { return used })
	m.RateLimited("login_ip")
	m.RateLimited("login_ip")
	m.ObserveRequest("GET", "GET /x", 200, time.Millisecond)

	out := scrape(t, m)
	for _, want := range []string{
		`go_sql_max_open_connections{db_name="smemories"}`,
		`smem_redis_pool_connections 0`,
		`smem_redis_pool_timeouts_total 0`,
		`smem_media_processing_slots_in_use 3`,
		`smem_ratelimit_rejected_total{limiter="login_ip"} 2`,
		`go_goroutines`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
}

func TestStatusClass(t *testing.T) {
	for code, want := range map[int]string{200: "2xx", 204: "2xx", 302: "3xx", 404: "4xx", 503: "5xx", 101: "1xx"} {
		if got := statusClass(code); got != want {
			t.Errorf("statusClass(%d) = %s, want %s", code, got, want)
		}
	}
}

// T-021: the failed-email counter is exported with the class label.
func TestMailSendFailuresAreExported(t *testing.T) {
	m := New()
	m.MailSendFailed("unavailable")
	m.MailSendFailed("unavailable")
	m.MailSendFailed("rejected")
	m.MailSendFailed("config")
	m.MailSendFailed("shutdown")
	out := scrape(t, m)
	for _, want := range []string{`smem_mail_send_failures_total{class="unavailable"} 2`, `smem_mail_send_failures_total{class="rejected"} 1`, `smem_mail_send_failures_total{class="config"} 1`, `smem_mail_send_failures_total{class="shutdown"} 1`} {
		if !strings.Contains(out, want) {
			t.Errorf("scrape lacks %q", want)
		}
	}
}
