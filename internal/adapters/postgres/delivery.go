package postgres

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type deliveryRow struct {
	ID           int64
	ProjectID    string
	ProjectName  string
	Direction    domain.DeliveryDirection
	Status       domain.DeliveryStatus
	SentAt       time.Time
	SenderID     int64
	SenderName   string
	ResolvedAt   *time.Time
	ResolverID   *int64
	ResolverName *string
}

type deliveryLineRow struct {
	DeliveryID int64
	domain.DeliveryLine
}

const deliveryColumns = `d.id, d.project_id, p.name AS project_name, d.direction, d.status,
 d.sent_at, d.sent_by AS sender_id, sender.name AS sender_name,
 d.resolved_at, d.resolved_by AS resolver_id, resolver.name AS resolver_name`

func deliveryQuery(tx *gorm.DB, scope domain.DeliveryScope) *gorm.DB {
	q := tx.Table("deliveries AS d").Select(deliveryColumns).
		Joins("JOIN projects p ON p.id = d.project_id").
		Joins("JOIN users sender ON sender.id = d.sent_by").
		Joins("LEFT JOIN users resolver ON resolver.id = d.resolved_by")
	if !scope.IsCompany() {
		q = q.Where("d.project_id = ?", scope.ProjectID)
	}
	return q
}

func deliveryLineRows(tx *gorm.DB, ids []int64) ([]deliveryLineRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []deliveryLineRow
	err := tx.Table("delivery_lines").Select(`delivery_id, material_id, material_name AS name,
  material_category AS category, material_unit AS unit, quantity`).
		Where("delivery_id IN ?", ids).Order("delivery_id, material_id").Scan(&rows).Error
	return rows, dbError(err)
}

func deliveryResponse(row deliveryRow, lines []domain.DeliveryLine, scope domain.DeliveryScope) domain.Delivery {
	if lines == nil {
		lines = []domain.DeliveryLine{}
	}
	out := domain.Delivery{ID: row.ID, Project: domain.DeliveryProject{ID: row.ProjectID, Name: row.ProjectName},
		Direction: row.Direction, Status: row.Status, Lines: lines, SentAt: row.SentAt.UTC(),
		SentBy:         domain.DeliveryPerson{ID: row.SenderID, Name: row.SenderName},
		AllowedActions: []domain.DeliveryAction{},
	}
	if row.ResolverID != nil && row.ResolverName != nil && row.ResolvedAt != nil {
		resolvedAt := row.ResolvedAt.UTC()
		out.ResolvedAt = &resolvedAt
		out.ResolvedBy = &domain.DeliveryPerson{ID: *row.ResolverID, Name: *row.ResolverName}
	}
	switch row.Status {
	case domain.DeliveryDelivered:
		out.ReceivedAt = out.ResolvedAt
		out.ReceivedBy = out.ResolvedBy
	case domain.DeliveryInTransit:
		if scope.CanResolve(row.Direction, domain.CancelDelivery) {
			out.AllowedActions = []domain.DeliveryAction{domain.CancelDelivery}
		} else {
			out.AllowedActions = []domain.DeliveryAction{domain.RecordDelivery, domain.RejectDelivery}
		}
	}
	return out
}

func deliveryDetail(tx *gorm.DB, scope domain.DeliveryScope, id int64) (domain.Delivery, error) {
	var row deliveryRow
	if err := deliveryQuery(tx, scope).Where("d.id = ?", id).Take(&row).Error; err != nil {
		return domain.Delivery{}, dbError(err)
	}
	lines, err := deliveryLineRows(tx, []int64{id})
	if err != nil {
		return domain.Delivery{}, err
	}
	items := make([]domain.DeliveryLine, 0, len(lines))
	for _, line := range lines {
		items = append(items, line.DeliveryLine)
	}
	return deliveryResponse(row, items, scope), nil
}

func (r *InventoryRepository) GetDelivery(ctx context.Context, actor domain.Actor, scope domain.DeliveryScope, id int64) (domain.Delivery, error) {
	var result domain.Delivery
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if !scope.IsCompany() {
			if _, err := projectAccess(tx, actor, scope.ProjectID, false); err != nil {
				return err
			}
		}
		var err error
		result, err = deliveryDetail(tx, scope, id)
		return err
	}, &inventoryReadSnapshot)
	return result, dbError(err)
}

