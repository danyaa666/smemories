// Package dbtest gives integration tests a private, fully migrated MySQL database.
package dbtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/db"
)

// DefaultDSN is the docker-compose database from .env.example.
const DefaultDSN = "smemories:smem_dev_pw@tcp(127.0.0.1:3306)/smemories"

// New creates a uniquely named database (smem_test_<random>) on the server named by
// SMEM_TEST_DB_DSN (default DefaultDSN), applies all migrations, and returns a pool on it.
// The database is dropped in t.Cleanup. The account needs CREATE/DROP on smem_test_%
// (docker/mysql/init-test-grants.sh grants that). An unreachable server fails the test.
func New(t testing.TB) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SMEM_TEST_DB_DSN")
	if dsn == "" {
		dsn = DefaultDSN
	}
	mc, err := db.Normalize(dsn)
	if err != nil {
		t.Fatalf("SMEM_TEST_DB_DSN: %v", err)
	}
	cfg := config.Config{DBMaxOpen: 10, DBMaxIdle: 2, DBConnMaxLifetime: time.Minute}
	ctx := context.Background()

	admin, err := db.New(mc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := admin.PingContext(pctx); err != nil {
		t.Fatalf("test database unreachable (run `make up`?): %v", err)
	}

	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	name := "smem_test_" + hex.EncodeToString(b[:])
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE `"+name+"`"); err != nil {
			t.Errorf("drop %s: %v", name, err)
		}
	})

	tc := mc.Clone()
	tc.DBName = name
	d, err := db.New(tc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}
