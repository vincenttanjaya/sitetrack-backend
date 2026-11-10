package postgres

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const invitationSelect = `SELECT i.id,i.project_id,p.name AS project_name,i.recipient_id,
 recipient.name AS recipient_name,recipient.email AS recipient_email,i.sender_id,sender.name AS sender_name,
 i.status,i.created_at,i.read_at,i.responded_at
 FROM invitations i JOIN projects p ON p.id=i.project_id
 JOIN users recipient ON recipient.id=i.recipient_id JOIN users sender ON sender.id=i.sender_id`

func invitationByID(db *gorm.DB, id int64) (domain.Invitation, error) {
	var i domain.Invitation
	result := db.Raw(invitationSelect+" WHERE i.id=?", id).Scan(&i)
	if result.Error != nil {
		return i, dbError(result.Error)
	}
	if result.RowsAffected == 0 {
		return i, domain.ErrNotFound
	}
	return i, nil
}

func (r *ProjectRepository) Invite(ctx context.Context, a domain.Actor, id, email string) (domain.Invitation, error) {
	var invitation domain.Invitation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := projectAdmin(tx, a, id); err != nil {
			return err
		}
		var recipient int64
		if err := tx.Raw("SELECT id FROM users WHERE email=?", email).Row().Scan(&recipient); err != nil {
			return dbError(err)
		}
		var member bool
		if err := tx.Raw("SELECT EXISTS (SELECT 1 FROM project_members WHERE project_id=? AND user_id=?)", id, recipient).Row().Scan(&member); err != nil {
			return dbError(err)
		}
		if member {
			return domain.Conflict("recipient is already a member")
		}
		var inviteID int64
		err := tx.Raw("INSERT INTO invitations (project_id,recipient_id,sender_id) VALUES (?,?,?) RETURNING id", id, recipient, a.UserID).Row().Scan(&inviteID)
		if err != nil {
			return dbError(err)
		}
		invitation, err = invitationByID(tx, inviteID)
		return err
	})
	return invitation, err
}

func (r *ProjectRepository) Invitations(ctx context.Context, a domain.Actor, id string, p domain.Page) (domain.PageResult[domain.Invitation], error) {
	var result domain.PageResult[domain.Invitation]
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := projectAccess(tx, a, id, false); err != nil {
			return err
		}
		var err error
		result, err = listInvitations(tx, p, "invitations", "i.project_id=? AND i.status='pending'", id)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, err
}

func (r *ProjectRepository) Withdraw(ctx context.Context, a domain.Actor, id string, invitationID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := projectAdmin(tx, a, id); err != nil {
			return err
		}
		var status string
		if err := tx.Raw("SELECT status FROM invitations WHERE id=? AND project_id=?", invitationID, id).Row().Scan(&status); err != nil {
			return dbError(err)
		}
		if status != "pending" {
			return domain.Conflict("only a pending invitation can be withdrawn")
		}
		return dbError(tx.Exec("UPDATE invitations SET status='withdrawn', responded_at=clock_timestamp() WHERE id=?", invitationID).Error)
	})
}

func (r *ProjectRepository) Respond(ctx context.Context, a domain.Actor, id int64, accept bool) (domain.Invitation, error) {
	var invitation domain.Invitation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var projectID string
		if err := tx.Raw("SELECT project_id FROM invitations WHERE id=? AND recipient_id=? AND status<>'withdrawn'", id, a.UserID).Row().Scan(&projectID); err != nil {
			return dbError(err)
		}
		var locked string
		if err := tx.Raw("SELECT id FROM projects WHERE id=? FOR UPDATE", projectID).Row().Scan(&locked); err != nil {
			return dbError(err)
		}
		var status string
		if err := tx.Raw("SELECT status FROM invitations WHERE id=? AND recipient_id=? AND status<>'withdrawn'", id, a.UserID).Row().Scan(&status); err != nil {
			return dbError(err)
		}
		if status != "pending" {
			return domain.Conflict("invitation has already been answered")
		}
		status = "rejected"
		if accept {
			status = "accepted"
			err := tx.Table("project_members").Clauses(clause.OnConflict{DoNothing: true}).Create(map[string]any{
				"project_id": projectID, "user_id": a.UserID, "is_admin": false,
			}).Error
			if err != nil {
				return dbError(err)
			}
		}
		if err := tx.Exec(`UPDATE invitations SET status=?,responded_at=clock_timestamp(),
     read_at=COALESCE(read_at,clock_timestamp()) WHERE id=?`, status, id).Error; err != nil {
			return dbError(err)
		}
		var err error
		invitation, err = invitationByID(tx, id)
		return err
	})
	return invitation, err
}

func (r *ProjectRepository) Notifications(ctx context.Context, a domain.Actor, p domain.Page) (domain.Notifications, error) {
	var result domain.Notifications
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result.PageResult, err = listInvitations(tx, p, "notifications", "i.recipient_id=? AND i.status<>'withdrawn'", a.UserID)
		if err != nil {
			return err
		}
		result.UnreadCount, err = unreadInvitations(tx, a.UserID)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, err
}

func (r *ProjectRepository) UnreadCount(ctx context.Context, a domain.Actor) (int64, error) {
	return unreadInvitations(r.db.WithContext(ctx), a.UserID)
}

func unreadInvitations(db *gorm.DB, userID int64) (int64, error) {
	var count int64
	err := db.Raw("SELECT count(*) FROM invitations WHERE recipient_id=? AND read_at IS NULL AND status<>'withdrawn'", userID).Row().Scan(&count)
	return count, dbError(err)
}

func (r *ProjectRepository) ReadNotification(ctx context.Context, a domain.Actor, id int64) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE invitations SET read_at=COALESCE(read_at,clock_timestamp())
  WHERE id=? AND recipient_id=? AND status<>'withdrawn'`, id, a.UserID)
	if result.Error != nil {
		return dbError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func listInvitations(db *gorm.DB, p domain.Page, scope, where string, arg any) (domain.PageResult[domain.Invitation], error) {
	result := domain.PageResult[domain.Invitation]{Items: []domain.Invitation{}}
	query := invitationSelect + " WHERE " + where
	args := []any{arg}
	if p.Cursor != "" {
		parts, err := domain.DecodeCursor(p.Cursor, 3)
		invalid := domain.Invalid("invalid invitation cursor")
		if err != nil || parts[0] != scope {
			return result, invalid
		}
		sent, err := time.Parse(time.RFC3339Nano, parts[1])
		if err != nil {
			return result, invalid
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || id <= 0 {
			return result, invalid
		}
		query += " AND (i.created_at,i.id)<(?,?)"
		args = append(args, sent, id)
	}
	query += " ORDER BY i.created_at DESC,i.id DESC LIMIT ?"
	args = append(args, p.Size()+1)
	if err := db.Raw(query, args...).Scan(&result.Items).Error; err != nil {
		return result, dbError(err)
	}
	if len(result.Items) > p.Size() {
		result.Items = result.Items[:p.Size()]
		last := result.Items[len(result.Items)-1]
		result.NextCursor = domain.EncodeCursor(scope, last.CreatedAt.UTC().Format(time.RFC3339Nano), strconv.FormatInt(last.ID, 10))
	}
	return result, nil
}
