package db

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/pressly/goose/v3"

	"github.com/danyaa666/smemories/migrations"
)

func provider(d *sql.DB) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectMySQL, d, migrations.FS)
}

// MigrateUp applies every pending migration.
func MigrateUp(ctx context.Context, d *sql.DB) error {
	p, err := provider(d)
	if err != nil {
		return err
	}
	_, err = p.Up(ctx)
	return err
}

// MigrateDown rolls back the most recently applied migration.
func MigrateDown(ctx context.Context, d *sql.DB) error {
	p, err := provider(d)
	if err != nil {
		return err
	}
	_, err = p.Down(ctx)
	return err
}

// MigrateStatus writes one line per migration (applied time or "pending") to w.
func MigrateStatus(ctx context.Context, d *sql.DB, w io.Writer) error {
	p, err := provider(d)
	if err != nil {
		return err
	}
	rows, err := p.Status(ctx)
	if err != nil {
		return err
	}
	for _, s := range rows {
		applied := "pending"
		if s.State == goose.StateApplied {
			applied = "applied " + s.AppliedAt.UTC().Format("2006-01-02 15:04:05Z")
		}
		_, _ = fmt.Fprintf(w, "%04d %-20s %s\n", s.Source.Version, s.Source.Path, applied)
	}
	return nil
}
