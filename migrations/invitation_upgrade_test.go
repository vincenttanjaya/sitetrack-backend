package migrations_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/vincenttanjaya/sitetrack-backend/internal/testdb"
	"github.com/vincenttanjaya/sitetrack-backend/migrations"
	"gorm.io/gorm"
)

const initial = "001_initial.up.sql"
const inventoryHistory = "002_inventory_history.up.sql"

func upgradeFixture(t *testing.T, through string) (*gorm.DB, *sql.DB) {
	t.Helper()
	db := testdb.OpenUnmigrated(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("CREATE TABLE schema_migrations(name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{initial, inventoryHistory} {
		script, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(string(script)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations (name) VALUES ($1)", name); err != nil {
			t.Fatal(err)
		}
		if name == through {
			break
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return db, pool
}

func TestUpgradeBackfillsInvitationLabelsOnlyForAuthorizedRecipients(t *testing.T) {
	_, pool := upgradeFixture(t, inventoryHistory)
	_, err := pool.Exec(`
	 INSERT INTO users (id,name,email,password_hash,role) VALUES
	  (1,'Owner','owner@example.test','unused','user'),
	  (2,'Former','former@example.test','unused','user'),
	  (3,'Rejected','rejected@example.test','unused','company_store_manager'),
	  (4,'Pending','pending@example.test','unused','user'),
	  (5,'Member','member@example.test','unused','user'),
	  (6,'Global','global@example.test','unused','superadmin'),
	  (7,'Withdrawn','withdrawn@example.test','unused','user');
	 INSERT INTO projects (number,name,start_date,end_date) VALUES (1,'Migration-time label','2026-01-01','2026-12-31');
	 INSERT INTO project_members (project_id,user_id,is_admin) VALUES ('PRJ-0001',1,true),('PRJ-0001',5,false);
	 INSERT INTO invitations (project_id,sender_id,recipient_id,status,responded_at) VALUES
	  ('PRJ-0001',1,2,'accepted',now()),
	  ('PRJ-0001',1,3,'rejected',now()),
	  ('PRJ-0001',1,4,'pending',NULL),
	  ('PRJ-0001',1,5,'accepted',now()),
	  ('PRJ-0001',1,6,'rejected',now()),
	  ('PRJ-0001',1,7,'withdrawn',now());`)
	if err != nil {
		t.Fatal(err)
	}
	apply := func() {
		t.Helper()
		if err := migrations.Apply(context.Background(), pool); err != nil {
			t.Fatal(err)
		}
	}
	apply()
	if _, err := pool.Exec("UPDATE projects SET name='Later label'"); err != nil {
		t.Fatal(err)
	}
	apply()
	for i, want := range []string{"PRJ-0001", "PRJ-0001", "Migration-time label", "Migration-time label", "Migration-time label", "PRJ-0001"} {
		var got string
		err := pool.QueryRow("SELECT project_name FROM invitations WHERE recipient_id=$1", i+2).Scan(&got)
		if err != nil || got != want {
			t.Errorf("recipient %d: got %q, want %q (error: %v)", i+2, got, want, err)
		}
	}
}
