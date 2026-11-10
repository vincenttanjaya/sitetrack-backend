package application

import (
	"context"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func deliveryActor(actor domain.Actor, scope domain.DeliveryScope) error {
	if scope.IsCompany() {
		return companyStoreActor(actor)
	}
	if err := inventoryActor(actor); err != nil {
		return err
	}
	if !inventoryProjectIDValid(scope.ProjectID) {
		return domain.ErrNotFound
	}
	return nil
}

func (s *InventoryService) InitiateDelivery(ctx context.Context, actor domain.Actor, scope domain.DeliveryScope, destinationProject string, lines []domain.DeliveryLineInput) (domain.Delivery, error) {
	if err := deliveryActor(actor, scope); err != nil {
		return domain.Delivery{}, err
	}
	if scope.IsCompany() {
		if !inventoryProjectIDValid(destinationProject) {
			return domain.Delivery{}, domain.Invalid("valid project_id is required")
		}
	} else if destinationProject != "" {
		return domain.Delivery{}, domain.Invalid("project deliveries can only be sent to the company store")
	}
	if len(lines) == 0 || len(lines) > domain.MaxDeliveryLines {
		return domain.Delivery{}, domain.Invalid("provide between 1 and 100 delivery lines")
	}
	seen := make(map[int64]bool, len(lines))
	for _, line := range lines {
		if line.MaterialID <= 0 || line.Quantity <= 0 {
			return domain.Delivery{}, domain.Invalid("each delivery line requires a material ID and positive quantity")
		}
		if seen[line.MaterialID] {
			return domain.Delivery{}, domain.Invalid("duplicate material in delivery")
		}
		seen[line.MaterialID] = true
	}
	return s.repo.InitiateDelivery(ctx, actor, scope, destinationProject, lines)
}

func (s *InventoryService) GetDelivery(ctx context.Context, actor domain.Actor, scope domain.DeliveryScope, id int64) (domain.Delivery, error) {
	if err := deliveryActor(actor, scope); err != nil {
		return domain.Delivery{}, err
	}
	if err := inventoryID(id); err != nil {
		return domain.Delivery{}, err
	}
	return s.repo.GetDelivery(ctx, actor, scope, id)
}

func (s *InventoryService) ListDeliveries(ctx context.Context, actor domain.Actor, scope domain.DeliveryScope, f domain.DeliveryFilter) (domain.PageResult[domain.Delivery], error) {
	if err := deliveryActor(actor, scope); err != nil {
		return domain.PageResult[domain.Delivery]{}, err
	}
	if f.Direction != "" && !f.Direction.Valid() {
		return domain.PageResult[domain.Delivery]{}, domain.Invalid("invalid delivery direction")
	}
	if f.Status != "" && !f.Status.Valid() {
		return domain.PageResult[domain.Delivery]{}, domain.Invalid("invalid delivery status")
	}
	if f.ProjectID != "" {
		if !scope.IsCompany() {
			return domain.PageResult[domain.Delivery]{}, domain.Invalid("project_id is only a company delivery filter")
		}
		if !inventoryProjectIDValid(f.ProjectID) {
			return domain.PageResult[domain.Delivery]{}, domain.Invalid("invalid project_id")
		}
	}
	f.SentOnOrAfter, f.SentBefore = nil, nil
	for _, field := range []struct {
		raw     string
		target  **time.Time
		nextDay bool
	}{
		{f.StartDate, &f.SentOnOrAfter, false}, {f.EndDate, &f.SentBefore, true},
	} {
		if field.raw == "" {
			continue
		}
		parsed, err := domain.ParseDate(field.raw)
		if err != nil {
			return domain.PageResult[domain.Delivery]{}, err
		}
		start := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, s.location)
		if field.nextDay {
			start = start.AddDate(0, 0, 1)
		}
		*field.target = &start
	}
	if f.SentOnOrAfter != nil && f.SentBefore != nil && !f.SentOnOrAfter.Before(*f.SentBefore) {
		return domain.PageResult[domain.Delivery]{}, domain.Invalid("end date precedes start date")
	}
	return s.repo.ListDeliveries(ctx, actor, scope, f)
}

func (s *InventoryService) ResolveDelivery(ctx context.Context, actor domain.Actor, scope domain.DeliveryScope, id int64, action domain.DeliveryAction) (domain.Delivery, error) {
	if err := deliveryActor(actor, scope); err != nil {
		return domain.Delivery{}, err
	}
	if err := inventoryID(id); err != nil {
		return domain.Delivery{}, err
	}
	if action.Result() == "" {
		return domain.Delivery{}, domain.Invalid("invalid delivery action")
	}
	return s.repo.ResolveDelivery(ctx, actor, scope, id, action)
}
