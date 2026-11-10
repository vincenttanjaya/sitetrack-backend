package seed_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/adapters/postgres"
	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/seed"
	"github.com/vincenttanjaya/sitetrack-backend/internal/testdb"
)

func TestSmallSeedIsValidAndRequiresReset(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	options := seed.Options{Preset: "small", Anchor: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)}
	if err := seed.Populate(ctx, pool, options); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int64{
		"users": 80, "projects": 8, "invitations": 64, "tasks": 160, "task_dependencies": 320,
		"materials": 120, "deliveries": 96, "delivery_lines": 336,
	} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil || count != want {
			t.Fatalf("%s: got %d want %d (%v)", table, count, want, err)
		}
	}
	verify := map[string]string{
		"backward edges": `SELECT count(*) FROM task_dependencies WHERE prerequisite_number >= task_number`,
		"started has unfinished prereq": `SELECT count(*) FROM task_dependencies d
   JOIN tasks t ON t.project_id=d.project_id AND t.number=d.task_number
   JOIN tasks pre ON pre.project_id=d.project_id AND pre.number=d.prerequisite_number
   WHERE t.status <> 'not_started' AND pre.status <> 'done'`,
		"pending invitations to current members": `SELECT count(*) FROM invitations i JOIN project_members m
   ON m.project_id=i.project_id AND m.user_id=i.recipient_id WHERE i.status='pending'`,
		"invitation labels match seeded projects": `SELECT count(*) FROM invitations i JOIN projects p ON p.id=i.project_id
   WHERE i.project_name IS DISTINCT FROM p.name`,
		"reservations match outgoing active lines": `WITH active AS (
   SELECT s.id AS store_id,l.material_id,sum(l.quantity) AS quantity
   FROM deliveries d JOIN delivery_lines l ON l.delivery_id=d.id
   JOIN stores s ON s.project_id IS NOT DISTINCT FROM
    CASE WHEN d.direction='project_to_company' THEN d.project_id END
   WHERE d.status='in_transit' GROUP BY s.id,l.material_id
  )
  SELECT count(*) FROM stock k FULL JOIN active a USING (store_id,material_id)
  WHERE COALESCE(k.outbound_quantity,0) <> COALESCE(a.quantity,0)`,
		"no historical project stock shortage": `WITH movements AS (
   SELECT d.project_id,l.material_id,d.resolved_at AS moved_at,l.quantity AS delta
   FROM deliveries d JOIN delivery_lines l ON l.delivery_id=d.id
   WHERE d.direction='company_to_project' AND d.status='delivered'
   UNION ALL
   SELECT d.project_id,l.material_id,d.sent_at,-l.quantity
   FROM deliveries d JOIN delivery_lines l ON l.delivery_id=d.id
   WHERE d.direction='project_to_company'
   UNION ALL
   SELECT d.project_id,l.material_id,d.resolved_at,l.quantity
   FROM deliveries d JOIN delivery_lines l ON l.delivery_id=d.id
   WHERE d.direction='project_to_company' AND d.status IN ('rejected','cancelled')
  ), balances AS (
   SELECT sum(delta) OVER (PARTITION BY project_id,material_id ORDER BY moved_at) AS balance FROM movements
  )
  SELECT count(*) FROM balances WHERE balance<0`,
	}
	for name, query := range verify {
		var n int64
		if err := db.Raw(query).Row().Scan(&n); err != nil || n != 0 {
			t.Errorf("%s: %d violations (%v)", name, n, err)
		}
	}
	auth := application.NewAuth(postgres.NewAuthRepository(db), []byte("seed test JWT signing key of at least 32 bytes"), time.Now, time.Minute, time.Hour)
	tokens, err := auth.Login(ctx, "member@sitetrack.test", "SiteTrack-demo-2026!")
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authenticate(ctx, tokens.AccessToken)
	if err != nil || actor.Role != domain.RoleUser {
		t.Fatalf("seed login: %+v, %v", actor, err)
	}
	projects := application.NewProjects(postgres.NewProjectRepository(db), func() time.Time { return options.Anchor }, time.UTC)
	if p, err := projects.Get(ctx, actor, "PRJ-0001"); err != nil || !p.IsAdmin {
		t.Fatalf("demo member project: %+v, %v", p, err)
	}
	if err := seed.Populate(ctx, pool, options); !errors.Is(err, seed.ErrNotEmpty) {
		t.Fatalf("repeat seed = %v", err)
	}
	options.Reset = true
	if err := seed.Populate(ctx, pool, options); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Authenticate(ctx, tokens.AccessToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("reset left session active: %v", err)
	}
	p, err := projects.Create(ctx, actor, domain.ProjectInput{Name: "New", StartDate: "2026-11-10", EndDate: "2026-12-31"})
	if err != nil || p.ID != "PRJ-0009" {
		t.Fatalf("sequence after seed: %+v, %v", p, err)
	}
}