func (r *InventoryRepository) ListDeliveries(ctx context.Context, actor domain.Actor, scope domain.DeliveryScope, f domain.DeliveryFilter) (domain.PageResult[domain.Delivery], error) {
	cursorScope := "company-deliveries"
	if !scope.IsCompany() {
		cursorScope = "project-deliveries:" + scope.ProjectID
	}
	result := domain.PageResult[domain.Delivery]{Items: []domain.Delivery{}}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if !scope.IsCompany() {
			if _, err := projectAccess(tx, actor, scope.ProjectID, false); err != nil {
				return err
			}
		}
		q := deliveryQuery(tx, scope)
		if f.ProjectID != "" {
			q = q.Where("d.project_id = ?", f.ProjectID)
		}
		if f.Direction != "" {
			q = q.Where("d.direction = ?", f.Direction)
		}
		if f.Status != "" {
			q = q.Where("d.status = ?", f.Status)
		}
		if f.CoarseSentOnOrAfter != nil {
			q = q.Where("d.sent_at >= ? AND (d.sent_at AT TIME ZONE ?::text)::date >= ?::date",
				*f.CoarseSentOnOrAfter, f.BusinessTimezone, f.StartDate)
		}
		if f.CoarseSentBefore != nil {
			q = q.Where("d.sent_at < ? AND (d.sent_at AT TIME ZONE ?::text)::date <= ?::date",
				*f.CoarseSentBefore, f.BusinessTimezone, f.EndDate)
		}
		cursor, err := domain.DecodeCursor(f.Page.Cursor, 3)
		if err != nil {
			return err
		}
		if cursor != nil {
			if cursor[0] != cursorScope {
				return domain.Invalid("cursor does not match this delivery list")
			}
			sentAt, err := inventoryTimeKey(cursor[1])
			if err != nil {
				return err
			}
			id, err := inventoryCursorID(cursor[2])
			if err != nil {
				return err
			}
			q = q.Where("(d.sent_at, d.id) < (?, ?)", sentAt, id)
		}
		var rows []deliveryRow
		if err := q.Order("d.sent_at DESC, d.id DESC").Limit(f.Page.Size() + 1).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) > f.Page.Size() {
			rows = rows[:f.Page.Size()]
			last := rows[len(rows)-1]
			result.NextCursor = domain.EncodeCursor(cursorScope, last.SentAt.UTC().Format(time.RFC3339Nano), strconv.FormatInt(last.ID, 10))
		}
		ids := make([]int64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		lines, err := deliveryLineRows(tx, ids)
		if err != nil {
			return err
		}
		byDelivery := make(map[int64][]domain.DeliveryLine, len(rows))
		for _, line := range lines {
			byDelivery[line.DeliveryID] = append(byDelivery[line.DeliveryID], line.DeliveryLine)
		}
		for _, row := range rows {
			result.Items = append(result.Items, deliveryResponse(row, byDelivery[row.ID], scope))
		}
		return nil
	}, &inventoryReadSnapshot)
	return result, dbError(err)
}

func (r *InventoryRepository) InitiateDelivery(ctx context.Context, actor domain.Actor, scope domain.DeliveryScope, destinationProject string, input []domain.DeliveryLineInput) (domain.Delivery, error) {
	var result domain.Delivery
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		projectID, direction := scope.ProjectID, domain.ProjectToCompany
		if scope.IsCompany() {
			projectID, direction = destinationProject, domain.CompanyToProject
			var existing string
			if err := tx.Raw("SELECT id FROM projects WHERE id = ? FOR UPDATE", projectID).Row().Scan(&existing); err != nil {
				return dbError(err)
			}
		} else if _, err := projectAccess(tx, actor, projectID, true); err != nil {
			return err
		}
		senderID, err := inventoryStoreID(tx, scope.ProjectID)
		if err != nil {
			return err
		}
		receiverProject := projectID
		if !scope.IsCompany() {
			receiverProject = ""
		}
		if _, err := inventoryStoreID(tx, receiverProject); err != nil {
			return err
		}
		lines := slices.Clone(input)
		slices.SortFunc(lines, func(a, b domain.DeliveryLineInput) int { return cmp.Compare(a.MaterialID, b.MaterialID) })
		materials := make([]domain.Material, 0, len(lines))
		// Material locks precede stock locks and freeze the catalogue data for this send.
		for _, line := range lines {
			item, err := inventoryMaterial(tx, line.MaterialID, "SHARE")
			if err != nil {
				return err
			}
			materials = append(materials, item)
		}
		var id int64
		var sentAt time.Time
		if err := tx.Raw(`INSERT INTO deliveries (project_id,direction,sent_by,sent_at) VALUES (?,?,?,clock_timestamp())
   RETURNING id,sent_at`, projectID, direction, actor.UserID).Row().Scan(&id, &sentAt); err != nil {
			return err
		}
		for i, line := range lines {
			quantity := int64(line.Quantity)
			update := tx.Exec(`UPDATE stock SET quantity = quantity - ?, outbound_quantity = outbound_quantity + ?,
     last_delivery_at = greatest(last_delivery_at, ?) WHERE store_id = ? AND material_id = ? AND quantity >= ?`,
				quantity, quantity, sentAt, senderID, line.MaterialID, quantity)
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return domain.Conflict("insufficient stock for delivery")
			}
			item := materials[i]
			if err := tx.Exec(`INSERT INTO delivery_lines
    (delivery_id,material_id,quantity,material_name,material_category,material_unit) VALUES (?,?,?,?,?,?)`,
				id, line.MaterialID, quantity, item.Name, item.Category, item.Unit).Error; err != nil {
				return err
			}
		}
		result, err = deliveryDetail(tx, scope, id)
		return err
	})
	return result, inventoryWriteError(err)
}

