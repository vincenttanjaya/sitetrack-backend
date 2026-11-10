package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	api "github.com/vincenttanjaya/sitetrack-backend/internal/adapters/http"
	"github.com/vincenttanjaya/sitetrack-backend/internal/adapters/postgres"
	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/testdb"
	"gorm.io/gorm"
)

type taskTestFixture struct {
	db       *gorm.DB
	router   *api.Router
	token    string
	memberID int64
}

func newTaskTestFixture(t *testing.T, role domain.Role) taskTestFixture {
	t.Helper()
	db := testdb.Open(t)
	ctx := context.Background()
	auth := application.NewAuth(postgres.NewAuthRepository(db), []byte("test signing key longer than thirty-two bytes"), time.Now, time.Minute, time.Hour)
	user, err := auth.Register(ctx, domain.RegisterInput{Name: "Task author", Email: "task@example.test", Password: "test-password", Role: role})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := auth.Login(ctx, "task@example.test", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO projects (name, start_date, end_date)
VALUES ('Site One', '2026-11-01', '2026-12-01'), ('Site Two', '2026-11-01', '2026-12-01')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO project_members (project_id, user_id) VALUES ('PRJ-0001', ?)", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	router := api.NewRouter(auth, nil)
	router.RegisterTaskRoutes(application.NewTasks(postgres.NewTaskRepository(db), func() time.Time {
		return time.Date(2026, 11, 9, 21, 0, 0, 0, time.UTC)
	}, time.FixedZone("Muscat", 4*60*60)))
	return taskTestFixture{db: db, router: router, token: pair.AccessToken, memberID: user.ID}
}

func (f taskTestFixture) request(method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}

func (f taskTestFixture) expect(t *testing.T, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	w := f.request(method, path, body)
	if w.Code != status {
		t.Fatalf("%s %s: want %d, got %d %s", method, path, status, w.Code, w.Body)
	}
	return w
}

const taskTestPath = "/v1/projects/PRJ-0001/tasks"
const taskTestA = "PRJ-0001-T-0001"
const taskTestB = "PRJ-0001-T-0002"
const taskTestC = "PRJ-0001-T-0003"
const taskTestD = "PRJ-0001-T-0004"

type taskTestRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Distance int    `json:"distance"`
}

type taskTestItem struct {
	ID                 string        `json:"id"`
	Name               string        `json:"name"`
	Status             string        `json:"status"`
	StartDate          string        `json:"start_date"`
	EndDate            string        `json:"end_date"`
	Late               bool          `json:"late"`
	Blocked            bool          `json:"blocked"`
	DirectDependencies []taskTestRef `json:"direct_dependencies"`
	DependentCount     int           `json:"dependent_count"`
	Prerequisites      []taskTestRef `json:"prerequisites"`
	Dependents         []taskTestRef `json:"dependents"`
	AllowedActions     []string      `json:"allowed_actions"`
}

type taskTestPage struct {
	Items      []taskTestItem `json:"items"`
	NextCursor string         `json:"next_cursor"`
}

func taskDecode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var data T
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func (f taskTestFixture) create(t *testing.T, name, start, end string) taskTestItem {
	t.Helper()
	body := `{"name":"` + name + `","start_date":"` + start + `","end_date":"` + end + `"}`
	return taskDecode[taskTestItem](t, f.expect(t, http.MethodPost, taskTestPath, body, http.StatusCreated))
}

func (f taskTestFixture) link(t *testing.T, task, prerequisite string, status int) {
	t.Helper()
	f.expect(t, http.MethodPost, taskTestPath+"/"+task+"/dependencies", `{"prerequisite_id":"`+prerequisite+`"}`, status)
}

func (f taskTestFixture) action(t *testing.T, task, action string, status int) taskTestItem {
	t.Helper()
	w := f.expect(t, http.MethodPost, taskTestPath+"/"+task+"/actions/"+action, "", status)
	if status != http.StatusOK {
		return taskTestItem{}
	}
	return taskDecode[taskTestItem](t, w)
}

