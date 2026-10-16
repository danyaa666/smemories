package dbtest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"os"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/danyaa666/smemories/internal/db"
)

// Spy opens a second pool on the test database of d and calls record with the text of every statement that goes through it
// (BEGIN and COMMIT excluded). The wrapper hides the driver's optional interfaces, so database/sql always prepares a
// statement first and nothing slips past unseen. record may be called from several goroutines.
func Spy(t testing.TB, d *sql.DB, record func(query string)) *sql.DB {
	t.Helper()
	var name string
	if err := d.QueryRow(`SELECT DATABASE()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	dsn := os.Getenv("SMEM_TEST_DB_DSN")
	if dsn == "" {
		dsn = DefaultDSN
	}
	mc, err := db.Normalize(dsn)
	if err != nil {
		t.Fatal(err)
	}
	mc.DBName = name
	inner, err := mysql.NewConnector(mc)
	if err != nil {
		t.Fatal(err)
	}
	c := sql.OpenDB(spyConnector{inner, record})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

type spyConnector struct {
	driver.Connector
	record func(string)
}

type spyConn struct {
	driver.Conn
	record func(string)
}

type spyStmt struct {
	driver.Stmt
	record func(string)
	query  string
}

func (c spyConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	return spyConn{conn, c.record}, err
}

func (c spyConn) PrepareContext(ctx context.Context, q string) (driver.Stmt, error) {
	st, err := c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, q)
	return spyStmt{st, c.record, q}, err
}

func (c spyConn) BeginTx(ctx context.Context, o driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, o)
}

func (c spyConn) CheckNamedValue(nv *driver.NamedValue) error {
	return c.Conn.(driver.NamedValueChecker).CheckNamedValue(nv)
}

func (s spyStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	s.record(s.query)
	return s.Stmt.(driver.StmtQueryContext).QueryContext(ctx, args)
}

func (s spyStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	s.record(s.query)
	return s.Stmt.(driver.StmtExecContext).ExecContext(ctx, args)
}

func (s spyStmt) CheckNamedValue(nv *driver.NamedValue) error {
	return s.Stmt.(driver.NamedValueChecker).CheckNamedValue(nv)
}
