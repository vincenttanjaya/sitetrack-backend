package application_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/adapters/postgres"
	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/testdb"
	"gorm.io/gorm"
)

type inventoryFixture struct {
	db                               *gorm.DB
	inventory                        *application.InventoryService
	ctx                              context.Context
	manager, member, stranger, admin domain.Actor
	projectID, otherProjectID        string
}

func inventorySetup(t *testing.T) inventoryFixture {
	t.Helper()
	db := testdb.Open(t)
	f := inventoryFixture{db: db, inventory: application.NewInventory(postgres.NewInventoryRepository(db), time.UTC), ctx: context.Background()}
	for i, a := range []*domain.Actor{&f.manager, &f.member, &f.stranger, &f.admin} {
		a.UserID = int64(i + 1)
		a.Role = []domain.Role{domain.RoleStoreManager, domain.RoleUser, domain.RoleUser, domain.RoleSuperAdmin}[i]
		if err := db.Exec("INSERT INTO users(id,name,email,password_hash,role) VALUES(?,?,?,'unused',?)", a.UserID, fmt.Sprintf("User %d", i+1), fmt.Sprintf("u%d@example.test", i+1), a.Role).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range []*string{&f.projectID, &f.otherProjectID} {
		if err := db.Raw("INSERT INTO projects(name,start_date,end_date) VALUES(?,'2026-01-01','2026-12-31') RETURNING id", fmt.Sprintf("Site %d", i+1)).Row().Scan(id); err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("INSERT INTO stores(project_id) VALUES(?)", *id).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("INSERT INTO project_members(project_id,user_id,is_admin) VALUES(?,?,true),(?,?,true)", f.projectID, f.member.UserID, f.otherProjectID, f.stranger.UserID).Error; err != nil {
		t.Fatal(err)
	}
	return f
}
func (f inventoryFixture) material(t *testing.T, name, category string, qty domain.Quantity) domain.Material {
	t.Helper()
	m, err := f.inventory.CreateMaterial(f.ctx, f.member, domain.MaterialInput{Name: name, Category: category, Unit: "kg"})
	if err != nil {
		t.Fatal(err)
	}
	if qty > 0 {
		if _, err := f.inventory.AddStock(f.ctx, f.manager, m.ID, qty); err != nil {
			t.Fatal(err)
		}
	}
	return m
}
func (f inventoryFixture) send(t *testing.T, id int64, qty domain.Quantity) domain.Delivery {
	t.Helper()
	d, err := f.inventory.InitiateDelivery(f.ctx, f.manager, domain.CompanyDeliveryScope(), f.projectID, []domain.DeliveryLineInput{{MaterialID: id, Quantity: qty}})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func (f inventoryFixture) companyQuantity(t *testing.T, name string) domain.CompanyStockItem {
	t.Helper()
	rows, err := f.inventory.ListCompanyStock(f.ctx, f.manager, domain.CompanyStockFilter{Search: name})
	if err != nil || len(rows.Items) != 1 {
		t.Fatalf("company stock: %+v, %v", rows, err)
	}
	return rows.Items[0]
}
func TestInventoryMultilineFailureRollsBackAndConcurrentSendsCannotOverspend(t *testing.T) {
	f := inventorySetup(t)
	cement := f.material(t, "Cement", "Mix", 1500)
	sand := f.material(t, "Sand", "Mix", 300)
	_, err := f.inventory.InitiateDelivery(f.ctx, f.manager, domain.CompanyDeliveryScope(), f.projectID, []domain.DeliveryLineInput{{MaterialID: cement.ID, Quantity: 1100}, {MaterialID: sand.ID, Quantity: 400}})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("wanted insufficient stock conflict, got %v", err)
	}
	if row := f.companyQuantity(t, "Cement"); row.Quantity != 1500 || row.InTransitQuantity != 0 {
		t.Fatalf("first line changed despite rollback: %+v", row)
	}
	if page, err := f.inventory.ListDeliveries(f.ctx, f.manager, domain.CompanyDeliveryScope(), domain.DeliveryFilter{}); err != nil || len(page.Items) != 0 {
		t.Fatalf("failed initiation left a delivery: %+v, %v", page, err)
	}
	start := make(chan struct{})
	result := make(chan error, 2)
	for _, projectID := range []string{f.projectID, f.otherProjectID} {
		go func() {
			<-start
			_, err := f.inventory.InitiateDelivery(f.ctx, f.manager, domain.CompanyDeliveryScope(), projectID, []domain.DeliveryLineInput{{MaterialID: cement.ID, Quantity: 1001}})
			result <- err
		}()
	}
	close(start)
	first, second := <-result, <-result
	if !((first == nil && errors.Is(second, domain.ErrConflict)) || (second == nil && errors.Is(first, domain.ErrConflict))) {
		t.Fatalf("concurrent sends: %v; %v", first, second)
	}
	if row := f.companyQuantity(t, "Cement"); row.Quantity != 499 || row.InTransitQuantity != 1001 {
		t.Fatalf("stock was not conserved: %+v", row)
	}
}
func TestInventoryTerminalActionsRaceAndRespectStoreAndMembership(t *testing.T) {
	f := inventorySetup(t)
	m := f.material(t, "Cement", "Mix", 2500)
	d := f.send(t, m.ID, 1250)
	project, company := domain.ProjectDeliveryScope(f.projectID), domain.CompanyDeliveryScope()
	for _, test := range []struct {
		actor  domain.Actor
		scope  domain.DeliveryScope
		action domain.DeliveryAction
		want   error
	}{
		{f.stranger, project, domain.RecordDelivery, domain.ErrNotFound}, {f.member, company, domain.CancelDelivery, domain.ErrForbidden},
		{f.manager, project, domain.RecordDelivery, domain.ErrNotFound}, {f.manager, company, domain.RecordDelivery, domain.ErrForbidden},
		{f.member, project, domain.CancelDelivery, domain.ErrForbidden}, {f.stranger, domain.ProjectDeliveryScope(f.otherProjectID), domain.RecordDelivery, domain.ErrNotFound},
	} {
		if _, err := f.inventory.ResolveDelivery(f.ctx, test.actor, test.scope, d.ID, test.action); !errors.Is(err, test.want) {
			t.Fatalf("actor %d action %s: got %v, want %v", test.actor.UserID, test.action, err, test.want)
		}
	}
	start := make(chan struct{})
	type result struct {
		d   domain.Delivery
		err error
	}
	results := make(chan result, 3)
	for _, job := range []struct {
		actor  domain.Actor
		scope  domain.DeliveryScope
		action domain.DeliveryAction
	}{
		{f.member, project, domain.RecordDelivery}, {f.member, project, domain.RejectDelivery}, {f.manager, company, domain.CancelDelivery},
	} {
		go func() {
			<-start
			d, err := f.inventory.ResolveDelivery(f.ctx, job.actor, job.scope, d.ID, job.action)
			results <- result{d, err}
		}()
	}
	close(start)
	var winner domain.Delivery
	wins := 0
	for range 3 {
		res := <-results
		if res.err == nil {
			winner = res.d
			wins++
		} else if !errors.Is(res.err, domain.ErrConflict) {
			t.Fatal(res.err)
		}
	}
	if wins != 1 || winner.ResolvedAt == nil || winner.ResolvedBy == nil {
		t.Fatalf("wanted one audited terminal winner, got %d %+v", wins, winner)
	}
	stock, err := f.inventory.ListProjectStock(f.ctx, f.member, f.projectID, domain.ProjectStockFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var projectQty domain.Quantity
	if len(stock.Items) > 0 {
		projectQty = stock.Items[0].Quantity
	}
	row := f.companyQuantity(t, "Cement")
	if row.Quantity+projectQty != 2500 || row.InTransitQuantity != 0 {
		t.Fatalf("double settlement: %+v, project %+v", row, stock.Items)
	}
	if (winner.Status == domain.DeliveryDelivered) != (projectQty == 1250) {
		t.Fatalf("wrong receiver credit: %+v, project %s", winner, projectQty)
	}
}
func TestInventoryRoundTripUsesExactUnitsAndKeepsDeliverySnapshots(t *testing.T) {
	f := inventorySetup(t)
	m := f.material(t, "Cement", "Mix", 3456)
	d := f.send(t, m.ID, 1234)
	project, company := domain.ProjectDeliveryScope(f.projectID), domain.CompanyDeliveryScope()
	name := "Renamed Cement"
	if _, err := f.inventory.UpdateMaterial(f.ctx, f.stranger, m.ID, domain.MaterialPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	unit := "bags"
	if _, err := f.inventory.UpdateMaterial(f.ctx, f.stranger, m.ID, domain.MaterialPatch{Unit: &unit}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stock unit edited: %v", err)
	}
	if err := f.inventory.DeleteMaterial(f.ctx, f.member, m.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("referenced material deleted: %v", err)
	}
	d, err := f.inventory.ResolveDelivery(f.ctx, f.member, project, d.ID, domain.RecordDelivery)
	if err != nil || len(d.Lines) != 1 || d.Lines[0].Name != "Cement" || d.Lines[0].Quantity != 1234 || d.ReceivedBy == nil || d.ReceivedBy.ID != f.member.UserID {
		t.Fatalf("bad recorded line: %+v %v", d, err)
	}
	back, err := f.inventory.InitiateDelivery(f.ctx, f.member, project, "", []domain.DeliveryLineInput{{MaterialID: m.ID, Quantity: 333}})
	if err != nil {
		t.Fatal(err)
	}
	if back.Direction != domain.ProjectToCompany || back.Lines[0].Name != name || back.SentBy.ID != f.member.UserID {
		t.Fatalf("bad return: %+v", back)
	}
	if _, err := f.inventory.ResolveDelivery(f.ctx, f.manager, company, back.ID, domain.RecordDelivery); err != nil {
		t.Fatal(err)
	}
	row := f.companyQuantity(t, name)
	stock, err := f.inventory.ListProjectStock(f.ctx, f.member, f.projectID, domain.ProjectStockFilter{})
	if err != nil || len(stock.Items) != 1 || stock.Items[0].Quantity != 901 || stock.Items[0].LastDeliveryAt == nil || !stock.Items[0].LastDeliveryAt.Equal(back.SentAt) || row.Quantity != 2555 || row.InTransitQuantity != 0 {
		t.Fatalf("company %+v; project %+v; error %v", row, stock, err)
	}
}
func TestInventorySupplierCannotConsumeReservedRefundHeadroom(t *testing.T) {
	f := inventorySetup(t)
	m := f.material(t, "Max", "Mix", math.MaxInt64)
	d := f.send(t, m.ID, 1000)
	if _, err := f.inventory.AddStock(f.ctx, f.manager, m.ID, 1); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("wanted refund capacity conflict, got %v", err)
	}
	if _, err := f.inventory.ResolveDelivery(f.ctx, f.manager, domain.CompanyDeliveryScope(), d.ID, domain.CancelDelivery); err != nil {
		t.Fatal(err)
	}
	if row := f.companyQuantity(t, "Max"); row.Quantity != math.MaxInt64 || row.InTransitQuantity != 0 {
		t.Fatalf("reservation refund failed: %+v", row)
	}
}

func TestInventoryStockFiltersAndSortsAcrossTiedPages(t *testing.T) {
	f := inventorySetup(t)
	a := f.material(t, "A_B", "Mix", 0)
	b := f.material(t, "a_b", "MIX", 1100)
	c := f.material(t, "A-B", "Mix", 1100)
	d := f.material(t, "A_B Other", "Metals", 1100)
	if _, err := f.inventory.SetReorderPoint(f.ctx, f.manager, b.ID, 1100); err != nil {
		t.Fatal(err)
	}
	catalogue, err := f.inventory.ListMaterials(f.ctx, f.stranger, domain.MaterialFilter{Search: "a_b", Category: "mix", Page: domain.Page{Limit: 1}})
	if err != nil || len(catalogue.Items) != 1 || catalogue.Items[0].ID != a.ID || catalogue.NextCursor == "" {
		t.Fatalf("literal search/tie: %+v %v", catalogue, err)
	}
	next, err := f.inventory.ListMaterials(f.ctx, f.stranger, domain.MaterialFilter{Search: "a_b", Category: "mix", Page: domain.Page{Limit: 1, Cursor: catalogue.NextCursor}})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != b.ID || next.NextCursor != "" {
		t.Fatalf("second tied name: %+v %v", next, err)
	}
	if _, err := f.inventory.ListMaterials(f.ctx, f.stranger, domain.MaterialFilter{Order: "desc", Page: domain.Page{Cursor: catalogue.NextCursor}}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("wrong order cursor: %v", err)
	}
	malformed := domain.EncodeCursor("materials", "name", "asc", "bad\x00name", "1")
	if _, err := f.inventory.ListMaterials(f.ctx, f.stranger, domain.MaterialFilter{Page: domain.Page{Cursor: malformed}}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("malformed sort key: %v", err)
	}
	for _, tc := range []struct {
		sort, order, category string
		status                domain.StockStatus
		want                  []int64
	}{
		{"status", "asc", "", "", []int64{a.ID, b.ID, c.ID, d.ID}},
		{"quantity", "desc", "mix", "", []int64{c.ID, b.ID, a.ID}},
		{"name", "asc", "mix", domain.LowStock, []int64{b.ID}},
		{"name", "asc", "mix", domain.OutOfStock, []int64{a.ID}},
		{"name", "asc", "mix", domain.Available, []int64{c.ID}},
	} {
		filter := domain.CompanyStockFilter{Category: tc.category, Sort: tc.sort, Order: tc.order, Status: tc.status, Page: domain.Page{Limit: 1}}
		got := []int64{}
		for range len(tc.want) + 1 {
			result, err := f.inventory.ListCompanyStock(f.ctx, f.admin, filter)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range result.Items {
				got = append(got, item.ID)
			}
			filter.Page.Cursor = result.NextCursor
			if result.NextCursor == "" {
				break
			}
		}
		if !slices.Equal(got, tc.want) || filter.Page.Cursor != "" {
			t.Fatalf("%+v returned IDs %v", tc, got)
		}
	}
	empty, err := f.inventory.ListCompanyStock(f.ctx, f.manager, domain.CompanyStockFilter{Search: "no match"})
	if err != nil || empty.Items == nil || len(empty.Items) != 0 || empty.NextCursor != "" {
		t.Fatalf("empty screen: %+v %v", empty, err)
	}
}

func TestInventoryProjectStockLastDeliveryUsesNullLastAndStableTieBreak(t *testing.T) {
	f := inventorySetup(t)
	a := f.material(t, "A", "Mix", 2000)
	b := f.material(t, "B", "Mix", 2000)
	c := f.material(t, "C", "Mix", 2000)
	d := f.material(t, "D", "Mix", 2000)
	if err := f.db.Exec(`INSERT INTO stock(store_id,material_id,quantity,last_delivery_at)
  SELECT id, data.material, data.quantity, data.delivered FROM stores
  CROSS JOIN (VALUES (?::bigint, 10, null::timestamptz), (?::bigint, 20, '2026-01-01T00:00:00Z'::timestamptz),
    (?::bigint, 20, '2026-01-01T00:00:00Z'::timestamptz), (?::bigint, 30, null::timestamptz)) data(material,quantity,delivered)
  WHERE project_id = ?`, a.ID, b.ID, c.ID, d.ID, f.projectID).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		sort, order string
		want        []int64
	}{
		{"last_delivery", "asc", []int64{b.ID, c.ID, a.ID, d.ID}},
		{"last_delivery", "desc", []int64{c.ID, b.ID, d.ID, a.ID}},
		{"name", "desc", []int64{d.ID, c.ID, b.ID, a.ID}},
		{"quantity", "asc", []int64{a.ID, b.ID, c.ID, d.ID}},
	} {
		filter := domain.ProjectStockFilter{Category: "mix", Sort: tc.sort, Order: tc.order, Page: domain.Page{Limit: 1}}
		var got []int64
		for range len(tc.want) + 1 {
			result, err := f.inventory.ListProjectStock(f.ctx, f.member, f.projectID, filter)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range result.Items {
				got = append(got, item.ID)
			}
			filter.Page.Cursor = result.NextCursor
			if result.NextCursor == "" {
				break
			}
		}
		if !slices.Equal(got, tc.want) || filter.Page.Cursor != "" {
			t.Fatalf("%+v returned IDs %v", tc, got)
		}
	}
	if _, err := f.inventory.ListProjectStock(f.ctx, f.stranger, f.projectID, domain.ProjectStockFilter{Search: "A"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("guessed stock: %v", err)
	}
}

func TestInventoryDeliveryListFiltersSentBusinessDatesAndPaginatesTies(t *testing.T) {
	f := inventorySetup(t)
	location, err := time.LoadLocation("Asia/Muscat")
	if err != nil {
		t.Fatal(err)
	}
	f.inventory = application.NewInventory(postgres.NewInventoryRepository(f.db), location)
	material := f.material(t, "Cement", "Mix", 10000)
	first, second := f.send(t, material.ID, 1000), f.send(t, material.ID, 1000)
	other, err := f.inventory.InitiateDelivery(f.ctx, f.manager, domain.CompanyDeliveryScope(), f.otherProjectID, []domain.DeliveryLineInput{{MaterialID: material.ID, Quantity: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	// All are tied, at midnight on Jan 3 in the business timezone.
	if err := f.db.Exec("UPDATE deliveries SET sent_at = '2026-01-02T20:00:00Z'").Error; err != nil {
		t.Fatal(err)
	}
	filter := domain.DeliveryFilter{Direction: domain.CompanyToProject, Status: domain.DeliveryInTransit, StartDate: "2026-01-03", EndDate: "2026-01-03", Page: domain.Page{Limit: 1}}
	company := domain.CompanyDeliveryScope()
	page, err := f.inventory.ListDeliveries(f.ctx, f.manager, company, filter)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != other.ID || page.Items[0].Project.ID != f.otherProjectID || len(page.Items[0].Lines) != 1 || page.NextCursor == "" {
		t.Fatalf("bad first delivery page: %+v %v", page, err)
	}
	filter.Page.Cursor = page.NextCursor
	page, err = f.inventory.ListDeliveries(f.ctx, f.manager, company, filter)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != second.ID || page.NextCursor == "" {
		t.Fatalf("bad second delivery page: %+v %v", page, err)
	}
	filter.Page.Cursor = page.NextCursor
	page, err = f.inventory.ListDeliveries(f.ctx, f.manager, company, filter)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != first.ID || page.NextCursor != "" {
		t.Fatalf("bad third delivery page: %+v %v", page, err)
	}
	filter.Page.Cursor = ""
	filter.ProjectID = f.projectID
	page, err = f.inventory.ListDeliveries(f.ctx, f.manager, company, filter)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != second.ID {
		t.Fatalf("bad project filter: %+v %v", page, err)
	}
	filter.ProjectID = ""
	page, err = f.inventory.ListDeliveries(f.ctx, f.member, domain.ProjectDeliveryScope(f.projectID), filter)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != second.ID {
		t.Fatalf("bad project scope: %+v %v", page, err)
	}
	filter.EndDate, filter.StartDate = "2026-01-02", ""
	page, err = f.inventory.ListDeliveries(f.ctx, f.admin, company, filter)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("business date boundary: %+v %v", page, err)
	}
}

func TestInventoryRecordOverflowRollsBackEveryLineAndRejectionRefunds(t *testing.T) {
	f := inventorySetup(t)
	first := f.material(t, "First", "Mix", 1000)
	max := f.material(t, "Max", "Mix", math.MaxInt64)
	d := f.send(t, max.ID, math.MaxInt64)
	project, company := domain.ProjectDeliveryScope(f.projectID), domain.CompanyDeliveryScope()
	if _, err := f.inventory.ResolveDelivery(f.ctx, f.member, project, d.ID, domain.RecordDelivery); err != nil {
		t.Fatal(err)
	}
	if _, err := f.inventory.AddStock(f.ctx, f.manager, max.ID, 1000); err != nil {
		t.Fatal(err)
	}
	d, err := f.inventory.InitiateDelivery(f.ctx, f.manager, company, f.projectID, []domain.DeliveryLineInput{{MaterialID: first.ID, Quantity: 1000}, {MaterialID: max.ID, Quantity: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.inventory.ResolveDelivery(f.ctx, f.member, project, d.ID, domain.RecordDelivery); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("wanted capacity conflict, got %v", err)
	}
	stock, err := f.inventory.ListProjectStock(f.ctx, f.member, f.projectID, domain.ProjectStockFilter{})
	if err != nil || len(stock.Items) != 1 || stock.Items[0].ID != max.ID || stock.Items[0].Quantity != math.MaxInt64 {
		t.Fatalf("receiver partly credited: %+v %v", stock, err)
	}
	if row := f.companyQuantity(t, "First"); row.Quantity != 0 || row.InTransitQuantity != 1000 {
		t.Fatalf("sender reservation partly released: %+v", row)
	}
	rejected, err := f.inventory.ResolveDelivery(f.ctx, f.member, project, d.ID, domain.RejectDelivery)
	if err != nil || rejected.Status != domain.DeliveryRejected || rejected.ReceivedAt != nil || rejected.ReceivedBy != nil {
		t.Fatalf("rejection: %+v %v", rejected, err)
	}
	if row := f.companyQuantity(t, "First"); row.Quantity != 1000 || row.InTransitQuantity != 0 {
		t.Fatalf("sender refund missing: %+v", row)
	}
}

func TestInventoryUnusedCatalogueEntryCanBeUpdatedAndDeleted(t *testing.T) {
	f := inventorySetup(t)
	m := f.material(t, "  Original  ", "Mix", 0)
	if m.Name != "Original" {
		t.Fatalf("name wasn't trimmed: %+v", m)
	}
	category, unit := "Hardware", "pieces"
	updated, err := f.inventory.UpdateMaterial(f.ctx, f.stranger, m.ID, domain.MaterialPatch{Category: &category, Unit: &unit})
	if err != nil || updated.Name != "Original" || updated.Category != "Hardware" || updated.Unit != "pieces" {
		t.Fatalf("patch: %+v %v", updated, err)
	}
	fetched, err := f.inventory.GetMaterial(f.ctx, f.member, m.ID)
	if err != nil || fetched != updated {
		t.Fatalf("material: %+v %v", fetched, err)
	}
	if _, err := f.inventory.SetReorderPoint(f.ctx, f.manager, m.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := f.inventory.DeleteMaterial(f.ctx, f.stranger, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.inventory.GetMaterial(f.ctx, f.member, m.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted material still readable: %v", err)
	}
}

func TestInventoryDeliveryLocalDateAcrossMidnightTransitions(t *testing.T) {
	for _, tc := range []struct {
		zone string
		sent [4]string
	}{
		{"America/Sao_Paulo", [4]string{"2018-11-04T02:30:00Z", "2018-11-04T03:00:00Z", "2018-11-05T01:30:00Z", "2018-11-05T02:00:00Z"}},
		{"America/Havana", [4]string{"2018-11-04T03:59:00Z", "2018-11-04T04:30:00Z", "2018-11-04T05:30:00Z", "2018-11-05T05:00:00Z"}},
	} {
		t.Run(tc.zone, func(t *testing.T) {
			f := inventorySetup(t)
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			f.inventory = application.NewInventory(postgres.NewInventoryRepository(f.db), loc)
			material := f.material(t, "Cement", "Mix", 10000)
			var ids [4]int64
			for i, stamp := range tc.sent {
				delivery := f.send(t, material.ID, 1000)
				ids[i] = delivery.ID
				if err := f.db.Exec("UPDATE deliveries SET sent_at = ?::timestamptz WHERE id = ?", stamp, delivery.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			const day = "2018-11-04"
			for _, scope := range []struct {
				name  string
				actor domain.Actor
				scope domain.DeliveryScope
			}{
				{"company", f.manager, domain.CompanyDeliveryScope()},
				{"project", f.member, domain.ProjectDeliveryScope(f.projectID)},
			} {
				for _, bounds := range []struct {
					name, start, end string
					want             []int64
				}{
					{"single-day", day, day, []int64{ids[2], ids[1]}},
					{"start-only", day, "", []int64{ids[3], ids[2], ids[1]}},
					{"end-only", "", day, []int64{ids[2], ids[1], ids[0]}},
				} {
					t.Run(scope.name+"/"+bounds.name, func(t *testing.T) {
						result, err := f.inventory.ListDeliveries(f.ctx, scope.actor, scope.scope, domain.DeliveryFilter{StartDate: bounds.start, EndDate: bounds.end})
						if err != nil {
							t.Fatal(err)
						}
						got := make([]int64, 0, len(result.Items))
						for _, item := range result.Items {
							got = append(got, item.ID)
						}
						if !slices.Equal(got, bounds.want) {
							t.Fatalf("got IDs %v, want %v (before/within/within/after IDs %v)", got, bounds.want, ids)
						}
					})
				}
			}
		})
	}
}
