package ports

import (
	"context"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

type Tasks interface {
	ListTasks(context.Context, domain.Actor, string, domain.TaskListQuery, string) (domain.PageResult[domain.Task], error)
	TaskDetail(ctx context.Context, actor domain.Actor, projectID, taskID, today string) (domain.TaskDetail, error)
	CreateTask(context.Context, domain.Actor, string, domain.TaskInput, string) (domain.TaskDetail, error)
	UpdateTask(context.Context, domain.Actor, string, string, domain.TaskInput, string) (domain.TaskDetail, error)
	DeleteTask(ctx context.Context, actor domain.Actor, projectID, taskID string) error
	ActOnTask(context.Context, domain.Actor, string, string, domain.TaskAction, string) (domain.TaskDetail, error)
	TaskDependency(ctx context.Context, actor domain.Actor, projectID, taskID, prerequisiteID string) (domain.TaskReference, error)
	AddTaskDependency(ctx context.Context, actor domain.Actor, projectID, taskID, prerequisiteID string) (domain.TaskReference, error)
	ReplaceTaskDependency(ctx context.Context, actor domain.Actor, projectID, taskID, oldID, newID string) (domain.TaskReference, error)
	DeleteTaskDependency(ctx context.Context, actor domain.Actor, projectID, taskID, prerequisiteID string) error
}
