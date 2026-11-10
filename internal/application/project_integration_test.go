package application_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/adapters/postgres"
	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/testdb"
	"gorm.io/gorm"
)

func testActor(t *testing.T, db *gorm.DB, name string, role domain.Role) domain.Actor {
	t.Helper()
	actor := domain.Actor{Role: role}
	err := db.Raw("INSERT INTO users (name,email,password_hash,role) VALUES (?,?,?,?) RETURNING id", name, name+"@example.com", "fixture", role).Row().Scan(&actor.UserID)
	if err != nil {
		t.Fatal(err)
	}
	return actor
}

func TestProjectVisibilityInvitationAndImmediateMembershipRemoval(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	projects := application.NewProjects(postgres.NewProjectRepository(db), func() time.Time { return time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC) }, time.UTC)
	owner := testActor(t, db, "owner", domain.RoleUser)
	invited := testActor(t, db, "invited", domain.RoleUser)
	manager := testActor(t, db, "warehouse", domain.RoleStoreManager)
	super := testActor(t, db, "admin", domain.RoleSuperAdmin)
	input := domain.ProjectInput{Name: "Pier", StartDate: "2026-10-01", EndDate: "2027-01-01"}
	project, err := projects.Create(ctx, owner, input)
	if err != nil || project.ID != "PRJ-0001" || !project.IsAdmin {
		t.Fatalf("new project = %+v, %v", project, err)
	}
	if _, err := projects.Get(ctx, manager, project.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("nonmember manager access: %v", err)
	}
	if view, err := projects.Get(ctx, super, project.ID); err != nil || view.IsAdmin {
		t.Fatalf("superadmin view: %+v, %v", view, err)
	}
	if _, err := projects.Invite(ctx, super, project.ID, "invited@example.com"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("nonmember superadmin invite: %v", err)
	}
	list, err := projects.List(ctx, invited, domain.ProjectFilter{Search: "Pier"})
	if err != nil || len(list.Items) != 0 || list.Items == nil {
		t.Fatalf("scoped search: %+v, %v", list, err)
	}
	invite, err := projects.Invite(ctx, owner, project.ID, " INVITED@example.com ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.Respond(ctx, manager, invite.ID, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("wrong recipient accept: %v", err)
	}
	if _, err := projects.Get(ctx, invited, project.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("pending invite granted access: %v", err)
	}
	inbox, err := projects.Notifications(ctx, invited, domain.Page{})
	if err != nil || inbox.UnreadCount != 1 || len(inbox.Items) != 1 || inbox.Items[0].ID != invite.ID {
		t.Fatalf("notification list: %+v, %v", inbox, err)
	}
	if _, err := projects.Respond(ctx, invited, invite.ID, true); err != nil {
		t.Fatal(err)
	}
	visible, err := projects.Get(ctx, invited, project.ID)
	if err != nil || visible.MemberCount != 2 || visible.IsAdmin {
		t.Fatalf("membership view: %+v, %v", visible, err)
	}
	if err := projects.RemoveMember(ctx, owner, project.ID, invited.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.Get(ctx, invited, project.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("removed actor retained access: %v", err)
	}
	if _, err := projects.Members(ctx, invited, project.ID, domain.Page{}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("removed actor reads members: %v", err)
	}
	if _, err := projects.Invitations(ctx, invited, project.ID, domain.Page{}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("removed actor reads invitations: %v", err)
	}
	if n, err := projects.UnreadCount(ctx, invited); err != nil || n != 0 {
		t.Fatalf("responded notification unread count: %d, %v", n, err)
	}
	next, err := projects.Invite(ctx, owner, project.ID, "invited@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := projects.Withdraw(ctx, owner, project.ID, next.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.Respond(ctx, invited, next.ID, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("withdrawn invite accepted: %v", err)
	}
}

