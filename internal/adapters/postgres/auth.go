package postgres

import (
	"context"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"gorm.io/gorm"
)

type AuthRepository struct{ db *gorm.DB }

func NewAuthRepository(db *gorm.DB) *AuthRepository { return &AuthRepository{db: db} }

type userRow struct {
	domain.User  `gorm:"embedded"`
	PasswordHash string
}

func (userRow) TableName() string { return "users" }

func (r *AuthRepository) CreateUser(ctx context.Context, input domain.RegisterInput, hash string) (domain.User, error) {
	row := userRow{User: domain.User{Name: input.Name, Email: input.Email, Role: input.Role}, PasswordHash: hash}
	err := r.db.WithContext(ctx).Create(&row).Error
	return row.User, dbError(err)
}

func (r *AuthRepository) Credentials(ctx context.Context, email string) (domain.User, string, error) {
	var row userRow
	err := r.db.WithContext(ctx).Where("email = ?", email).Take(&row).Error
	return row.User, row.PasswordHash, dbError(err)
}

func (r *AuthRepository) User(ctx context.Context, id int64) (domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Table("users").Select("id, name, email, role, created_at").Where("id = ?", id).Take(&user).Error
	return user, dbError(err)
}

func (r *AuthRepository) CreateSession(ctx context.Context, s domain.Session) error {
	return dbError(r.db.WithContext(ctx).Table("sessions").Create(map[string]any{
		"id": s.ID, "user_id": s.UserID, "refresh_hash": s.RefreshHash[:], "expires_at": s.ExpiresAt,
	}).Error)
}

func (r *AuthRepository) CurrentActor(ctx context.Context, id string, userID int64, now time.Time) (domain.Actor, error) {
	var a domain.Actor
	err := r.db.WithContext(ctx).Raw(`SELECT u.id, u.role, s.id
  FROM sessions s JOIN users u ON u.id = s.user_id
  WHERE s.id = ? AND s.user_id = ? AND s.revoked_at IS NULL AND s.expires_at > ?`, id, userID, now).
		Row().Scan(&a.UserID, &a.Role, &a.SessionID)
	return a, dbError(err)
}

func (r *AuthRepository) RotateSession(ctx context.Context, previous, replacement [32]byte, now, expires time.Time) (domain.Session, error) {
	s := domain.Session{RefreshHash: replacement}
	err := r.db.WithContext(ctx).Raw(`UPDATE sessions SET refresh_hash = ?, expires_at = ?
  WHERE refresh_hash = ? AND revoked_at IS NULL AND expires_at > ?
  RETURNING id, user_id, expires_at`, replacement[:], expires, previous[:], now).
		Row().Scan(&s.ID, &s.UserID, &s.ExpiresAt)
	return s, dbError(err)
}

func (r *AuthRepository) RevokeSession(ctx context.Context, actor domain.Actor, now time.Time) error {
	return r.db.WithContext(ctx).Table("sessions").Where("id = ? AND user_id = ? AND revoked_at IS NULL", actor.SessionID, actor.UserID).
		Update("revoked_at", now).Error
}
