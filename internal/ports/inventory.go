package ports

import (
	"context"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

type InventoryRepository interface {
	CreateMaterial(context.Context, domain.MaterialInput) (domain.Material, error)
	GetMaterial(context.Context, int64) (domain.Material, error)
	UpdateMaterial(context.Context, int64, domain.MaterialPatch) (domain.Material, error)
	DeleteMaterial(context.Context, int64) error
	ListMaterials(context.Context, domain.MaterialFilter) (domain.PageResult[domain.Material], error)
	ListCompanyStock(context.Context, domain.CompanyStockFilter) (domain.PageResult[domain.CompanyStockItem], error)
	ListProjectStock(context.Context, domain.Actor, string, domain.ProjectStockFilter) (domain.PageResult[domain.ProjectStockItem], error)
	AddStock(context.Context, int64, domain.Quantity) (domain.CompanyStockItem, error)
	SetReorderPoint(context.Context, int64, domain.Quantity) (domain.CompanyStockItem, error)
	InitiateDelivery(context.Context, domain.Actor, domain.DeliveryScope, string, []domain.DeliveryLineInput) (domain.Delivery, error)
	GetDelivery(context.Context, domain.Actor, domain.DeliveryScope, int64) (domain.Delivery, error)
	ListDeliveries(context.Context, domain.Actor, domain.DeliveryScope, domain.DeliveryFilter) (domain.PageResult[domain.Delivery], error)
	ResolveDelivery(context.Context, domain.Actor, domain.DeliveryScope, int64, domain.DeliveryAction) (domain.Delivery, error)
}