func TestTaskCRUDKeepsProjectLocalIDsAndCurrentMembership(t *testing.T) {
	f := newTaskTestFixture(t, domain.RoleUser)
	a := f.create(t, "Pour wall", "2026-11-09", "2026-11-12")
	if a.ID != taskTestA || a.Status != "not_started" || !a.Late || a.Blocked {
		t.Fatalf("created task: %+v", a)
	}
	f.expect(t, http.MethodPut, taskTestPath+"/"+a.ID, `{"name":"Place wall","start_date":"2026-11-10","end_date":"2026-11-11","status":"done"}`, http.StatusBadRequest)
	updated := taskDecode[taskTestItem](t, f.expect(t, http.MethodPut, taskTestPath+"/"+a.ID, `{"name":"Place wall","start_date":"2026-11-10","end_date":"2026-11-11"}`, http.StatusOK))
	if updated.ID != a.ID || updated.Name != "Place wall" || updated.Late || updated.StartDate != "2026-11-10" {
		t.Fatalf("updated task: %+v", updated)
	}
	f.expect(t, http.MethodDelete, taskTestPath+"/"+a.ID, "", http.StatusNoContent)
	f.expect(t, http.MethodGet, taskTestPath+"/"+a.ID, "", http.StatusNotFound)
	if b := f.create(t, "Second pour", "2026-11-10", "2026-11-11"); b.ID != taskTestB {
		t.Fatalf("number reused: %+v", b)
	}
	if err := f.db.Exec("DELETE FROM project_members WHERE project_id = 'PRJ-0001' AND user_id = ?", f.memberID).Error; err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []struct{ method, path, body string }{
		{http.MethodGet, taskTestPath + "?q=Second", ""},
		{http.MethodGet, taskTestPath + "/" + taskTestB, ""},
		{http.MethodGet, taskTestPath + "/" + taskTestB + "/dependencies/" + taskTestA, ""},
		{http.MethodPost, taskTestPath + "/" + taskTestB + "/dependencies", `{"prerequisite_id":"` + taskTestA + `"}`},
		{http.MethodPost, taskTestPath + "/" + taskTestB + "/actions/start", ""},
		{http.MethodPut, taskTestPath + "/" + taskTestB, `{"name":"Wall","start_date":"2026-11-10","end_date":"2026-11-11"}`},
		{http.MethodDelete, taskTestPath + "/" + taskTestB, ""},
		{http.MethodPost, taskTestPath, `{"name":"New","start_date":"2026-11-10","end_date":"2026-11-11"}`},
	} {
		f.expect(t, endpoint.method, endpoint.path, endpoint.body, http.StatusNotFound)
	}
}

