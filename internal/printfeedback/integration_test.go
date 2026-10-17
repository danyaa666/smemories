//go:build integration

package printfeedback

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/ratelimit"
)

// Parallel answers lose no count (one INSERT ... ON DUPLICATE KEY UPDATE), and each (family, verdict) is one row.
func TestCountersInMySQL(t *testing.T) {
	d := dbtest.New(t)
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	svc := NewService(NewStore(d), ratelimit.NewMemoryFactory(nil), func() time.Time { return now })
	svc.ip = ratelimit.NewMemory(1000, time.Hour, nil)
	h := router(svc)

	const n = 40
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := `{"verdict":"yes","browser_family":"safari_ios"}`
			if i%4 == 0 {
				body = `{"verdict":"no","browser_family":"safari_ios"}`
			}
			if rec := do(h, "9.9.9.9", "application/json", body); rec.Code != http.StatusOK {
				t.Errorf("%d %s", rec.Code, rec.Body.String())
			}
		}()
	}
	wg.Wait()

	rows, err := d.QueryContext(context.Background(), `SELECT browser_family, verdict, hits, created_at, updated_at FROM print_feedback_tab ORDER BY verdict`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	got := map[string]int{}
	for rows.Next() {
		var fam, verdict string
		var hits int
		var created, updated int64
		if err := rows.Scan(&fam, &verdict, &hits, &created, &updated); err != nil {
			t.Fatal(err)
		}
		if created != now.UnixMilli() || updated != now.UnixMilli() {
			t.Errorf("times %d %d", created, updated)
		}
		got[fam+"/"+verdict] = hits
	}
	if got["safari_ios/no"] != n/4 || got["safari_ios/yes"] != n-n/4 || len(got) != 2 {
		t.Fatalf("counters %v", got)
	}
}

// The table holds counters only: no column could carry a user, an address or a user agent.
func TestTableHasNoPersonalData(t *testing.T) {
	d := dbtest.New(t)
	rows, err := d.Query(`SELECT column_name FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'print_feedback_tab'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, c)
	}
	if got, want := strings.Join(slices.Sorted(slices.Values(cols)), ","), "browser_family,created_at,hits,id,updated_at,verdict"; got != want {
		t.Fatalf("columns %s, want %s", got, want)
	}
}
