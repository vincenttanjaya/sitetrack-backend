package ports

import (
	"context"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

type AuthRepository interface {
	CreateUser(context.Context, domain.RegisterInput, string) (domain.User, error)
	Credentials(context.Context, string) (domain.User, string, error)
	User(context.Context, int64) (domain.User, error)
	CreateSession(context.Context, domain.Session) error
	CurrentActor(context.Context, string, int64, time.Time) (domain.Actor, error)
	RotateSession(context.Context, [32]byte, [32]byte, time.Time, time.Time) (domain.Session, error)
	RevokeSession(context.Context, domain.Actor, time.Time) error
}
