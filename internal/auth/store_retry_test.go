package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestRetryTx(t *testing.T) {
	ms := time.Millisecond
	// Window of each wait: lo..hi, doubling, upper bound capped at 200 ms.
	windows := [][2]time.Duration{{5 * ms, 25 * ms}, {10 * ms, 50 * ms}, {20 * ms, 100 * ms}, {40 * ms, 200 * ms}, {80 * ms, 200 * ms}}

	for _, n := range []uint16{mysqlDeadlock, mysqlLockTimeout, mysqlDuplicateEntry} {
		var waits []time.Duration
		calls := 0
		err := retryTx(context.Background(), func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil },
			func() error { calls++; return &mysql.MySQLError{Number: n} })
		var me *mysql.MySQLError
		if !errors.As(err, &me) || me.Number != n || calls != txAttempts || len(waits) != len(windows) {
			t.Fatalf("%d: err %v, %d calls, %d waits", n, err, calls, len(waits))
		}
		for i, d := range waits {
			if d < windows[i][0] || d >= windows[i][1] {
				t.Errorf("%d: wait %d = %v, want [%v,%v)", n, i, d, windows[i][0], windows[i][1])
			}
		}
	}

	t.Run("non-retryable returns at once", func(t *testing.T) {
		calls := 0
		boom := &mysql.MySQLError{Number: 1146}
		err := retryTx(context.Background(), func(context.Context, time.Duration) error { t.Fatal("slept"); return nil },
			func() error { calls++; return boom })
		if err != boom || calls != 1 {
			t.Fatalf("%v, %d calls", err, calls)
		}
	})

	t.Run("succeeds after a retry", func(t *testing.T) {
		calls := 0
		err := retryTx(context.Background(), func(context.Context, time.Duration) error { return nil },
			func() error {
				if calls++; calls < 3 {
					return &mysql.MySQLError{Number: mysqlDeadlock}
				}
				return nil
			})
		if err != nil || calls != 3 {
			t.Fatalf("%v, %d calls", err, calls)
		}
	})

	t.Run("cancelled context stops the retries", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		err := retryTx(ctx, sleepCtx, func() error { calls++; return &mysql.MySQLError{Number: mysqlDeadlock} })
		if calls != 1 || err == nil {
			t.Fatalf("%v, %d calls", err, calls)
		}
	})
}
