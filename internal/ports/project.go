package ports

import (
	"context"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

type ProjectRepository interface {
	Create(context.Context, domain.Actor, domain.ProjectInput) (domain.Project, error)
	List(context.Context, domain.Actor, domain.ProjectFilter, time.Time) (domain.PageResult[domain.Project], error)
	Get(context.Context, domain.Actor, string, time.Time) (domain.Project, error)
	Update(context.Context, domain.Actor, string, domain.ProjectInput, time.Time) (domain.Project, error)
	Delete(context.Context, domain.Actor, string) error
	Members(context.Context, domain.Actor, string, domain.Page) (domain.PageResult[domain.Member], error)
	SetMemberAdmin(context.Context, domain.Actor, string, int64, bool) error
	RemoveMember(context.Context, domain.Actor, string, int64) error
	Invite(context.Context, domain.Actor, string, string) (domain.Invitation, error)
	Invitations(context.Context, domain.Actor, string, domain.Page) (domain.PageResult[domain.Invitation], error)
	Withdraw(context.Context, domain.Actor, string, int64) error
	Respond(context.Context, domain.Actor, int64, bool) (domain.Invitation, error)
	Notifications(context.Context, domain.Actor, domain.Page) (domain.Notifications, error)
	UnreadCount(context.Context, domain.Actor) (int64, error)
	ReadNotification(context.Context, domain.Actor, int64) error
}
