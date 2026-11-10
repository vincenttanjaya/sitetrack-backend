package migrations_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/adapters/postgres"
	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/migrations"
)

const inventoryFixture = `
INSERT INTO users (id,name,email,password_hash,role) VALUES
 (1,'Manager','manager@example.test','unused','company_store_manager'),
 (2,'Member','member@example.test','unused','user');
INSERT INTO projects (number,name,start_date,end_date) VALUES (1,'Site','2026-01-01','2026-12-31');
INSERT INTO project_members (project_id,user_id,is_admin) VALUES ('PRJ-0001',2,true);
INSERT INTO stores (project_id) VALUES ('PRJ-0001');
INSERT INTO materials (id,name,category,unit) VALUES (1,'Cement','Building','kg'),(2,'Sand','Building','kg'),(3,'Steel','Building','kg');
INSERT INTO stock (store_id,material_id,quantity) VALUES (1,1,90000),(1,2,50000),(2,1,40000),(2,2,30000);
INSERT INTO deliveries (id,project_id,direction,sent_by) VALUES
 (1,'PRJ-0001','company_to_project',1),(2,'PRJ-0001','company_to_project',1),(3,'PRJ-0001','company_to_project',1),
 (4,'PRJ-0001','project_to_company',2),(5,'PRJ-0001','project_to_company',2),(6,'PRJ-0001','project_to_company',2),
 (7,'PRJ-0001','company_to_project',1);
UPDATE deliveries SET status='cancelled', resolved_at=sent_at, resolved_by=1 WHERE id=7;`

const lineValues = "(1,1,10000),(1,2,7000),(2,1,20000),(3,1,30000),(4,1,4000),(5,1,5000),(5,3,2000),(6,1,6000),(7,1,120000)"

func TestUpgradeReconcilesInTransitReservations(t *testing.T) {
	for _, tc := range []struct {
		name, through, liveReserves string
	}{
		{"from_initial", initial, ""},
		{"from_inventory_history", inventoryHistory, "UPDATE stock SET outbound_quantity=7000 WHERE store_id=1 AND material_id=2"},
		{"already_correct", inventoryHistory, `
		 UPDATE stock SET outbound_quantity=CASE
		  WHEN store_id=1 AND material_id=1 THEN 60000 WHEN store_id=1 AND material_id=2 THEN 7000
		  WHEN store_id=2 AND material_id=1 THEN 15000 ELSE 0 END;
		 INSERT INTO stock (store_id,material_id,outbound_quantity,last_delivery_at)
		  SELECT 2,3,2000,sent_at FROM deliveries WHERE id=5`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, pool := upgradeFixture(t, tc.through)
			execUpgradeSQL(t, pool, inventoryFixture)
			if tc.through == initial {
				execUpgradeSQL(t, pool, "INSERT INTO delivery_lines (delivery_id,material_id,quantity) VALUES "+lineValues)
			} else {
				execUpgradeSQL(t, pool, `INSERT INTO delivery_lines (delivery_id,material_id,quantity,material_name,material_category,material_unit)
				 SELECT delivery_id,material_id,quantity,'Original snapshot','Building','kg' FROM (VALUES `+lineValues+`) v(delivery_id,material_id,quantity)`)
				execUpgradeSQL(t, pool, tc.liveReserves)
			}
			applyUpgrade(t, pool)
			applyUpgrade(t, pool)

			inv := application.NewInventory(postgres.NewInventoryRepository(db), time.UTC)
			manager := domain.Actor{UserID: 1, Role: domain.RoleStoreManager}
			member := domain.Actor{UserID: 2, Role: domain.RoleUser}
			company := domain.CompanyDeliveryScope()
			project := domain.ProjectDeliveryScope("PRJ-0001")
			stock, err := inv.ListCompanyStock(ctx, manager, domain.CompanyStockFilter{})
			if err != nil {
				t.Fatal(err)
			}
			if len(stock.Items) != 3 || stock.Items[0].ID != 1 ||
				stock.Items[0].Quantity != 90000 || stock.Items[0].InTransitQuantity != 60000 {
				t.Errorf("unexpected company stock after upgrade: %+v", stock.Items)
			}
			expectStock(t, pool, [][4]int64{{1, 1, 90000, 60000}, {1, 2, 50000, 7000}, {2, 1, 40000, 15000}, {2, 2, 30000, 0}, {2, 3, 0, 2000}})
			var lastDispatchSaved bool
			err = pool.QueryRow(`SELECT k.last_delivery_at IS NOT DISTINCT FROM d.sent_at
			 FROM stock k CROSS JOIN deliveries d WHERE k.store_id=2 AND k.material_id=3 AND d.id=5`).Scan(&lastDispatchSaved)
			if err != nil || !lastDispatchSaved {
				t.Errorf("missing sender row did not retain its dispatch date: %v", err)
			}
			for _, action := range []struct {
				id    int64
				actor domain.Actor
				scope domain.DeliveryScope
				kind  domain.DeliveryAction
			}{
				{1, member, project, domain.RecordDelivery},
				{2, member, project, domain.RejectDelivery},
				{3, manager, company, domain.CancelDelivery},
				{4, manager, company, domain.RecordDelivery},
				{5, manager, company, domain.RejectDelivery},
				{6, member, project, domain.CancelDelivery},
			} {
				result, err := inv.ResolveDelivery(ctx, action.actor, action.scope, action.id, action.kind)
				if err != nil || result.Status != action.kind.Result() {
					t.Errorf("delivery %d %s: status=%s, error=%v", action.id, action.kind, result.Status, err)
				}
			}
			applyUpgrade(t, pool)
			expectStock(t, pool, [][4]int64{{1, 1, 144000, 0}, {1, 2, 50000, 0}, {2, 1, 61000, 0}, {2, 2, 37000, 0}, {2, 3, 2000, 0}})
			var status, snapshot string
			err = pool.QueryRow(`SELECT d.status,l.material_name FROM deliveries d JOIN delivery_lines l ON l.delivery_id=d.id WHERE d.id=7`).Scan(&status, &snapshot)
			wantSnapshot := "Original snapshot"
			if tc.through == initial {
				wantSnapshot = "Cement"
			}
			if err != nil || status != "cancelled" || snapshot != wantSnapshot {
				t.Errorf("terminal history: status=%q, snapshot=%q, error=%v", status, snapshot, err)
			}
		})
	}
}

