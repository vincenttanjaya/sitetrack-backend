package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
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

func TestProjectHTTPRejectsMalformedText(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	auth := application.NewAuth(postgres.NewAuthRepository(db), []byte("test signing key longer than thirty-two bytes"), time.Now, time.Minute, time.Hour)
	if _, err := auth.Register(ctx, domain.RegisterInput{
		Name: "Project reviewer", Email: "reviewer@example.test", Password: "test-password", Role: domain.RoleSuperAdmin,
	}); err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.Login(ctx, "reviewer@example.test", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	router := api.NewRouter(auth, nil)
	router.RegisterProjectRoutes(application.NewProjects(postgres.NewProjectRepository(db), time.Now, time.UTC))

	expectError := func(t *testing.T, method, path, body string, status int, code string) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		var payload struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil || resp.Code != status || payload.Error.Code != code {
			t.Fatalf("%s %s: want %d %s, got %d %s (decode: %v)", method, path, status, code, resp.Code, resp.Body, err)
		}
	}

	t.Run("search", func(t *testing.T) {
		expectError(t, http.MethodGet, "/v1/projects?q=%FF", "", http.StatusBadRequest, "invalid_input")
	})

	for _, invalidID := range []string{"PRJ-%FF", "PRJ-%00"} {
		for _, route := range []struct{ method, suffix, body string }{
			{http.MethodGet, "", ""},
			{http.MethodPut, "", `{"name":"Site","start_date":"2026-01-01","end_date":"2027-01-01"}`},
			{http.MethodDelete, "", ""},
			{http.MethodGet, "/members", ""},
			{http.MethodPut, "/members/1", `{"is_admin":true}`},
			{http.MethodDelete, "/members/1", ""},
			{http.MethodGet, "/invitations", ""},
			{http.MethodPost, "/invitations", `{"email":"other@example.test"}`},
			{http.MethodDelete, "/invitations/1", ""},
		} {
			path := "/v1/projects/" + invalidID + route.suffix
			t.Run(route.method+" "+path, func(t *testing.T) {
				expectError(t, route.method, path, route.body, http.StatusNotFound, "not_found")
			})
		}
	}
}
