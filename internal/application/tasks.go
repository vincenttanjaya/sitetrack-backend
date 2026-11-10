package application

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/ports"
)

type TaskService struct {
	repo     ports.Tasks
	now      func() time.Time
	location *time.Location
}

func NewTasks(repo ports.Tasks, now func() time.Time, location *time.Location) *TaskService {
	return &TaskService{repo: repo, now: now, location: location}
}

func (s *TaskService) today() string {
	return s.now().In(s.location).Format(time.DateOnly)
}

func (s *TaskService) List(ctx context.Context, actor domain.Actor, projectID string, q domain.TaskListQuery) (domain.PageResult[domain.Task], error) {
	var empty domain.PageResult[domain.Task]
	if !validTaskResourceIDs(projectID) {
		return empty, domain.ErrNotFound
	}
	if !utf8.ValidString(q.Search) || utf8.RuneCountInString(q.Search) > 200 || strings.ContainsRune(q.Search, 0) {
		return empty, domain.Invalid("q must contain at most 200 characters")
	}
	if q.Status != "" && !q.Status.Valid() {
		return empty, domain.Invalid("unknown task status")
	}
	for _, date := range []string{q.From, q.To} {
		if date != "" {
			if _, err := domain.ParseDate(date); err != nil {
				return empty, err
			}
		}
	}
	if q.From != "" && q.To != "" && q.To < q.From {
		return empty, domain.Invalid("to precedes from")
	}
	if q.Sort == "" {
		q.Sort = "id"
	}
	if q.Order == "" {
		q.Order = "asc"
	}
	if q.Order != "asc" && q.Order != "desc" {
		return empty, domain.Invalid("order must be asc or desc")
	}
	return s.repo.ListTasks(ctx, actor, projectID, q, s.today())
}

func (s *TaskService) Detail(ctx context.Context, actor domain.Actor, projectID, taskID string) (domain.TaskDetail, error) {
	if !validTaskResourceIDs(projectID, taskID) {
		return domain.TaskDetail{}, domain.ErrNotFound
	}
	return s.repo.TaskDetail(ctx, actor, projectID, taskID, s.today())
}

func (s *TaskService) Create(ctx context.Context, actor domain.Actor, projectID string, input domain.TaskInput) (domain.TaskDetail, error) {
	if !validTaskResourceIDs(projectID) {
		return domain.TaskDetail{}, domain.ErrNotFound
	}
	input, err := validateTaskInput(input)
	if err != nil {
		return domain.TaskDetail{}, err
	}
	return s.repo.CreateTask(ctx, actor, projectID, input, s.today())
}

func (s *TaskService) Update(ctx context.Context, actor domain.Actor, projectID, taskID string, input domain.TaskInput) (domain.TaskDetail, error) {
	if !validTaskResourceIDs(projectID, taskID) {
		return domain.TaskDetail{}, domain.ErrNotFound
	}
	input, err := validateTaskInput(input)
	if err != nil {
		return domain.TaskDetail{}, err
	}
	return s.repo.UpdateTask(ctx, actor, projectID, taskID, input, s.today())
}

func (s *TaskService) Delete(ctx context.Context, actor domain.Actor, projectID, taskID string) error {
	if !validTaskResourceIDs(projectID, taskID) {
		return domain.ErrNotFound
	}
	return s.repo.DeleteTask(ctx, actor, projectID, taskID)
}

func (s *TaskService) Act(ctx context.Context, actor domain.Actor, projectID, taskID string, action domain.TaskAction) (domain.TaskDetail, error) {
	if !validTaskResourceIDs(projectID, taskID) {
		return domain.TaskDetail{}, domain.ErrNotFound
	}
	if action != domain.TaskStart && action != domain.TaskFinish && action != domain.TaskReopen {
		return domain.TaskDetail{}, domain.Invalid("unknown task action")
	}
	return s.repo.ActOnTask(ctx, actor, projectID, taskID, action, s.today())
}

func (s *TaskService) Dependency(ctx context.Context, actor domain.Actor, projectID, taskID, prerequisiteID string) (domain.TaskReference, error) {
	if !validTaskResourceIDs(projectID, taskID, prerequisiteID) {
		return domain.TaskReference{}, domain.ErrNotFound
	}
	return s.repo.TaskDependency(ctx, actor, projectID, taskID, prerequisiteID)
}

func (s *TaskService) AddDependency(ctx context.Context, actor domain.Actor, projectID, taskID, prerequisiteID string) (domain.TaskReference, error) {
	if !validTaskResourceIDs(projectID, taskID) {
		return domain.TaskReference{}, domain.ErrNotFound
	}
	if !validTaskResourceIDs(prerequisiteID) {
		return domain.TaskReference{}, domain.Invalid("invalid prerequisite_id")
	}
	return s.repo.AddTaskDependency(ctx, actor, projectID, taskID, prerequisiteID)
}

func (s *TaskService) ReplaceDependency(ctx context.Context, actor domain.Actor, projectID, taskID, oldID, newID string) (domain.TaskReference, error) {
	if !validTaskResourceIDs(projectID, taskID, oldID) {
		return domain.TaskReference{}, domain.ErrNotFound
	}
	if !validTaskResourceIDs(newID) {
		return domain.TaskReference{}, domain.Invalid("invalid prerequisite_id")
	}
	return s.repo.ReplaceTaskDependency(ctx, actor, projectID, taskID, oldID, newID)
}

func (s *TaskService) DeleteDependency(ctx context.Context, actor domain.Actor, projectID, taskID, prerequisiteID string) error {
	if !validTaskResourceIDs(projectID, taskID, prerequisiteID) {
		return domain.ErrNotFound
	}
	return s.repo.DeleteTaskDependency(ctx, actor, projectID, taskID, prerequisiteID)
}

func validateTaskInput(input domain.TaskInput) (domain.TaskInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > 200 || strings.ContainsRune(input.Name, 0) {
		return input, domain.Invalid("name must be between 1 and 200 characters")
	}
	return input, domain.ValidateDates(input.StartDate, input.EndDate)
}

func validTaskResourceIDs(ids ...string) bool {
	for _, id := range ids {
		if id == "" || len(id) > 64 || !utf8.ValidString(id) || strings.ContainsRune(id, 0) {
			return false
		}
	}
	return true
}
