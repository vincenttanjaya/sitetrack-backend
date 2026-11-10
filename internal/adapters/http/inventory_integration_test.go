package httpapi_test

import (
	"encoding/json"
	"fmt"
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

func TestInventoryHTTPDeliveryFlowAndLiveAccess(t *testing.T) {
	db := testdb.Open(t)
	auth := application.NewAuth(postgres.NewAuthRepository(db), []byte("test signing key longer than thirty-two bytes"), time.Now, time.Minute, time.Hour)
	handler := api.NewRouter(auth, nil)
	handler.RegisterInventoryRoutes(application.NewInventory(postgres.NewInventoryRepository(db), time.UTC))
	send := func(method, path, body, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: status %d, wanted %d: %s", method, path, w.Code, want, w.Body)
		}
		return w
	}
	token := func(email string, role domain.Role) (int64, string) {
		t.Helper()
		r := send(http.MethodPost, "/v1/auth/register", fmt.Sprintf(`{"name":"Store user","email":%q,"password":"safe-password","role":%q}`, email, role), "", 201)
		var user domain.User
		if err := json.Unmarshal(r.Body.Bytes(), &user); err != nil {
			t.Fatal(err)
		}
		r = send(http.MethodPost, "/v1/auth/login", fmt.Sprintf(`{"email":%q,"password":"safe-password"}`, email), "", 200)
		var pair domain.TokenPair
		if err := json.Unmarshal(r.Body.Bytes(), &pair); err != nil {
			t.Fatal(err)
		}
		return user.ID, pair.AccessToken
	}
	_, manager := token("warehouse@example.test", domain.RoleStoreManager)
	memberID, member := token("site@example.test", domain.RoleUser)
	var projectID string
	if err := db.Raw("INSERT INTO projects(name,start_date,end_date) VALUES ('Site A','2026-01-01','2026-12-31') RETURNING id").Row().Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO stores(project_id) VALUES(?)", projectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO project_members(project_id,user_id) VALUES(?,?)", projectID, memberID).Error; err != nil {
		t.Fatal(err)
	}
	r := send(http.MethodPost, "/v1/materials", `{"name":"Cement","category":"Mix","unit":"kg"}`, member, 201)
	var m domain.Material
	if err := json.Unmarshal(r.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	storeItem := fmt.Sprintf("/v1/company/store/%d", m.ID)
	send(http.MethodPut, storeItem+"/reorder-point", `{}`, manager, 400)
	send(http.MethodPost, storeItem+"/add-stock", `{"quantity":2.5}`, manager, 400)
	send(http.MethodPost, storeItem+"/add-stock", `{"quantity":"1.2345"}`, manager, 400)
	send(http.MethodPost, storeItem+"/add-stock", `{"quantity":"2.500"}`, member, 403)
	send(http.MethodPost, storeItem+"/add-stock", `{"quantity":"2.500"}`, manager, 200)
	send(http.MethodPut, storeItem+"/reorder-point", `{"reorder_point":"1.250"}`, manager, 200)
	send(http.MethodGet, "/v1/company/store?status=unknown", "", manager, 400)
	send(http.MethodGet, "/v1/company/store?order=asc&order=desc", "", manager, 400)
	send(http.MethodGet, "/v1/company/store?unsupported=true", "", manager, 400)
	deliveries := "/v1/company/deliveries"
	payload := fmt.Sprintf(`{"project_id":%q,"lines":[{"material_id":%d,"quantity":"1.250"}]}`, projectID, m.ID)
	r = send(http.MethodPost, deliveries, payload, manager, 201)
	var d domain.Delivery
	if err := json.Unmarshal(r.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.AllowedActions) != 1 || d.AllowedActions[0] != domain.CancelDelivery {
		t.Fatalf("company actions: %+v", d.AllowedActions)
	}
	r = send(http.MethodGet, "/v1/company/store?status=low_stock&category=mix&sort=quantity&order=desc", "", manager, 200)
	if !strings.Contains(r.Body.String(), `"quantity":"1.250"`) || !strings.Contains(r.Body.String(), `"in_transit_quantity":"1.250"`) {
		t.Fatalf("bad exact quantities: %s", r.Body)
	}
	project := "/v1/projects/" + projectID
	projectDelivery := fmt.Sprintf("%s/deliveries/%d", project, d.ID)
	send(http.MethodGet, project+"/deliveries?project_id="+projectID, "", member, 400)
	send(http.MethodGet, projectDelivery, "", manager, 404)
	send(http.MethodPost, projectDelivery+"/cancel", "", member, 403)
	r = send(http.MethodPost, projectDelivery+"/record", "", member, 200)
	if err := json.Unmarshal(r.Body.Bytes(), &d); err != nil || d.ReceivedBy == nil || d.ReceivedBy.ID != memberID || d.Status != domain.DeliveryDelivered {
		t.Fatalf("record: %+v %v", d, err)
	}
	send(http.MethodPost, projectDelivery+"/record", "", member, 409)
	send(http.MethodGet, project+"/store?sort=last_delivery", "", member, 200)
	if err := db.Exec("DELETE FROM project_members WHERE project_id=? AND user_id=?", projectID, memberID).Error; err != nil {
		t.Fatal(err)
	}
	send(http.MethodGet, projectDelivery, "", member, 404)
	send(http.MethodGet, project+"/store", "", member, 404)
	send(http.MethodGet, project+"/deliveries", "", member, 404)
	send(http.MethodGet, deliveries, "", member, 403)
	send(http.MethodGet, deliveries, "", manager, 200)
}