func TestTaskDetailsShowShortestPathsAndLegalActions(t *testing.T) {
	f := newTaskTestFixture(t, domain.RoleUser)
	for _, name := range []string{"A", "B", "C", "D"} {
		f.create(t, name, "2026-11-08", "2026-11-12")
	}
	f.link(t, taskTestB, taskTestA, http.StatusCreated)
	f.link(t, taskTestC, taskTestA, http.StatusCreated)
	f.link(t, taskTestD, taskTestB, http.StatusCreated)
	f.link(t, taskTestD, taskTestC, http.StatusCreated)
	f.link(t, taskTestD, taskTestA, http.StatusCreated)

	d := taskDecode[taskTestItem](t, f.expect(t, http.MethodGet, taskTestPath+"/"+taskTestD, "", http.StatusOK))
	want := []taskTestRef{
		{ID: taskTestA, Name: "A", Status: "not_started", Distance: 1},
		{ID: taskTestB, Name: "B", Status: "not_started", Distance: 1},
		{ID: taskTestC, Name: "C", Status: "not_started", Distance: 1},
	}
	if !d.Late || !d.Blocked || !reflect.DeepEqual(d.Prerequisites, want) || !reflect.DeepEqual(d.AllowedActions, []string{}) {
		t.Fatalf("blocked details: %+v", d)
	}

	f.expect(t, http.MethodDelete, taskTestPath+"/"+taskTestD+"/dependencies/"+taskTestA, "", http.StatusNoContent)
	d = taskDecode[taskTestItem](t, f.expect(t, http.MethodGet, taskTestPath+"/"+taskTestD, "", http.StatusOK))
	if len(d.Prerequisites) != 3 || d.Prerequisites[2].ID != taskTestA || d.Prerequisites[2].Distance != 2 {
		t.Fatalf("diamond did not deduplicate paths or record shortest distance: %+v", d.Prerequisites)
	}
	a := taskDecode[taskTestItem](t, f.expect(t, http.MethodGet, taskTestPath+"/"+taskTestA, "", http.StatusOK))
	if len(a.Dependents) != 3 || a.Dependents[2].ID != taskTestD || a.Dependents[2].Distance != 2 || !reflect.DeepEqual(a.AllowedActions, []string{"start"}) {
		t.Fatalf("dependent paths or allowed actions: %+v", a)
	}

	f.action(t, taskTestD, "start", http.StatusConflict)
	f.action(t, taskTestD, "done", http.StatusConflict)
	f.action(t, taskTestD, "reopen", http.StatusConflict)
	if got := f.action(t, taskTestA, "start", http.StatusOK); got.Status != "in_progress" || got.Late || !reflect.DeepEqual(got.AllowedActions, []string{"done"}) {
		t.Fatalf("start result: %+v", got)
	}
	f.action(t, taskTestA, "start", http.StatusConflict)
	if got := f.action(t, taskTestA, "done", http.StatusOK); got.Status != "done" || got.Late || got.Blocked || !reflect.DeepEqual(got.AllowedActions, []string{"reopen"}) {
		t.Fatalf("done result: %+v", got)
	}
	f.action(t, taskTestB, "start", http.StatusOK)
	f.action(t, taskTestA, "reopen", http.StatusConflict)
	f.action(t, taskTestB, "done", http.StatusOK)
	f.action(t, taskTestD, "start", http.StatusConflict)
	f.action(t, taskTestC, "start", http.StatusOK)
	f.action(t, taskTestC, "done", http.StatusOK)
	f.action(t, taskTestD, "start", http.StatusOK)
}

func TestTaskDependencyMutationsProtectGraphAndReferencedTasks(t *testing.T) {
	f := newTaskTestFixture(t, domain.RoleSuperAdmin)
	for _, name := range []string{"A", "B", "C"} {
		f.create(t, name, "2026-11-08", "2026-11-12")
	}
	other := taskDecode[taskTestItem](t, f.expect(t, http.MethodPost, "/v1/projects/PRJ-0002/tasks", `{"name":"Other","start_date":"2026-11-08","end_date":"2026-11-12"}`, http.StatusCreated))
	if other.ID != "PRJ-0002-T-0001" {
		t.Fatalf("numbering is not project-local: %s", other.ID)
	}
	f.link(t, taskTestA, other.ID, http.StatusNotFound)
	f.link(t, taskTestA, taskTestA, http.StatusConflict)
	f.link(t, taskTestB, taskTestA, http.StatusCreated)
	f.link(t, taskTestB, taskTestA, http.StatusConflict)
	f.link(t, taskTestC, taskTestB, http.StatusCreated)
	f.link(t, taskTestA, taskTestC, http.StatusConflict)
	path := taskTestPath + "/" + taskTestB + "/dependencies/" + taskTestA
	f.expect(t, http.MethodPut, path, `{"prerequisite_id":"`+taskTestC+`"}`, http.StatusConflict)
	ref := taskDecode[taskTestRef](t, f.expect(t, http.MethodGet, path, "", http.StatusOK))
	if ref.ID != taskTestA || ref.Name != "A" || ref.Status != "not_started" {
		t.Fatalf("failed replacement removed original edge: %+v", ref)
	}
	f.expect(t, http.MethodPut, taskTestPath+"/"+taskTestB+"/dependencies/"+taskTestD, `{"prerequisite_id":"`+taskTestC+`"}`, http.StatusNotFound)
	f.expect(t, http.MethodDelete, taskTestPath+"/"+taskTestA, "", http.StatusConflict)
	f.expect(t, http.MethodPut, taskTestPath+"/"+taskTestC+"/dependencies/"+taskTestB, `{"prerequisite_id":"`+taskTestA+`"}`, http.StatusOK)
	f.expect(t, http.MethodGet, taskTestPath+"/"+taskTestC+"/dependencies/"+taskTestB, "", http.StatusNotFound)
	f.expect(t, http.MethodDelete, path, "", http.StatusNoContent)
	f.action(t, taskTestB, "start", http.StatusOK)
	f.link(t, taskTestB, taskTestA, http.StatusConflict)
	f.action(t, taskTestA, "start", http.StatusOK)
	f.action(t, taskTestA, "done", http.StatusOK)
	f.link(t, taskTestB, taskTestA, http.StatusCreated)
	f.action(t, taskTestA, "reopen", http.StatusConflict)
	f.expect(t, http.MethodDelete, taskTestPath+"/"+taskTestB, "", http.StatusNoContent)
}