func TestUpgradeRejectsImpossibleRefundCapacityAtomically(t *testing.T) {
	for _, tc := range []struct {
		name, setup, outbound string
	}{
		{"on_hand_plus_reserve_overflow", "UPDATE stock SET quantity=9223372036854775807 WHERE store_id=1 AND material_id=1", "(1,1,1),(4,1,10)"},
		{"reserve_sum_overflow", "", "(1,1,9223372036854775807),(2,1,1),(4,1,10)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, pool := upgradeFixture(t, inventoryHistory)
			execUpgradeSQL(t, pool, inventoryFixture)
			if tc.setup != "" {
				execUpgradeSQL(t, pool, tc.setup)
			}
			execUpgradeSQL(t, pool, `INSERT INTO delivery_lines (delivery_id,material_id,quantity,material_name,material_category,material_unit)
			 SELECT delivery_id,material_id,quantity,'Original snapshot','Building','kg' FROM (VALUES `+tc.outbound+`) v(delivery_id,material_id,quantity)`)
			err := migrations.Apply(context.Background(), pool)
			if err == nil || !strings.Contains(err.Error(), "refund capacity exceeded for store 1, material 1") {
				t.Errorf("expected refund capacity diagnostic, got %v", err)
			}
			var migrationsCount, modifiedReserves int
			if err := pool.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&migrationsCount); err != nil || migrationsCount != 2 {
				t.Errorf("applied migrations: got %d, want 2 (error: %v)", migrationsCount, err)
			}
			if err := pool.QueryRow("SELECT count(*) FROM stock WHERE outbound_quantity<>0").Scan(&modifiedReserves); err != nil || modifiedReserves != 0 {
				t.Errorf("modified reserves: got %d, want 0 (error: %v)", modifiedReserves, err)
			}
			onHand := int64(90000)
			if tc.setup != "" {
				onHand = 9223372036854775807
			}
			expectStock(t, pool, [][4]int64{{1, 1, onHand, 0}, {1, 2, 50000, 0}, {2, 1, 40000, 0}, {2, 2, 30000, 0}})
		})
	}
}

func execUpgradeSQL(t *testing.T, pool *sql.DB, query string) {
	t.Helper()
	if _, err := pool.Exec(query); err != nil {
		t.Fatal(err)
	}
}

func applyUpgrade(t *testing.T, pool *sql.DB) {
	t.Helper()
	if err := migrations.Apply(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
}

func expectStock(t *testing.T, pool *sql.DB, balances [][4]int64) {
	t.Helper()
	for _, balance := range balances {
		storeID, materialID := balance[0], balance[1]
		var available, reserved int64
		err := pool.QueryRow("SELECT quantity, outbound_quantity FROM stock WHERE store_id=$1 AND material_id=$2", storeID, materialID).Scan(&available, &reserved)
		if err != nil || available != balance[2] || reserved != balance[3] {
			t.Errorf("stock %d/%d: got available=%d, reserved=%d; want %d, %d (error: %v)",
				storeID, materialID, available, reserved, balance[2], balance[3], err)
		}
	}
}
