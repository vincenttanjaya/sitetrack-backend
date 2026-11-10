package application

import (
	"context"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/ports"
)

type ProjectService struct {
	repo     ports.ProjectRepository
	clock    func() time.Time
	location *time.Location
}

func NewProjects(repo ports.ProjectRepository, clock func() time.Time, location *time.Location) *ProjectService {
	return &ProjectService{repo: repo, clock: clock, location: location}
}

func (s *ProjectService) today() time.Time {
	y, m, d := s.clock().In(s.location).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func cleanProject(in domain.ProjectInput) (domain.ProjectInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 200 || strings.ContainsRune(in.Name, 0) {
		return in, domain.Invalid("name must be between 1 and 200 characters")
	}
	return in, domain.ValidateDates(in.StartDate, in.EndDate)
}

func (s *ProjectService) Create(ctx context.Context, a domain.Actor, in domain.ProjectInput) (domain.Project, error) {
	in, err := cleanProject(in)
	if err != nil {
		return domain.Project{}, err
	}
	return s.repo.Create(ctx, a, in)
}

func (s *ProjectService) List(ctx context.Context, a domain.Actor, f domain.ProjectFilter) (domain.PageResult[domain.Project], error) {
	f.Search = strings.TrimSpace(f.Search)
	if utf8.RuneCountInString(f.Search) > 200 || strings.ContainsRune(f.Search, 0) {
		return domain.PageResult[domain.Project]{}, domain.Invalid("search is too long or invalid")
	}
	if f.Sort == "" {
		f.Sort = "name"
	}
	if f.Order == "" {
		f.Order = "asc"
	}
	switch f.Sort {
	case "name", "start_date", "percent_done", "late_tasks":
	default:
		return domain.PageResult[domain.Project]{}, domain.Invalid("invalid project sort")
	}
	if f.Order != "asc" && f.Order != "desc" {
		return domain.PageResult[domain.Project]{}, domain.Invalid("order must be asc or desc")
	}
	return s.repo.List(ctx, a, f, s.today())
}

func (s *ProjectService) Get(ctx context.Context, a domain.Actor, id string) (domain.Project, error) {
	return s.repo.Get(ctx, a, id, s.today())
}

func (s *ProjectService) Update(ctx context.Context, a domain.Actor, id string, in domain.ProjectInput) (domain.Project, error) {
	in, err := cleanProject(in)
	if err != nil {
		return domain.Project{}, err
	}
	return s.repo.Update(ctx, a, id, in, s.today())
}

func (s *ProjectService) Delete(ctx context.Context, a domain.Actor, id string) error {
	return s.repo.Delete(ctx, a, id)
}

func (s *ProjectService) Members(ctx context.Context, a domain.Actor, id string, p domain.Page) (domain.PageResult[domain.Member], error) {
	return s.repo.Members(ctx, a, id, p)
}

func (s *ProjectService) SetMemberAdmin(ctx context.Context, a domain.Actor, id string, userID int64, admin bool) error {
	return s.repo.SetMemberAdmin(ctx, a, id, userID, admin)
}

func (s *ProjectService) RemoveMember(ctx context.Context, a domain.Actor, id string, userID int64) error {
	return s.repo.RemoveMember(ctx, a, id, userID)
}

func (s *ProjectService) Invite(ctx context.Context, a domain.Actor, id, email string) (domain.Invitation, error) {
	email = normalizedEmail(email)
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 {
		return domain.Invitation{}, domain.Invalid("email is invalid")
	}
	return s.repo.Invite(ctx, a, id, email)
}

func (s *ProjectService) Invitations(ctx context.Context, a domain.Actor, id string, p domain.Page) (domain.PageResult[domain.Invitation], error) {
	return s.repo.Invitations(ctx, a, id, p)
}

func (s *ProjectService) Withdraw(ctx context.Context, a domain.Actor, id string, invitationID int64) error {
	return s.repo.Withdraw(ctx, a, id, invitationID)
}

func (s *ProjectService) Respond(ctx context.Context, a domain.Actor, id int64, accept bool) (domain.Invitation, error) {
	return s.repo.Respond(ctx, a, id, accept)
}

func (s *ProjectService) Notifications(ctx context.Context, a domain.Actor, p domain.Page) (domain.Notifications, error) {
	return s.repo.Notifications(ctx, a, p)
}

func (s *ProjectService) UnreadCount(ctx context.Context, a domain.Actor) (int64, error) {
	return s.repo.UnreadCount(ctx, a)
}

func (s *ProjectService) ReadNotification(ctx context.Context, a domain.Actor, id int64) error {
	return s.repo.ReadNotification(ctx, a, id)
}
