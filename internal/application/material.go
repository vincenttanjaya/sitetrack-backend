package application

import (
	"context"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func (s *InventoryService) CreateMaterial(ctx context.Context, actor domain.Actor, input domain.MaterialInput) (domain.Material, error) {
	if err := inventoryActor(actor); err != nil {
		return domain.Material{}, err
	}
	var err error
	if input.Name, err = inventoryText(input.Name, "name", 200, false); err != nil {
		return domain.Material{}, err
	}
	if input.Category, err = inventoryText(input.Category, "category", 120, false); err != nil {
		return domain.Material{}, err
	}
	if input.Unit, err = inventoryText(input.Unit, "unit", 50, false); err != nil {
		return domain.Material{}, err
	}
	return s.repo.CreateMaterial(ctx, input)
}

func (s *InventoryService) GetMaterial(ctx context.Context, actor domain.Actor, id int64) (domain.Material, error) {
	if err := inventoryActor(actor); err != nil {
		return domain.Material{}, err
	}
	if err := inventoryID(id); err != nil {
		return domain.Material{}, err
	}
	return s.repo.GetMaterial(ctx, id)
}

func (s *InventoryService) UpdateMaterial(ctx context.Context, actor domain.Actor, id int64, patch domain.MaterialPatch) (domain.Material, error) {
	if err := inventoryActor(actor); err != nil {
		return domain.Material{}, err
	}
	if err := inventoryID(id); err != nil {
		return domain.Material{}, err
	}
	if patch.Name == nil && patch.Category == nil && patch.Unit == nil {
		return domain.Material{}, domain.Invalid("provide at least one material field")
	}
	for _, field := range []struct {
		value **string
		name  string
		max   int
	}{
		{&patch.Name, "name", 200}, {&patch.Category, "category", 120}, {&patch.Unit, "unit", 50},
	} {
		if *field.value == nil {
			continue
		}
		value, err := inventoryText(**field.value, field.name, field.max, false)
		if err != nil {
			return domain.Material{}, err
		}
		*field.value = &value
	}
	return s.repo.UpdateMaterial(ctx, id, patch)
}

func (s *InventoryService) DeleteMaterial(ctx context.Context, actor domain.Actor, id int64) error {
	if err := inventoryActor(actor); err != nil {
		return err
	}
	if err := inventoryID(id); err != nil {
		return err
	}
	return s.repo.DeleteMaterial(ctx, id)
}

func (s *InventoryService) ListMaterials(ctx context.Context, actor domain.Actor, f domain.MaterialFilter) (domain.PageResult[domain.Material], error) {
	if err := inventoryActor(actor); err != nil {
		return domain.PageResult[domain.Material]{}, err
	}
	if err := inventoryListOptions(&f.Search, &f.Category, &f.Sort, &f.Order, "name"); err != nil {
		return domain.PageResult[domain.Material]{}, err
	}
	return s.repo.ListMaterials(ctx, f)
}
