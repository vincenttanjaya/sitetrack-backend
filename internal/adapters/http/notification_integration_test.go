package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	api "github.com/vincenttanjaya/sitetrack-backend/internal/adapters/http"
	"github.com/vincenttanjaya/sitetrack-backend/internal/adapters/postgres"
	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/testdb"
)

func TestNotificationKeepsInvitationProjectLabel(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	auth := application.NewAuth(postgres.NewAuthRepository(db), []byte("notification test key of at least 32 bytes"), time.Now, time.Minute, time.Hour)
	projects := application.NewProjects(postgres.NewProjectRepository(db), time.Now, time.UTC)
	router := api.NewRouter(auth, nil)
	router.RegisterProjectRoutes(projects)
	account := func(email string) (domain.Actor, string) {
		t.Helper()
		if _, err := auth.Register(ctx, domain.RegisterInput{Name: "Test account", Email: email, Password: "test-password", Role: domain.RoleUser}); err != nil {
			t.Fatal(err)
		}
		tokens, err := auth.Login(ctx, email, "test-password")
		if err != nil {
			t.Fatal(err)
		}
		actor, err := auth.Authenticate(ctx, tokens.AccessToken)
		if err != nil {
			t.Fatal(err)
		}
		return actor, tokens.AccessToken
	}
	request := func(token, method, path, body string, status int) []byte {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, res.Code, status, res.Body)
		}
		return res.Body.Bytes()
	}
	owner, _ := account("owner@example.test")
	const original = "Original invitation label"
	input := domain.ProjectInput{Name: original, StartDate: "2026-01-01", EndDate: "2026-12-31"}
	project, err := projects.Create(ctx, owner, input)
	if err != nil {
		t.Fatal(err)
	}

	type recipient struct {
		email, action, status, token string
		id                           int64
	}
	recipients := []recipient{
		{email: "accept@example.test", action: "accept", status: "accepted"},
		{email: "reject@example.test", action: "reject", status: "rejected"},
	}
	checkInbox := func(rec recipient, status string, unread int64) {
		t.Helper()
		var inbox domain.Notifications
		body := request(rec.token, "GET", "/v1/notifications", "", 200)
		if err := json.Unmarshal(body, &inbox); err != nil {
			t.Fatal(err)
		}
		if inbox.UnreadCount != unread || len(inbox.Items) != 1 || inbox.Items[0].ID != rec.id ||
			inbox.Items[0].ProjectName != original || inbox.Items[0].Status != status {
			t.Fatalf("unexpected inbox for %s: %s", rec.action, body)
		}
	}
	for i := range recipients {
		rec := &recipients[i]
		actor, token := account(rec.email)
		rec.token = token
		invite, err := projects.Invite(ctx, owner, project.ID, rec.email)
		if err != nil {
			t.Fatal(err)
		}
		rec.id = invite.ID
		checkInbox(*rec, "pending", 1)
		path := fmt.Sprintf("/v1/notifications/%d/respond", invite.ID)
		request(token, "POST", path, fmt.Sprintf(`{"response":%q}`, rec.action), 200)
		if rec.action == "accept" {
			if err := projects.RemoveMember(ctx, owner, project.ID, actor.UserID); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, label := range []string{"First revised label", "Second revised label"} {
		input.Name = label
		if _, err := projects.Update(ctx, owner, project.ID, input); err != nil {
			t.Fatal(err)
		}
		for _, rec := range recipients {
			request(rec.token, "GET", "/v1/projects/"+project.ID, "", 404)
			checkInbox(rec, rec.status, 0)
		}
	}
}
