package postgres

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/ports"
	"gorm.io/gorm"
)

type InventoryRepository struct{ db *gorm.DB }

func NewInventoryRepository(db *gorm.DB) *InventoryRepository { return &InventoryRepository{db: db} }

var _ ports.InventoryRepository = (*InventoryRepository)(nil)

var inventoryReadSnapshot = sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}

func inventoryWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" && pgErr.ConstraintName == "stock_refund_headroom" {
		return domain.Conflict("quantity exceeds store capacity")
	}
	return dbError(err)
}

type inventorySortKey struct {
	column   string
	parse    func(string) (any, error)
	nullable bool
}

func stockNumberKey(raw string) (any, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return nil, domain.Invalid("invalid cursor")
	}
	return value, nil
}

func stockNameKey(raw string) (any, error) {
	if raw == "" || strings.ContainsRune(raw, 0) {
		return nil, domain.Invalid("invalid cursor")
	}
	return raw, nil
}

func inventoryTimeKey(raw string) (any, error) {
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || value.Year() < 1 {
		return nil, domain.Invalid("invalid cursor")
	}
	return value, nil
}

func inventoryCursorID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.Invalid("invalid cursor")
	}
	return id, nil
}

func inventoryOrder(q *gorm.DB, page domain.Page, scope, sortName, order string, key inventorySortKey) (*gorm.DB, error) {
	cursor, err := domain.DecodeCursor(page.Cursor, 5)
	if err != nil {
		return nil, err
	}
	comparison, sqlOrder := ">", "ASC"
	if order == "desc" {
		comparison, sqlOrder = "<", "DESC"
	}
	if cursor != nil {
		if cursor[0] != scope || cursor[1] != sortName || cursor[2] != order {
			return nil, domain.Invalid("cursor does not match this list or sort")
		}
		id, err := inventoryCursorID(cursor[4])
		if err != nil {
			return nil, err
		}
		if cursor[3] == "" && key.nullable {
			q = q.Where(key.column+" IS NULL AND id "+comparison+" ?", id)
		} else {
			value, err := key.parse(cursor[3])
			if err != nil {
				return nil, err
			}
			predicate := "(" + key.column + ", id) " + comparison + " (?, ?)"
			if key.nullable {
				predicate = "(" + predicate + " OR " + key.column + " IS NULL)"
			}
			q = q.Where(predicate, value, id)
		}
	}
	ordering := key.column + " " + sqlOrder
	if key.nullable {
		ordering += " NULLS LAST"
	}
	return q.Order(ordering + ", id " + sqlOrder).Limit(page.Size() + 1), nil
}

func inventoryPage[T, R any](rows []R, page domain.Page, scope, sortName, order string, convert func(R) T, cursorKey func(R) (string, int64)) domain.PageResult[T] {
	out := domain.PageResult[T]{Items: make([]T, 0, min(len(rows), page.Size()))}
	if len(rows) > page.Size() {
		rows = rows[:page.Size()]
		key, id := cursorKey(rows[len(rows)-1])
		out.NextCursor = domain.EncodeCursor(scope, sortName, order, key, strconv.FormatInt(id, 10))
	}
	for _, row := range rows {
		out.Items = append(out.Items, convert(row))
	}
	return out
}

func inventoryMaterialFilters(q *gorm.DB, search, category string) *gorm.DB {
	if search != "" {
		q = q.Where("name ILIKE ?", containsPattern(search))
	}
	if category != "" {
		q = q.Where("lower(category)=lower(?)", category)
	}
	return q
}
