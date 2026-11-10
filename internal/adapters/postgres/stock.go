package postgres

import (
	"context"
	"strconv"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"gorm.io/gorm"
)

const companyStockColumns = `m.id, m.name, m.category, m.unit, lower(m.name) AS sort_name,
 COALESCE(s.quantity, 0) AS quantity, COALESCE(s.outbound_quantity, 0) AS in_transit_quantity,
 COALESCE(s.reorder_point, 0) AS reorder_point,
 CASE WHEN COALESCE(s.quantity, 0) = 0 THEN 'out_of_stock'
 WHEN s.quantity <= s.reorder_point THEN 'low_stock' ELSE 'available' END AS status,
 CASE WHEN COALESCE(s.quantity, 0) = 0 THEN 0
 WHEN s.quantity <= s.reorder_point THEN 1 ELSE 2 END AS status_rank`

type companyStockRow struct {
	domain.CompanyStockItem
	SortName   string
	StatusRank int
}

func (r *InventoryRepository) ListCompanyStock(ctx context.Context, f domain.CompanyStockFilter) (domain.PageResult[domain.CompanyStockItem], error) {
	const scope = "company-stock"
	db := r.db.WithContext(ctx)
	base := db.Table("materials AS m").Select(companyStockColumns).
		Joins("LEFT JOIN stock s ON s.material_id = m.id AND s.store_id = (SELECT id FROM stores WHERE project_id IS NULL)")
	q := inventoryMaterialFilters(db.Table("(?) AS inventory", base), f.Search, f.Category)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	key := inventorySortKey{column: "sort_name", parse: stockNameKey}
	switch f.Sort {
	case "quantity":
		key = inventorySortKey{column: "quantity", parse: stockNumberKey}
	case "status":
		key = inventorySortKey{column: "status_rank", parse: stockStatusKey}
	}
	q, err := inventoryOrder(q, f.Page, scope, f.Sort, f.Order, key)
	if err != nil {
		return domain.PageResult[domain.CompanyStockItem]{}, err
	}
	var rows []companyStockRow
	if err := q.Scan(&rows).Error; err != nil {
		return domain.PageResult[domain.CompanyStockItem]{}, dbError(err)
	}
	return inventoryPage(rows, f.Page, scope, f.Sort, f.Order, func(row companyStockRow) domain.CompanyStockItem { return row.CompanyStockItem }, func(row companyStockRow) (string, int64) {
		switch f.Sort {
		case "quantity":
			return strconv.FormatInt(int64(row.Quantity), 10), row.ID
		case "status":
			return strconv.Itoa(row.StatusRank), row.ID
		default:
			return row.SortName, row.ID
		}
	}), nil
}

func stockStatusKey(raw string) (any, error) {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || value > 2 {
		return nil, domain.Invalid("invalid cursor")
	}
	return value, nil
}

type projectStockRow struct {
	domain.ProjectStockItem
	SortName string
}

func (r *InventoryRepository) ListProjectStock(ctx context.Context, actor domain.Actor, projectID string, f domain.ProjectStockFilter) (domain.PageResult[domain.ProjectStockItem], error) {
	scope := "project-stock:" + projectID
	var rows []projectStockRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := projectAccess(tx, actor, projectID, false); err != nil {
			return err
		}
		q := tx.Table("stock AS s").Select("m.id, m.name, m.category, m.unit, lower(m.name) AS sort_name, s.quantity, s.last_delivery_at").
			Joins("JOIN materials m ON m.id = s.material_id").Where("s.store_id = (SELECT id FROM stores WHERE project_id = ?)", projectID)
		q = inventoryMaterialFilters(q, f.Search, f.Category)
		key := inventorySortKey{column: "lower(m.name)", parse: stockNameKey}
		switch f.Sort {
		case "quantity":
			key = inventorySortKey{column: "s.quantity", parse: stockNumberKey}
		case "last_delivery":
			key = inventorySortKey{column: "s.last_delivery_at", parse: inventoryTimeKey, nullable: true}
		}
		q, err := inventoryOrder(q, f.Page, scope, f.Sort, f.Order, key)
		if err != nil {
			return err
		}
		return q.Scan(&rows).Error
	}, &inventoryReadSnapshot)
	if err != nil {
		return domain.PageResult[domain.ProjectStockItem]{}, dbError(err)
	}
	return inventoryPage(rows, f.Page, scope, f.Sort, f.Order, func(row projectStockRow) domain.ProjectStockItem {
		if row.LastDeliveryAt != nil {
			at := row.LastDeliveryAt.UTC()
			row.LastDeliveryAt = &at
		}
		return row.ProjectStockItem
	}, func(row projectStockRow) (string, int64) {
		switch f.Sort {
		case "quantity":
			return strconv.FormatInt(int64(row.Quantity), 10), row.ID
		case "last_delivery":
			if row.LastDeliveryAt == nil {
				return "", row.ID
			}
			return row.LastDeliveryAt.UTC().Format(time.RFC3339Nano), row.ID
		default:
			return row.SortName, row.ID
		}
	}), nil
}

func (r *InventoryRepository) AddStock(ctx context.Context, materialID int64, quantity domain.Quantity) (domain.CompanyStockItem, error) {
	return r.changeCompanyStock(ctx, materialID, quantity, false)
}

func (r *InventoryRepository) SetReorderPoint(ctx context.Context, materialID int64, quantity domain.Quantity) (domain.CompanyStockItem, error) {
	return r.changeCompanyStock(ctx, materialID, quantity, true)
}

func (r *InventoryRepository) changeCompanyStock(ctx context.Context, materialID int64, quantity domain.Quantity, reorder bool) (domain.CompanyStockItem, error) {
	var row companyStockRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := inventoryMaterial(tx, materialID, "SHARE"); err != nil {
			return err
		}
		storeID, err := inventoryStoreID(tx, "")
		if err != nil {
			return err
		}
		statement := `INSERT INTO stock (store_id,material_id,quantity) VALUES (?,?,?)
   ON CONFLICT (store_id,material_id) DO UPDATE SET quantity = stock.quantity + EXCLUDED.quantity`
		if reorder {
			statement = `INSERT INTO stock (store_id,material_id,reorder_point) VALUES (?,?,?)
    ON CONFLICT (store_id,material_id) DO UPDATE SET reorder_point = EXCLUDED.reorder_point`
		}
		if err := tx.Exec(statement, storeID, materialID, int64(quantity)).Error; err != nil {
			return err
		}
		return tx.Table("materials AS m").Select(companyStockColumns).
			Joins("JOIN stock s ON s.material_id = m.id AND s.store_id = ?", storeID).Where("m.id = ?", materialID).Take(&row).Error
	})
	return row.CompanyStockItem, inventoryWriteError(err)
}

func inventoryStoreID(tx *gorm.DB, projectID string) (int64, error) {
	q := tx.Table("stores").Select("id")
	if projectID == "" {
		q = q.Where("project_id IS NULL")
	} else {
		q = q.Where("project_id = ?", projectID)
	}
	var id int64
	err := q.Row().Scan(&id)
	return id, dbError(err)
}
