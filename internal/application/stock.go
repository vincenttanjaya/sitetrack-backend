package application

import (
	"context"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func (s *InventoryService) ListCompanyStock(ctx context.Context, actor domain.Actor, f domain.CompanyStockFilter) (domain.PageResult[domain.CompanyStockItem], error) {
	if err := companyStoreActor(actor); err != nil {
		return domain.PageResult[domain.CompanyStockItem]{}, err
	}
	if f.Status != "" && !f.Status.Valid() {
		return domain.PageResult[domain.CompanyStockItem]{}, domain.Invalid("invalid stock status")
	}
	if err := inventoryListOptions(&f.Search, &f.Category, &f.Sort, &f.Order, "name", "quantity", "status"); err != nil {
		return domain.PageResult[domain.CompanyStockItem]{}, err
	}
	return s.repo.ListCompanyStock(ctx, f)
}

func (s *InventoryService) ListProjectStock(ctx context.Context, actor domain.Actor, projectID string, f domain.ProjectStockFilter) (domain.PageResult[domain.ProjectStockItem], error) {
	if err := inventoryActor(actor); err != nil {
		return domain.PageResult[domain.ProjectStockItem]{}, err
	}
	if !inventoryProjectIDValid(projectID) {
		return domain.PageResult[domain.ProjectStockItem]{}, domain.ErrNotFound
	}
	if err := inventoryListOptions(&f.Search, &f.Category, &f.Sort, &f.Order, "name", "quantity", "last_delivery"); err != nil {
		return domain.PageResult[domain.ProjectStockItem]{}, err
	}
	return s.repo.ListProjectStock(ctx, actor, projectID, f)
}

func (s *InventoryService) AddStock(ctx context.Context, actor domain.Actor, id int64, quantity domain.Quantity) (domain.CompanyStockItem, error) {
	if err := companyStoreActor(actor); err != nil {
		return domain.CompanyStockItem{}, err
	}
	if err := inventoryID(id); err != nil {
		return domain.CompanyStockItem{}, err
	}
	if quantity <= 0 {
		return domain.CompanyStockItem{}, domain.Invalid("quantity must be positive")
	}
	return s.repo.AddStock(ctx, id, quantity)
}

func (s *InventoryService) SetReorderPoint(ctx context.Context, actor domain.Actor, id int64, point domain.Quantity) (domain.CompanyStockItem, error) {
	if err := companyStoreActor(actor); err != nil {
		return domain.CompanyStockItem{}, err
	}
	if err := inventoryID(id); err != nil {
		return domain.CompanyStockItem{}, err
	}
	if point < 0 {
		return domain.CompanyStockItem{}, domain.Invalid("reorder point must be nonnegative")
	}
	return s.repo.SetReorderPoint(ctx, id, point)
}
