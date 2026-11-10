// Package seed builds deterministic fixture data using set-based PostgreSQL batches.
package seed

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const DemoPassword = "SiteTrack-demo-2026!"

var ErrNotEmpty = errors.New("database already contains data; pass --reset to replace it")

type Options struct {
	Preset   string
	Anchor   time.Time
	Reset    bool
	Progress io.Writer
}

type preset struct {
	users, projects, maxMembers, invitations, tasks, largestProject, edges, materials, deliveries, lines int64
}

var presets = map[string]preset{
	"small": {users: 80, projects: 8, maxMembers: 12, invitations: 64, tasks: 160, largestProject: 40, edges: 320, materials: 120, deliveries: 96, lines: 336},
	"full":  {users: 5000, projects: 2000, maxMembers: 50, invitations: 50000, tasks: 500000, largestProject: 3000, edges: 1000000, materials: 20000, deliveries: 400000, lines: 3000000},
}

//go:embed sql/*.sql
var batches embed.FS

func Populate(ctx context.Context, db *sql.DB, options Options) error {
	p, ok := presets[options.Preset]
	if !ok {
		return fmt.Errorf("unknown seed preset %q (use small or full)", options.Preset)
	}
	if options.Anchor.IsZero() {
		return errors.New("a seed anchor date is required")
	}
	log := options.Progress
	if log == nil {
		log = io.Discard
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Block competing writers and other seeds for the entire check/load transaction.
	if _, err := tx.ExecContext(ctx, "LOCK TABLE users, projects, materials, deliveries IN ACCESS EXCLUSIVE MODE"); err != nil {
		return err
	}
	var hasData bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users) OR EXISTS (SELECT 1 FROM projects)
  OR EXISTS (SELECT 1 FROM materials) OR EXISTS (SELECT 1 FROM deliveries)`).Scan(&hasData); err != nil {
		return err
	}
	if hasData && !options.Reset {
		return ErrNotEmpty
	}
	if options.Reset {
		if _, err := tx.ExecContext(ctx, `TRUNCATE users,sessions,projects,project_members,invitations,tasks,
   task_dependencies,stores,materials,stock,deliveries,delivery_lines RESTART IDENTITY;
   INSERT INTO stores (project_id) VALUES (NULL)`); err != nil {
			return err
		}
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(DemoPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL work_mem = '32MB';
 CREATE TEMP TABLE seed_config (
  user_count bigint, project_count bigint, max_members bigint, invitation_count bigint,
  task_count bigint, largest_project bigint, dependency_count bigint, material_count bigint,
  delivery_count bigint, line_count bigint, anchor date, password_hash text
 ) ON COMMIT DROP`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO seed_config VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::date,$12)`,
		p.users, p.projects, p.maxMembers, p.invitations, p.tasks, p.largestProject, p.edges, p.materials, p.deliveries, p.lines,
		options.Anchor.Format(time.DateOnly), string(passwordHash)); err != nil {
		return err
	}

	steps := []struct {
		name     string
		expected int64
	}{
		{"users", p.users}, {"project_rosters", 0}, {"projects", p.projects}, {"members", 0}, {"stores", p.projects},
		{"invitations", p.invitations}, {"tasks", p.tasks}, {"dependencies", p.edges}, {"materials", p.materials},
		{"deliveries", p.deliveries}, {"delivery_lines", p.lines}, {"stock", 0}, {"sequences", 0},
	}
	for _, step := range steps {
		start := time.Now()
		script, err := batches.ReadFile("sql/" + step.name + ".sql")
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, string(script))
		if err != nil {
			return fmt.Errorf("seed %s: %w", step.name, err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if step.expected != 0 && n != step.expected {
			return fmt.Errorf("seed %s inserted %d, expected %d", step.name, n, step.expected)
		}
		fmt.Fprintf(log, "%-17s %9d rows   %s\n", step.name, n, time.Since(start).Round(time.Millisecond))
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Fprintln(log, "Seed committed.")
	return nil
}