func TestConcurrentProjectAdminDemotionKeepsAnAdmin(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	projects := application.NewProjects(postgres.NewProjectRepository(db), time.Now, time.UTC)
	first := testActor(t, db, "first", domain.RoleUser)
	second := testActor(t, db, "second", domain.RoleUser)
	p, err := projects.Create(ctx, first, domain.ProjectInput{Name: "Site", StartDate: "2026-01-01", EndDate: "2027-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO project_members (project_id,user_id,is_admin) VALUES (?,?,true)", p.ID, second.UserID).Error; err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	result := make(chan error, 2)
	var wg sync.WaitGroup
	for _, actor := range []domain.Actor{first, second} {
		wg.Go(func() { <-start; result <- projects.SetMemberAdmin(ctx, actor, p.ID, actor.UserID, false) })
	}
	close(start)
	wg.Wait()
	close(result)
	successes, conflicts := 0, 0
	for err := range result {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrConflict):
			conflicts++
		default:
			t.Errorf("demotion error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestProjectSummariesAndStableTiedPagination(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	projects := application.NewProjects(postgres.NewProjectRepository(db), func() time.Time { return time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC) }, time.UTC)
	owner := testActor(t, db, "owner", domain.RoleUser)
	input := domain.ProjectInput{Name: "Site", StartDate: "2026-01-01", EndDate: "2027-01-01"}
	for i := 1; i <= 3; i++ {
		p, err := projects.Create(ctx, owner, input)
		if err != nil || p.ID != fmt.Sprintf("PRJ-%04d", i) {
			t.Fatalf("id allocation = %+v, %v", p, err)
		}
	}
	if err := db.Exec(`INSERT INTO tasks (project_id,number,name,start_date,end_date,status) VALUES
  ('PRJ-0002',1,'Foundation','2026-01-01','2026-10-02','in_progress'),
  ('PRJ-0002',2,'Pillars','2026-10-03','2026-12-02','not_started'),
  ('PRJ-0002',3,'Permit','2026-01-01','2026-01-02','done')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO task_dependencies (project_id,task_number,prerequisite_number) VALUES ('PRJ-0002',2,1)").Error; err != nil {
		t.Fatal(err)
	}
	filtered, err := projects.List(ctx, owner, domain.ProjectFilter{LateOnly: true, Sort: "late_tasks", Order: "desc"})
	if err != nil || len(filtered.Items) != 1 {
		t.Fatalf("late projects = %+v, %v", filtered, err)
	}
	p := filtered.Items[0]
	if p.ID != "PRJ-0002" || p.LateTasks != 2 || p.BlockedTasks != 1 || p.PercentDone != 33.33 || p.TaskCounts.Done != 1 || p.TaskCounts.InProgress != 1 || p.TaskCounts.NotStarted != 1 || p.MemberCount != 1 {
		t.Fatalf("project stats = %+v", p)
	}
	filter := domain.ProjectFilter{Page: domain.Page{Limit: 1}}
	for _, want := range []string{"PRJ-0001", "PRJ-0002", "PRJ-0003"} {
		page, err := projects.List(ctx, owner, filter)
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != want {
			t.Fatalf("tied page = %+v, %v", page, err)
		}
		filter.Page.Cursor = page.NextCursor
	}
	if filter.Page.Cursor != "" {
		t.Fatal("unexpected final cursor")
	}
	if err := projects.Delete(ctx, owner, "PRJ-0003"); err != nil {
		t.Fatal(err)
	}
	p, err = projects.Create(ctx, owner, input)
	if err != nil || p.ID != "PRJ-0004" {
		t.Fatalf("deleted project id reused = %+v, %v", p, err)
	}
	if err := db.Exec("ALTER TABLE projects ALTER COLUMN number RESTART WITH 10000").Error; err != nil {
		t.Fatal(err)
	}
	p, err = projects.Create(ctx, owner, input)
	if err != nil || p.ID != "PRJ-10000" {
		t.Fatalf("minimum width project id = %+v, %v", p, err)
	}
}
