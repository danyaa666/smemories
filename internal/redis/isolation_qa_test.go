//go:build integration

package redis_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/danyaa666/smemories/internal/redis/redistest"
)

// 20 parallel tests write the same logical key and counter; each must only ever see its own values, and a
// whole-prefix scan must show only its own keys (T-051 AC5, QA).
func TestHelperParallelIsolation(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprintf("p%d", i), func(t *testing.T) {
			t.Parallel()
			c := redistest.New(t)
			ctx := context.Background()
			rdb := c.Client()
			for n := 1; n <= 50; n++ {
				if err := rdb.Set(ctx, c.Key("shared"), strconv.Itoa(i), 0).Err(); err != nil {
					t.Fatal(err)
				}
				if got := rdb.Incr(ctx, c.Key("ctr")).Val(); got != int64(n) {
					t.Fatalf("counter = %d, want %d: another test touched this prefix", got, n)
				}
				if got := rdb.Get(ctx, c.Key("shared")).Val(); got != strconv.Itoa(i) {
					t.Fatalf("shared = %q, want %d", got, i)
				}
			}
			keys, err := rdb.Keys(ctx, c.Key("*")).Result()
			if err != nil || len(keys) != 2 {
				t.Fatalf("prefix scan = %v, %v; want exactly shared and ctr", keys, err)
			}
		})
	}
}

// The helper deletes every key under its prefix at cleanup (checked per prefix, so other packages sharing the server
// cannot make it flaky).
func TestHelperLeavesNothingBehind(t *testing.T) {
	c := redistest.New(t)
	ctx := context.Background()
	var pattern string
	t.Run("work", func(t *testing.T) {
		d := redistest.New(t)
		pattern = d.Key("*")
		for i := 0; i < 250; i++ {
			d.Client().Set(ctx, d.Key("k", strconv.Itoa(i)), "v", 0)
		}
	})
	if keys, err := c.Client().Keys(ctx, pattern).Result(); err != nil || len(keys) != 0 {
		t.Fatalf("%d keys left under %s: %v", len(keys), pattern, err)
	}
}
