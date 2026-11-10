package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"gorm.io/gorm"
)

type taskListRow struct {
	domain.TaskReference
	Number         int64
	StartDate      string
	EndDate        string
	Late           bool
	Blocked        bool
	DependentCount int
	SortKey        string
}

type taskListSort struct {
	expression string
	value      string
	cast       string
}

var taskListSorts = map[string]taskListSort{
	"id":              {"t.number", "t.number::text", "bigint"},
	"name":            {"lower(t.name)", "lower(t.name)", "text"},
	"start_date":      {"t.start_date", "to_char(t.start_date, 'YYYY-MM-DD')", "date"},
	"end_date":        {"t.end_date", "to_char(t.end_date, 'YYYY-MM-DD')", "date"},
	"dependent_count": {"counts.dependent_count", "counts.dependent_count::text", "bigint"},
}

func (r *TaskRepository) ListTasks(ctx context.Context, actor domain.Actor, projectID string, q domain.TaskListQuery, today string) (result domain.PageResult[domain.Task], err error) {
	err = r.projectTaskRead(ctx, actor, projectID, func(tx *gorm.DB) error {
		result, err = queryTaskList(tx, projectID, q, today)
		return err
	})
	return result, err
}

func queryTaskList(db *gorm.DB, projectID string, q domain.TaskListQuery, today string) (domain.PageResult[domain.Task], error) {
	var result domain.PageResult[domain.Task]
	sortKey, ok := taskListSorts[q.Sort]
	if !ok {
		return result, domain.Invalid("unknown task sort")
	}
	order, comparison := "ASC", ">"
	if q.Order == "desc" {
		order, comparison = "DESC", "<"
	}

	conditions := []string{"t.project_id = ?"}
	params := []any{today, projectID}
	if q.Status != "" {
		conditions = append(conditions, "t.status = ?")
		params = append(params, q.Status)
	}
	if q.Late != nil {
		conditions = append(conditions, "flags.late = ?")
		params = append(params, *q.Late)
	}
	if q.Blocked != nil {
		conditions = append(conditions, "flags.blocked = ?")
		params = append(params, *q.Blocked)
	}
	if q.From != "" {
		conditions = append(conditions, "t.end_date >= CAST(? AS date)")
		params = append(params, q.From)
	}
	if q.To != "" {
		conditions = append(conditions, "t.start_date <= CAST(? AS date)")
		params = append(params, q.To)
	}
	if q.Search != "" {
		pattern := containsPattern(q.Search)
		conditions = append(conditions, "(t.name ILIKE ? OR t.id ILIKE ?)")
		params = append(params, pattern, pattern)
	}

	cursor, err := domain.DecodeCursor(q.Page.Cursor, 5)
	if err != nil {
		return result, err
	}
	if cursor != nil {
		number, err := parseTaskListCursor(cursor, projectID, q, sortKey)
		if err != nil {
			return result, err
		}
		conditions = append(conditions, "("+sortKey.expression+", t.number) "+comparison+" (CAST(? AS "+sortKey.cast+"), ?)")
		params = append(params, cursor[3], number)
	}
	limit := q.Page.Size()
	params = append(params, limit+1)

	// A started task can only depend on Done tasks, so checking direct
	// prerequisites detects every blocked task in a valid graph.
	query := `SELECT t.number, t.id, t.name, t.status,
       to_char(t.start_date, 'YYYY-MM-DD') AS start_date, to_char(t.end_date, 'YYYY-MM-DD') AS end_date,
       flags.late, flags.blocked, counts.dependent_count, ` + sortKey.value + ` AS sort_key
FROM tasks t
CROSS JOIN (SELECT CAST(? AS date) AS today) business
CROSS JOIN LATERAL (
    SELECT (t.status <> 'done' AND t.end_date < business.today)
           OR (t.status = 'not_started' AND t.start_date < business.today) AS late,
           t.status <> 'done' AND EXISTS (
               SELECT 1 FROM task_dependencies d JOIN tasks p ON p.project_id = d.project_id AND p.number = d.prerequisite_number
               WHERE d.project_id = t.project_id AND d.task_number = t.number AND p.status <> 'done'
           ) AS blocked
) flags
CROSS JOIN LATERAL (
    SELECT count(*) AS dependent_count FROM task_dependencies d
    WHERE d.project_id = t.project_id AND d.prerequisite_number = t.number
) counts
WHERE ` + strings.Join(conditions, " AND ") + `
ORDER BY ` + sortKey.expression + " " + order + ", t.number " + order + `
LIMIT ?`
	var rows []taskListRow
	if err := db.Raw(query, params...).Scan(&rows).Error; err != nil {
		return result, dbError(err)
	}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		result.NextCursor = domain.EncodeCursor(projectID, q.Sort, q.Order, last.SortKey, strconv.FormatInt(last.Number, 10))
	}

	result.Items = make([]domain.Task, len(rows))
	if len(rows) == 0 {
		return result, nil
	}
	numbers := make([]int64, len(rows))
	positions := make(map[int64]int, len(rows))
	for i, row := range rows {
		numbers[i] = row.Number
		positions[row.Number] = i
		result.Items[i] = domain.Task{
			TaskReference:      row.TaskReference,
			StartDate:          row.StartDate,
			EndDate:            row.EndDate,
			Late:               row.Late,
			Blocked:            row.Blocked,
			DependentCount:     row.DependentCount,
			DirectDependencies: []domain.TaskReference{},
		}
	}
	type listDependency struct {
		TaskNumber int64
		domain.TaskReference
	}
	var dependencies []listDependency
	if err := db.Raw(`SELECT d.task_number, p.id, p.name, p.status
FROM task_dependencies d JOIN tasks p ON p.project_id = d.project_id AND p.number = d.prerequisite_number
WHERE d.project_id = ? AND d.task_number IN ?
ORDER BY d.task_number, d.prerequisite_number`, projectID, numbers).Scan(&dependencies).Error; err != nil {
		return result, dbError(err)
	}
	for _, dependency := range dependencies {
		item := &result.Items[positions[dependency.TaskNumber]]
		item.DirectDependencies = append(item.DirectDependencies, dependency.TaskReference)
	}
	return result, nil
}

func parseTaskListCursor(cursor []string, projectID string, q domain.TaskListQuery, sortKey taskListSort) (int64, error) {
	invalid := domain.Invalid("invalid task cursor")
	if cursor[0] != projectID || cursor[1] != q.Sort || cursor[2] != q.Order {
		return 0, invalid
	}
	number, err := strconv.ParseInt(cursor[4], 10, 64)
	if err != nil || number < 1 || strconv.FormatInt(number, 10) != cursor[4] {
		return 0, invalid
	}
	switch sortKey.cast {
	case "bigint":
		key, err := strconv.ParseInt(cursor[3], 10, 64)
		if err != nil || key < 0 || strconv.FormatInt(key, 10) != cursor[3] || (q.Sort == "id" && key != number) {
			return 0, invalid
		}
	case "date":
		if _, err := domain.ParseDate(cursor[3]); err != nil {
			return 0, invalid
		}
	case "text":
		if cursor[3] == "" || strings.ContainsRune(cursor[3], 0) {
			return 0, invalid
		}
	}
	return number, nil
}
