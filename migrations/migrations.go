package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
)

//go:embed *.up.sql
var files embed.FS

func Apply(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// A transaction-scoped database lock lets multiple API processes bootstrap safely.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(75624032814450413)"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
  name text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
 )`); err != nil {
		return err
	}
	names, err := fs.Glob(files, "*.up.sql")
	if err != nil {
		return err
	}
	for _, name := range names {
		var applied bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)", name).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		script, err := files.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(script)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (name) VALUES ($1)", name); err != nil {
			return err
		}
	}
	return tx.Commit()
}