func (r *InventoryRepository) ResolveDelivery(ctx context.Context, actor domain.Actor, scope domain.DeliveryScope, id int64, action domain.DeliveryAction) (domain.Delivery, error) {
	var result domain.Delivery
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Table("deliveries").Select("project_id, direction, status").Where("id = ?", id)
		if !scope.IsCompany() {
			if _, err := projectAccess(tx, actor, scope.ProjectID, true); err != nil {
				return err
			}
			q = q.Where("project_id = ?", scope.ProjectID)
		}
		var projectID string
		var direction domain.DeliveryDirection
		var status domain.DeliveryStatus
		if err := q.Clauses(clause.Locking{Strength: "UPDATE"}).Row().Scan(&projectID, &direction, &status); err != nil {
			return dbError(err)
		}
		if !scope.CanResolve(direction, action) {
			return domain.ErrForbidden
		}
		if status != domain.DeliveryInTransit {
			return domain.Conflict("delivery is no longer in transit")
		}
		lines, err := deliveryLineRows(tx, []int64{id})
		if err != nil {
			return err
		}
		for _, line := range lines {
			if _, err := inventoryMaterial(tx, line.MaterialID, "SHARE"); err != nil {
				return err
			}
		}
		senderProject, receiverProject := "", projectID
		if direction == domain.ProjectToCompany {
			senderProject, receiverProject = receiverProject, senderProject
		}
		senderID, err := inventoryStoreID(tx, senderProject)
		if err != nil {
			return err
		}
		var resolvedAt time.Time
		if err := tx.Raw(`UPDATE deliveries SET status = ?, resolved_by = ?, resolved_at = greatest(sent_at, clock_timestamp())
   WHERE id = ? RETURNING resolved_at`, action.Result(), actor.UserID, id).Row().Scan(&resolvedAt); err != nil {
			return err
		}
		if action != domain.RecordDelivery {
			if err := releaseStockReservation(tx, senderID, lines, true); err != nil {
				return err
			}
		} else {
			receiverID, err := inventoryStoreID(tx, receiverProject)
			if err != nil {
				return err
			}
			// Every mutation acquires stock in (store ID, material ID) order, including
			// incoming movements that touch both stores.
			if senderID < receiverID {
				if err := releaseStockReservation(tx, senderID, lines, false); err != nil {
					return err
				}
				if err := receiveDeliveryStock(tx, receiverID, lines, resolvedAt); err != nil {
					return err
				}
			} else {
				if err := receiveDeliveryStock(tx, receiverID, lines, resolvedAt); err != nil {
					return err
				}
				if err := releaseStockReservation(tx, senderID, lines, false); err != nil {
					return err
				}
			}
		}
		result, err = deliveryDetail(tx, scope, id)
		return err
	})
	return result, inventoryWriteError(err)
}

func releaseStockReservation(tx *gorm.DB, storeID int64, lines []deliveryLineRow, refund bool) error {
	for _, line := range lines {
		quantity, refundQuantity := int64(line.Quantity), int64(0)
		if refund {
			refundQuantity = quantity
		}
		update := tx.Exec(`UPDATE stock SET outbound_quantity = outbound_quantity - ?, quantity = quantity + ?
   WHERE store_id = ? AND material_id = ? AND outbound_quantity >= ?`, quantity, refundQuantity, storeID, line.MaterialID, quantity)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return domain.Conflict("delivery stock reservation is unavailable")
		}
	}
	return nil
}

func receiveDeliveryStock(tx *gorm.DB, storeID int64, lines []deliveryLineRow, at time.Time) error {
	for _, line := range lines {
		if err := tx.Exec(`INSERT INTO stock (store_id, material_id, quantity, last_delivery_at) VALUES (?,?,?,?)
   ON CONFLICT (store_id,material_id) DO UPDATE SET quantity = stock.quantity + EXCLUDED.quantity,
    last_delivery_at = greatest(stock.last_delivery_at, EXCLUDED.last_delivery_at)`,
			storeID, line.MaterialID, int64(line.Quantity), at).Error; err != nil {
			return err
		}
	}
	return nil
}