func TestTaskListFiltersAndCursorTies(t *testing.T) {
	f := newTaskTestFixture(t, domain.RoleUser)
	f.create(t, "Form_01", "2026-11-01", "2026-11-12")
	f.create(t, "Form_01", "2026-11-01", "2026-11-12")
	f.create(t, "Formx01", "2026-11-12", "2026-11-14")
	f.create(t, "Copper", "2026-11-08", "2026-11-09")
	f.link(t, taskTestB, taskTestA, http.StatusCreated)
	f.link(t, taskTestC, taskTestA, http.StatusCreated)
	f.link(t, taskTestD, taskTestC, http.StatusCreated)
	query := "?status=not_started&late=true&blocked=true&from=2026-11-09&to=2026-11-11&q=Form_"
	page := taskDecode[taskTestPage](t, f.expect(t, http.MethodGet, taskTestPath+query, "", http.StatusOK))
	if len(page.Items) != 1 || page.Items[0].ID != taskTestB || len(page.Items[0].DirectDependencies) != 1 || page.Items[0].DirectDependencies[0].ID != taskTestA {
		t.Fatalf("combined filters: %+v", page)
	}
	filter := "?sort=name&order=desc&limit=1&q=Form_"
	page = taskDecode[taskTestPage](t, f.expect(t, http.MethodGet, taskTestPath+filter, "", http.StatusOK))
	if len(page.Items) != 1 || page.Items[0].ID != taskTestB || page.NextCursor == "" {
		t.Fatalf("first page: %+v", page)
	}
	cursor := page.NextCursor
	page = taskDecode[taskTestPage](t, f.expect(t, http.MethodGet, taskTestPath+filter+"&cursor="+cursor, "", http.StatusOK))
	if len(page.Items) != 1 || page.Items[0].ID != taskTestA || page.NextCursor != "" {
		t.Fatalf("second page: %+v", page)
	}
	f.expect(t, http.MethodGet, taskTestPath+"?sort=id&cursor="+cursor, "", http.StatusBadRequest)
	for _, tc := range []struct{ query, want string }{
		{"?sort=dependent_count&order=desc", taskTestA},
		{"?sort=start_date&order=desc", taskTestC},
		{"?sort=end_date", taskTestD},
		{"?sort=id&order=desc", taskTestD},
		{"?blocked=false", taskTestA},
		{"?late=false", taskTestC},
		{"?q=0003", taskTestC},
		{"?from=2026-11-14", taskTestC},
		{"?to=2026-11-01", taskTestA},
	} {
		page = taskDecode[taskTestPage](t, f.expect(t, http.MethodGet, taskTestPath+tc.query, "", http.StatusOK))
		if len(page.Items) == 0 || page.Items[0].ID != tc.want {
			t.Fatalf("%s: %+v", tc.query, page)
		}
	}
	page = taskDecode[taskTestPage](t, f.expect(t, http.MethodGet, taskTestPath+"?q=nomatch", "", http.StatusOK))
	if page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("empty list: %+v", page)
	}
	for _, query := range []string{"?sort=invalid", "?order=invalid", "?limit=101", "?status=late", "?late=yes", "?from=2026-11-30&to=2026-11-01", "?to=2026-02-30", "?include=anything", "?status=", "?q=%FF"} {
		f.expect(t, http.MethodGet, taskTestPath+query, "", http.StatusBadRequest)
	}
}

