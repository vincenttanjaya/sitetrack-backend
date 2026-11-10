package postgres

import (
	"context"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *InventoryRepository) CreateMaterial(ctx context.Context, in domain.MaterialInput) (domain.Material, error) {
	item := domain.Material{Name: in.Name, Category: in.Category, Unit: in.Unit}
	err := r.db.WithContext(ctx).Table("materials").Create(&item).Error
	return item, dbError(err)
}

func (r *InventoryRepository) GetMaterial(ctx context.Context, id int64) (domain.Material, error) {
	return inventoryMaterial(r.db.WithContext(ctx), id, "")
}

func inventoryMaterial(tx *gorm.DB, id int64, lock string) (domain.Material, error) {
	var item domain.Material
	q := tx.Table("materials").Where("id = ?", id)
	if lock != "" {
		q = q.Clauses(clause.Locking{Strength: lock})
	}
	err := q.Take(&item).Error
	return item, dbError(err)
}

func materialInUse(tx *gorm.DB, id int64) (bool, error) {
	var used bool
	err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM stock WHERE material_id = ?
  AND (quantity > 0 OR outbound_quantity > 0 OR reorder_point > 0))
  OR EXISTS (SELECT 1 FROM delivery_lines WHERE material_id = ?)`, id, id).Row().Scan(&used)
	return used, dbError(err)
}

func (r *InventoryRepository) UpdateMaterial(ctx context.Context, id int64, in domain.MaterialPatch) (domain.Material, error) {
	var item domain.Material
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		item, err = inventoryMaterial(tx, id, "UPDATE")
		if err != nil {
			return err
		}
		changes := make(map[string]any, 3)
		if in.Name != nil {
			item.Name = *in.Name
			changes["name"] = item.Name
		}
		if in.Category != nil {
			item.Category = *in.Category
			changes["category"] = item.Category
		}
		if in.Unit != nil && *in.Unit != item.Unit {
			used, err := materialInUse(tx, id)
			if err != nil {
				return err
			}
			if used {
				return domain.Conflict("unit cannot change after a material is used")
			}
			item.Unit = *in.Unit
			changes["unit"] = item.Unit
		}
		if len(changes) == 0 {
			return nil
		}
		return tx.Table("materials").Where("id = ?", id).Updates(changes).Error
	})
	return item, dbError(err)
}

func (r *InventoryRepository) DeleteMaterial(ctx context.Context, id int64) error {
	return dbError(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := inventoryMaterial(tx, id, "UPDATE"); err != nil {
			return err
		}
		used, err := materialInUse(tx, id)
		if err != nil {
			return err
		}
		if used {
			return domain.Conflict("material has stock, a reorder point, or delivery history")
		}
		if err := tx.Exec("DELETE FROM stock WHERE material_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Exec("DELETE FROM materials WHERE id = ?", id).Error
	}))
}

type materialListRow struct {
	domain.Material
	SortName string
}

func (r *InventoryRepository) ListMaterials(ctx context.Context, f domain.MaterialFilter) (domain.PageResult[domain.Material], error) {
	const scope = "materials"
	q := r.db.WithContext(ctx).Table("materials").Select("id, name, category, unit, lower(name) AS sort_name")
	q = inventoryMaterialFilters(q, f.Search, f.Category)
	q, err := inventoryOrder(q, f.Page, scope, f.Sort, f.Order, inventorySortKey{column: "lower(name)", parse: stockNameKey})
	if err != nil {
		return domain.PageResult[domain.Material]{}, err
	}
	var rows []materialListRow
	if err := q.Scan(&rows).Error; err != nil {
		return domain.PageResult[domain.Material]{}, dbError(err)
	}
	return inventoryPage(rows, f.Page, scope, f.Sort, f.Order, func(row materialListRow) domain.Material { return row.Material }, func(row materialListRow) (string, int64) { return row.SortName, row.ID }), nil
}
