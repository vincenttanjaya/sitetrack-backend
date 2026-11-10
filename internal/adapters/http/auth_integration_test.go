package httpapi_test

import (
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

func TestAuthenticationHTTPFlow(t *testing.T) {
	db := testdb.Open(t)
	auth := application.NewAuth(postgres.NewAuthRepository(db), []byte("test signing key longer than thirty-two bytes"), time.Now, time.Minute, time.Hour)
	handler := api.NewRouter(auth, nil)
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	register := request(http.MethodPost, "/v1/auth/register", `{"name":"Asha","email":"asha@example.com","password":"safe-password","role":"user"}`, "")
	if register.Code != http.StatusCreated || strings.Contains(register.Body.String(), "safe-password") {
		t.Fatalf("register: %d %s", register.Code, register.Body)
	}
	login := request(http.MethodPost, "/v1/auth/login", `{"email":"asha@example.com","password":"safe-password"}`, "")
	if login.Code != http.StatusOK {
		t.Fatalf("login: %d %s", login.Code, login.Body)
	}
	var pair domain.TokenPair
	if err := json.Unmarshal(login.Body.Bytes(), &pair); err != nil || pair.AccessToken == "" {
		t.Fatalf("token: %v", err)
	}
	me := request(http.MethodGet, "/v1/me", "", pair.AccessToken)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), "asha@example.com") {
		t.Fatalf("me: %d %s", me.Code, me.Body)
	}
	logout := request(http.MethodPost, "/v1/auth/logout", "", pair.AccessToken)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout: %d %s", logout.Code, logout.Body)
	}
	if revoked := request(http.MethodGet, "/v1/me", "", pair.AccessToken); revoked.Code != http.StatusUnauthorized {
		t.Fatalf("revoked: %d %s", revoked.Code, revoked.Body)
	}
	if malformed := request(http.MethodPost, "/v1/auth/login", `{"email":"asha@example.com","password":"safe-password","extra":1}`, ""); malformed.Code != http.StatusBadRequest {
		t.Fatalf("unknown input: %d %s", malformed.Code, malformed.Body)
	}
}