func TestTaskProjectLockSerializesCompetingWrites(t *testing.T) {
	f := newTaskTestFixture(t, domain.RoleUser)
	createBody := `{"name":"Parallel","start_date":"2026-11-10","end_date":"2026-11-11"}`
	responses := make(chan *httptest.ResponseRecorder, 8)
	for range 8 {
		go func() { responses <- f.request(http.MethodPost, taskTestPath, createBody) }()
	}
	ids := map[string]bool{}
	for range 8 {
		w := <-responses
		if w.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", w.Code, w.Body)
		}
		id := taskDecode[taskTestItem](t, w).ID
		if ids[id] {
			t.Fatalf("concurrent ID reuse: %s", id)
		}
		ids[id] = true
	}
	start := make(chan struct{})
	for _, edge := range [][2]string{{taskTestA, taskTestB}, {taskTestB, taskTestA}} {
		go func() {
			<-start
			responses <- f.request(http.MethodPost, taskTestPath+"/"+edge[0]+"/dependencies", `{"prerequisite_id":"`+edge[1]+`"}`)
		}()
	}
	close(start)
	first, second := <-responses, <-responses
	if min(first.Code, second.Code) != http.StatusCreated || max(first.Code, second.Code) != http.StatusConflict {
		t.Fatalf("cycle creation race: %d %s, %d %s", first.Code, first.Body, second.Code, second.Body)
	}
	f.link(t, taskTestD, taskTestC, http.StatusCreated)
	f.action(t, taskTestC, "start", http.StatusOK)
	f.action(t, taskTestC, "done", http.StatusOK)
	start = make(chan struct{})
	for _, action := range [][2]string{{taskTestC, "reopen"}, {taskTestD, "start"}} {
		go func() {
			<-start
			responses <- f.request(http.MethodPost, taskTestPath+"/"+action[0]+"/actions/"+action[1], "")
		}()
	}
	close(start)
	first, second = <-responses, <-responses
	if min(first.Code, second.Code) != http.StatusOK || max(first.Code, second.Code) != http.StatusConflict {
		t.Fatalf("reopen/start race: %d %s, %d %s", first.Code, first.Body, second.Code, second.Body)
	}
}

func TestTaskGraphTraversesALargeDiamondChain(t *testing.T) {
	f := newTaskTestFixture(t, domain.RoleUser)
	if err := f.db.Exec(`INSERT INTO tasks (project_id, number, name, start_date, end_date)
SELECT 'PRJ-0001', n, 'Task ' || n, '2026-11-10', '2026-11-12'
FROM generate_series(7001, 10000) n`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`INSERT INTO task_dependencies (project_id, task_number, prerequisite_number)
SELECT 'PRJ-0001', n, n - hop FROM generate_series(7002, 10000) n
CROSS JOIN generate_series(1, 2) hop WHERE n - hop >= 7001`).Error; err != nil {
		t.Fatal(err)
	}

	const lastID = "PRJ-0001-T-10000"
	started := time.Now()
	detail := taskDecode[taskTestItem](t, f.expect(t, http.MethodGet, taskTestPath+"/"+lastID, "", http.StatusOK))
	t.Logf("3,000-task diamond chain detail: %s", time.Since(started))
	if len(detail.Prerequisites) != 2999 || detail.Prerequisites[0].ID != "PRJ-0001-T-9998" ||
		detail.Prerequisites[2998].ID != "PRJ-0001-T-7001" || detail.Prerequisites[2998].Distance != 1500 {
		t.Fatalf("unexpected distances or duplicate paths, count=%d", len(detail.Prerequisites))
	}
	page := taskDecode[taskTestPage](t, f.expect(t, http.MethodGet, taskTestPath+"?sort=id&order=desc&limit=2", "", http.StatusOK))
	if len(page.Items) != 2 || page.Items[0].ID != lastID || page.Items[1].ID != "PRJ-0001-T-9999" ||
		len(page.Items[0].DirectDependencies) != 2 || page.Items[0].DependentCount != 0 || page.Items[1].DependentCount != 1 {
		t.Fatalf("numeric ID ordering or edge list projection: %+v", page)
	}
	page = taskDecode[taskTestPage](t, f.expect(t, http.MethodGet, taskTestPath+"?sort=id&order=desc&limit=2&cursor="+page.NextCursor, "", http.StatusOK))
	if len(page.Items) != 2 || page.Items[0].ID != "PRJ-0001-T-9998" {
		t.Fatalf("numeric ID cursor: %+v", page)
	}
	started = time.Now()
	f.link(t, "PRJ-0001-T-7001", lastID, http.StatusConflict)
	t.Logf("3,000-task chain cycle rejection: %s", time.Since(started))
}

