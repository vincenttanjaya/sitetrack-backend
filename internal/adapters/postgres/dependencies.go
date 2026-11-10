package postgres

import (
	"context"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"gorm.io/gorm"
)

func (r *TaskRepository) TaskDependency(ctx context.Context, actor domain.Actor, projectID, taskID, prerequisiteID string) (result domain.TaskReference, err error) {
	err = r.projectTaskRead(ctx, actor, projectID, func(tx *gorm.DB) error {
		result, err = readTaskDependency(tx, projectID, taskID, prerequisiteID)
		return err
	})
	return result, err
}

func readTaskDependency(db *gorm.DB, projectID, taskID, prerequisiteID string) (result domain.TaskReference, err error) {
	err = db.Raw(`SELECT p.id, p.name, p.status
FROM task_dependencies d
JOIN tasks t ON t.project_id = d.project_id AND t.number = d.task_number
JOIN tasks p ON p.project_id = d.project_id AND p.number = d.prerequisite_number
WHERE d.project_id = ? AND t.id = ? AND p.id = ?`, projectID, taskID, prerequisiteID).
		Row().Scan(&result.ID, &result.Name, &result.Status)
	return result, dbError(err)
}

func (r *TaskRepository) AddTaskDependency(ctx context.Context, actor domain.Actor, projectID, taskID, prerequisiteID string) (result domain.TaskReference, err error) {
	err = r.projectTaskWrite(ctx, actor, projectID, func(tx *gorm.DB) error {
		task, prerequisite, err := loadDependencyTasks(tx, projectID, taskID, prerequisiteID)
		if err != nil {
			return err
		}
		if err := validateDependency(tx, projectID, task, prerequisite); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO task_dependencies (project_id, task_number, prerequisite_number)
VALUES (?, ?, ?)`, projectID, task.Number, prerequisite.Number).Error; err != nil {
			return dbError(err)
		}
		result = prerequisite.reference()
		return nil
	})
	return result, err
}

func (r *TaskRepository) ReplaceTaskDependency(ctx context.Context, actor domain.Actor, projectID, taskID, oldID, newID string) (result domain.TaskReference, err error) {
	err = r.projectTaskWrite(ctx, actor, projectID, func(tx *gorm.DB) error {
		if _, err := readTaskDependency(tx, projectID, taskID, oldID); err != nil {
			return err
		}
		task, prerequisite, err := loadDependencyTasks(tx, projectID, taskID, newID)
		if err != nil {
			return err
		}
		if err := validateDependency(tx, projectID, task, prerequisite); err != nil {
			return err
		}
		updated := tx.Exec(`UPDATE task_dependencies d SET prerequisite_number = ?
FROM tasks p WHERE d.project_id = ? AND d.task_number = ? AND p.project_id = d.project_id
  AND p.number = d.prerequisite_number AND p.id = ?`, prerequisite.Number, projectID, task.Number, oldID)
		if updated.Error != nil {
			return dbError(updated.Error)
		}
		if updated.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		result = prerequisite.reference()
		return nil
	})
	return result, err
}

func (r *TaskRepository) DeleteTaskDependency(ctx context.Context, actor domain.Actor, projectID, taskID, prerequisiteID string) error {
	return r.projectTaskWrite(ctx, actor, projectID, func(tx *gorm.DB) error {
		deleted := tx.Exec(`DELETE FROM task_dependencies d USING tasks t, tasks p
WHERE d.project_id = ? AND t.project_id = d.project_id AND p.project_id = d.project_id
  AND t.number = d.task_number AND p.number = d.prerequisite_number AND t.id = ? AND p.id = ?`,
			projectID, taskID, prerequisiteID)
		if deleted.Error != nil {
			return dbError(deleted.Error)
		}
		if deleted.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func loadDependencyTasks(db *gorm.DB, projectID, taskID, prerequisiteID string) (numberedTaskReference, numberedTaskReference, error) {
	var task, prerequisite numberedTaskReference
	err := db.Raw(`SELECT t.number, t.status, p.number, p.id, p.name, p.status
FROM tasks t JOIN tasks p ON p.project_id = t.project_id
WHERE t.project_id = ? AND t.id = ? AND p.id = ?`, projectID, taskID, prerequisiteID).
		Row().Scan(&task.Number, &task.Status, &prerequisite.Number, &prerequisite.ID, &prerequisite.Name, &prerequisite.Status)
	return task, prerequisite, dbError(err)
}

func validateDependency(db *gorm.DB, projectID string, task, prerequisite numberedTaskReference) error {
	if task.Number == prerequisite.Number {
		return domain.Conflict("task cannot depend on itself")
	}
	if task.Status != domain.TaskNotStarted && prerequisite.Status != domain.TaskDone {
		return domain.Conflict("a started task can only depend on a done task")
	}
	// UNION deduplicates reachable tasks. Replacing an outgoing edge from
	// task cannot affect whether prerequisite can reach task in the old graph.
	// Keep each frontier lookup parameterized so PostgreSQL can use the edge
	// index instead of re-scanning all project edges at each depth.
	var cyclic bool
	err := db.Raw(`WITH RECURSIVE reachable(number) AS (
    SELECT CAST(? AS bigint)
    UNION
    SELECT d.prerequisite_number FROM reachable r
    CROSS JOIN LATERAL (
        SELECT prerequisite_number FROM task_dependencies
        WHERE project_id = ? AND task_number = r.number
        OFFSET 0
    ) d
)
SELECT EXISTS (SELECT 1 FROM reachable WHERE number = ?)`, prerequisite.Number, projectID, task.Number).Row().Scan(&cyclic)
	if err != nil {
		return dbError(err)
	}
	if cyclic {
		return domain.Conflict("dependency would create a cycle")
	}
	return nil
}
