package db

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/danyaa666/smemories/internal/config"
)

func TestNormalizeForcesSessionSettings(t *testing.T) {
	mc, err := Normalize("u:p@tcp(127.0.0.1:3306)/db?parseTime=false&loc=Asia%2FSaigon&collation=latin1_swedish_ci&charset=latin1")
	if err != nil {
		t.Fatal(err)
	}
	if !mc.ParseTime || mc.Loc != time.UTC || mc.Collation != "utf8mb4_0900_ai_ci" {
		t.Fatalf("settings not forced: %+v", mc)
	}
	if _, ok := mc.Params["charset"]; ok {
		t.Fatal("charset param must be dropped")
	}
	if mc.User != "u" || mc.Passwd != "p" || mc.Addr != "127.0.0.1:3306" || mc.DBName != "db" {
		t.Fatalf("connection parts lost: %+v", mc)
	}
}

func TestNormalizeErrorHidesDSN(t *testing.T) {
	_, err := Normalize("user:hunter2@tcp(broken")
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("want an error without the password, got %v", err)
	}
}

func TestNewAppliesPoolSettings(t *testing.T) {
	mc, _ := Normalize("u:p@tcp(127.0.0.1:3306)/db")
	d, err := New(mc, config.Config{DBMaxOpen: 3, DBMaxIdle: 1, DBConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if got := d.Stats().MaxOpenConnections; got != 3 {
		t.Fatalf("MaxOpenConnections = %d, want 3", got)
	}
}

func TestOpenGivesUpWhenUnreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0") // grab a free port, then close it so nothing listens
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	start := time.Now()
	cfg := config.Config{DBDSN: "u:hunter2@tcp(" + addr + ")/db", DBMaxOpen: 1, DBMaxIdle: 1, DBConnMaxLifetime: time.Minute}
	d, err := Open(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		_ = d.Close()
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "hunter2") || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("Open did not honour the context deadline: %v", time.Since(start))
	}
}

func TestOpenRejectsBadDSN(t *testing.T) {
	_, err := Open(context.Background(), config.Config{DBDSN: "nope"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "SMEM_DB_DSN") {
		t.Fatalf("want error naming SMEM_DB_DSN, got %v", err)
	}
}

func TestOpenRejectsEmptyDSN(t *testing.T) {
	_, err := Open(context.Background(), config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "SMEM_DB_DSN") || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("want an empty-DSN error naming SMEM_DB_DSN, got %v", err)
	}
}

func TestWaitReadyFailsFastOnPermanentErrors(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, num := range []uint16{1045, 1049} {
		calls := 0
		ping := func(context.Context) error {
			calls++
			return &mysql.MySQLError{Number: num, Message: "refused"}
		}
		start := time.Now()
		err := waitReady(context.Background(), "db:3306", "smem", ping, log)
		if err == nil || calls != 1 || time.Since(start) > time.Second {
			t.Fatalf("error %d: want one attempt and an immediate error, got calls=%d err=%v after %v", num, calls, err, time.Since(start))
		}
		if !strings.Contains(err.Error(), "db:3306") || !strings.Contains(err.Error(), `"smem"`) {
			t.Errorf("error %d must name address and user: %v", num, err)
		}
	}
}

func TestWaitReadyRetriesTransientErrors(t *testing.T) {
	calls := 0
	ping := func(context.Context) error {
		calls++
		if calls < 3 {
			return &mysql.MySQLError{Number: 1040, Message: "too many connections"}
		}
		return nil
	}
	if err := waitReady(context.Background(), "db:3306", "smem", ping, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil || calls != 3 {
		t.Fatalf("want success on the 3rd attempt, got calls=%d err=%v", calls, err)
	}
}
