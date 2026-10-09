package notes

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/ratelimit"
)

// takeSubmit counts per IP (hour and day) and per collection (hour); a refusal consumes nothing.
func TestTakeSubmit(t *testing.T) {
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	h := &Handler{subIPHr: ratelimit.NewMemory(submitPerIPHour, time.Hour, clock), subIPDay: ratelimit.NewMemory(submitPerIPDay, 24*time.Hour, clock),
		subColl: ratelimit.NewMemory(submitPerCollectionHour, time.Hour, clock)}
	n := 0
	take := func(ip string) (bool, time.Duration) {
		n++
		return h.takeSubmit(context.Background(), ip, fmt.Sprint("collection", n))
	}

	for range 3 { // three hours of 100: the daily 300 is used up
		for range submitPerIPHour {
			if ok, _ := take("ip"); !ok {
				t.Fatal("refused under the limits")
			}
		}
		if ok, wait := take("ip"); ok || wait <= 0 {
			t.Fatalf("hourly limit not enforced: %v %v", ok, wait)
		}
		now = now.Add(time.Hour + time.Second)
	}
	if ok, wait := take("ip"); ok || wait < 20*time.Hour {
		t.Fatalf("daily limit not enforced: %v %v", ok, wait)
	}
	if ok, _ := take("another"); !ok {
		t.Fatal("the limit is per IP")
	}
	// one collection: 60 an hour, whoever sends
	for range submitPerCollectionHour {
		if ok, _ := h.takeSubmit(context.Background(), "a"+fmt.Sprint(n), "same"); !ok {
			t.Fatal("refused under the collection limit")
		}
		n++
	}
	if ok, _ := h.takeSubmit(context.Background(), "fresh", "same"); ok {
		t.Fatal("collection limit not enforced")
	}
	if ok, _ := h.takeSubmit(context.Background(), "fresh", "elsewhere"); !ok { // and the refused request used none of fresh's allowance
		t.Fatal("a refusal consumed the IP allowance")
	}
}