func TestTaskAccessKeepsCompanyStoreManagersProjectScoped(t *testing.T) {
	f := newTaskTestFixture(t, domain.RoleStoreManager)
	f.create(t, "Accessible", "2026-11-10", "2026-11-12")
	if err := f.db.Exec(`INSERT INTO tasks (project_id, number, name, start_date, end_date)
VALUES ('PRJ-0002', 1, 'Private', '2026-11-10', '2026-11-12')`).Error; err != nil {
		t.Fatal(err)
	}
	const otherID = "PRJ-0002-T-0001"
	f.expect(t, http.MethodGet, "/v1/projects/PRJ-0002/tasks", "", http.StatusNotFound)
	f.expect(t, http.MethodGet, taskTestPath+"/"+otherID, "", http.StatusNotFound)
	f.link(t, taskTestA, otherID, http.StatusNotFound)
}

func TestTaskDetailAndUpdateAvoidUnrelatedDenseGraph(t *testing.T) {
	f := newTaskTestFixture(t, domain.RoleUser)
	f.create(t, "A", "2026-11-10", "2026-11-12")
	f.create(t, "B", "2026-11-10", "2026-11-12")
	f.link(t, taskTestB, taskTestA, http.StatusCreated)
	if err := f.db.Exec(`INSERT INTO tasks (project_id, number, name, start_date, end_date)
SELECT 'PRJ-0001', n, 'Dense component ' || n, '2026-11-10', '2026-11-12'
FROM generate_series(3, 602) n`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`INSERT INTO task_dependencies (project_id, task_number, prerequisite_number)
SELECT 'PRJ-0001', n, prerequisite
FROM generate_series(4, 602) n
CROSS JOIN LATERAL generate_series(3, n - 1) prerequisite`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("ANALYZE task_dependencies").Error; err != nil {
		t.Fatal(err)
	}

	for _, req := range []struct{ method, path, body string }{
		{http.MethodGet, taskTestPath + "/" + taskTestB, ""},
		{http.MethodPut, taskTestPath + "/" + taskTestB, `{"name":"B","start_date":"2026-11-10","end_date":"2026-11-12"}`},
	} {
		t.Run(req.method, func(t *testing.T) {
			started := time.Now()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			detail := taskDecode[taskTestItem](t, f.expect(t, req.method, req.path, req.body, http.StatusOK))
			runtime.ReadMemStats(&after)
			allocated := after.TotalAlloc - before.TotalAlloc
			t.Logf("179,700 unrelated edges: %s allocated %d bytes in %s", req.method, allocated, time.Since(started))
			if len(detail.Prerequisites) != 1 || detail.Prerequisites[0].ID != taskTestA ||
				detail.Prerequisites[0].Distance != 1 || len(detail.Dependents) != 0 {
				t.Fatalf("detail contains unrelated tasks: %+v", detail)
			}
			// A two-task component should require far less memory than even
			// a single pair of int64 values per unrelated edge.
			if allocated > 2<<20 {
				t.Fatalf("detail allocation %d bytes exceeded 2 MiB", allocated)
			}
		})
	}
}
