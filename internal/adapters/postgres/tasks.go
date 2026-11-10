package postgres

import (
	"context"
	"database/sql"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/ports"
	"gorm.io/gorm"
)

type TaskRepository struct {
	db *gorm.DB
}

var _ ports.Tasks = (*TaskRepository)(nil)

func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) projectTaskWrite(ctx context.Context, actor domain.Actor, projectID string, fn func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := projectAccess(tx, actor, projectID, true); err != nil {
			return err
		}
		return fn(tx)
	})
}

func (r *TaskRepository) projectTaskRead(ctx context.Context, actor domain.Actor, projectID string, fn func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := projectAccess(tx, actor, projectID, false); err != nil {
			return err
		}
		return fn(tx)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

func (r *TaskRepository) CreateTask(ctx context.Context, actor domain.Actor, projectID string, input domain.TaskInput, today string) (result domain.TaskDetail, err error) {
	err = r.projectTaskWrite(ctx, actor, projectID, func(tx *gorm.DB) error {
		task := domain.TaskDetail{
			Task: domain.Task{
				TaskReference:      domain.TaskReference{Name: input.Name},
				StartDate:          input.StartDate,
				EndDate:            input.EndDate,
				DirectDependencies: []domain.TaskReference{},
			},
			Prerequisites: []domain.TaskDistance{},
			Dependents:    []domain.TaskDistance{},
		}
		if err := tx.Raw(`WITH allocated AS (
    UPDATE projects SET next_task_number = next_task_number + 1 WHERE id = ?
    RETURNING id, next_task_number - 1 AS number
)
INSERT INTO tasks (project_id, number, name, start_date, end_date)
SELECT id, number, ?, CAST(? AS date), CAST(? AS date) FROM allocated
RETURNING id`, projectID, input.Name, input.StartDate, input.EndDate).Row().Scan(&task.ID); err != nil {
			return dbError(err)
		}
		result = task.WithStatus(domain.TaskNotStarted, today)
		return nil
	})
	return result, err
}

func (r *TaskRepository) UpdateTask(ctx context.Context, actor domain.Actor, projectID, taskID string, input domain.TaskInput, today string) (result domain.TaskDetail, err error) {
	err = r.projectTaskWrite(ctx, actor, projectID, func(tx *gorm.DB) error {
		updated := tx.Exec(`UPDATE tasks SET name = ?, start_date = CAST(? AS date), end_date = CAST(? AS date)
WHERE project_id = ? AND id = ?`, input.Name, input.StartDate, input.EndDate, projectID, taskID)
		if updated.Error != nil {
			return dbError(updated.Error)
		}
		if updated.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		result, err = readTaskDetail(tx, projectID, taskID, today)
		return err
	})
	return result, err
}

func (r *TaskRepository) DeleteTask(ctx context.Context, actor domain.Actor, projectID, taskID string) error {
	return r.projectTaskWrite(ctx, actor, projectID, func(tx *gorm.DB) error {
		var number int64
		var referenced bool
		err := tx.Raw(`SELECT t.number, EXISTS (
    SELECT 1 FROM task_dependencies d WHERE d.project_id = t.project_id AND d.prerequisite_number = t.number
) FROM tasks t WHERE t.project_id = ? AND t.id = ?`, projectID, taskID).Row().Scan(&number, &referenced)
		if err != nil {
			return dbError(err)
		}
		if referenced {
			return domain.Conflict("remove dependent links before deleting this task")
		}
		return dbError(tx.Exec("DELETE FROM tasks WHERE project_id = ? AND number = ?", projectID, number).Error)
	})
}

func (r *TaskRepository) TaskDetail(ctx context.Context, actor domain.Actor, projectID, taskID, today string) (result domain.TaskDetail, err error) {
	err = r.projectTaskRead(ctx, actor, projectID, func(tx *gorm.DB) error {
		result, err = readTaskDetail(tx, projectID, taskID, today)
		return err
	})
	return result, err
}

func (r *TaskRepository) ActOnTask(ctx context.Context, actor domain.Actor, projectID, taskID string, action domain.TaskAction, today string) (result domain.TaskDetail, err error) {
	err = r.projectTaskWrite(ctx, actor, projectID, func(tx *gorm.DB) error {
		task, err := readTaskDetail(tx, projectID, taskID, today)
		if err != nil {
			return err
		}
		result, err = task.ApplyAction(action, today)
		if err != nil {
			return err
		}
		return dbError(tx.Exec("UPDATE tasks SET status = ? WHERE project_id = ? AND id = ?", result.Status, projectID, taskID).Error)
	})
	return result, err
}
