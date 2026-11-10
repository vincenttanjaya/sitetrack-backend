package postgres

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"gorm.io/gorm"
)

func (r *ProjectRepository) Members(ctx context.Context, a domain.Actor, id string, p domain.Page) (domain.PageResult[domain.Member], error) {
	result := domain.PageResult[domain.Member]{Items: []domain.Member{}}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := projectAccess(tx, a, id, false); err != nil {
			return err
		}
		after, err := memberCursor(p.Cursor)
		if err != nil {
			return err
		}
		err = tx.Raw(`SELECT u.id AS user_id,u.name,u.email,m.is_admin,m.joined_at
   FROM project_members m JOIN users u ON u.id=m.user_id
   WHERE m.project_id=? AND m.user_id>? ORDER BY m.user_id LIMIT ?`, id, after, p.Size()+1).Scan(&result.Items).Error
		if err != nil {
			return dbError(err)
		}
		if len(result.Items) > p.Size() {
			result.Items = result.Items[:p.Size()]
			result.NextCursor = domain.EncodeCursor("members", strconv.FormatInt(result.Items[len(result.Items)-1].UserID, 10))
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, err
}

func memberCursor(token string) (int64, error) {
	if token == "" {
		return 0, nil
	}
	parts, err := domain.DecodeCursor(token, 2)
	if err != nil || parts[0] != "members" {
		return 0, domain.Invalid("invalid member cursor")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.Invalid("invalid member cursor")
	}
	return id, nil
}

func (r *ProjectRepository) SetMemberAdmin(ctx context.Context, a domain.Actor, id string, userID int64, admin bool) error {
	return r.mutateMember(ctx, a, id, userID, admin, func(tx *gorm.DB) error {
		return tx.Table("project_members").Where("project_id = ? AND user_id = ?", id, userID).Update("is_admin", admin).Error
	})
}

func (r *ProjectRepository) RemoveMember(ctx context.Context, a domain.Actor, id string, userID int64) error {
	return r.mutateMember(ctx, a, id, userID, false, func(tx *gorm.DB) error {
		return tx.Exec("DELETE FROM project_members WHERE project_id = ? AND user_id = ?", id, userID).Error
	})
}

func (r *ProjectRepository) mutateMember(ctx context.Context, a domain.Actor, id string, userID int64, retainsAdmin bool, mutate func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := projectAdmin(tx, a, id); err != nil {
			return err
		}
		var isAdmin bool
		if err := tx.Raw("SELECT is_admin FROM project_members WHERE project_id=? AND user_id=?", id, userID).Row().Scan(&isAdmin); err != nil {
			return dbError(err)
		}
		if isAdmin && !retainsAdmin {
			var otherAdmin bool
			err := tx.Raw("SELECT EXISTS (SELECT 1 FROM project_members WHERE project_id=? AND user_id<>? AND is_admin)", id, userID).Row().Scan(&otherAdmin)
			if err != nil {
				return dbError(err)
			}
			if !otherAdmin {
				return domain.Conflict("project must retain an admin")
			}
		}
		return dbError(mutate(tx))
	})
}
