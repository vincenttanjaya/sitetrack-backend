package postgres

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"gorm.io/gorm"
)

type ProjectRepository struct{ db *gorm.DB }

func NewProjectRepository(db *gorm.DB) *ProjectRepository { return &ProjectRepository{db: db} }

func (r *ProjectRepository) Create(ctx context.Context, a domain.Actor, in domain.ProjectInput) (domain.Project, error) {
	p := domain.Project{ProjectInput: in, IsAdmin: true, MemberCount: 1}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw("INSERT INTO projects (name,start_date,end_date) VALUES (?,?::date,?::date) RETURNING id", in.Name, in.StartDate, in.EndDate).Row().Scan(&p.ID); err != nil {
			return dbError(err)
		}
		if err := tx.Table("project_members").Create(map[string]any{"project_id": p.ID, "user_id": a.UserID, "is_admin": true}).Error; err != nil {
			return dbError(err)
		}
		return dbError(tx.Table("stores").Create(map[string]any{"project_id": p.ID}).Error)
	})
	return p, err
}

func projectAdmin(tx *gorm.DB, a domain.Actor, id string) error {
	admin, err := projectAccess(tx, a, id, true)
	if err != nil {
		return err
	}
	if !admin {
		return domain.ErrForbidden
	}
	return nil
}

func (r *ProjectRepository) Update(ctx context.Context, a domain.Actor, id string, in domain.ProjectInput, today time.Time) (domain.Project, error) {
	var project domain.Project
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := projectAdmin(tx, a, id); err != nil {
			return err
		}
		if err := tx.Table("projects").Where("id = ?", id).Updates(map[string]any{
			"name": in.Name, "start_date": gorm.Expr("?::date", in.StartDate), "end_date": gorm.Expr("?::date", in.EndDate),
		}).Error; err != nil {
			return dbError(err)
		}
		var err error
		project, err = getProject(tx, a, id, today)
		return err
	})
	return project, err
}

func (r *ProjectRepository) Delete(ctx context.Context, a domain.Actor, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := projectAdmin(tx, a, id); err != nil {
			return err
		}
		var hasInventory bool
		err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM deliveries WHERE project_id = ?)
      OR EXISTS (SELECT 1 FROM stores s JOIN stock k ON k.store_id = s.id
      WHERE s.project_id = ? AND (k.quantity > 0 OR k.outbound_quantity > 0))`, id, id).Row().Scan(&hasInventory)
		if err != nil {
			return dbError(err)
		}
		if hasInventory {
			return domain.Conflict("project has stock or delivery history")
		}
		return dbError(tx.Exec("DELETE FROM projects WHERE id = ?", id).Error)
	})
}

func (r *ProjectRepository) Get(ctx context.Context, a domain.Actor, id string, today time.Time) (domain.Project, error) {
	return getProject(r.db.WithContext(ctx), a, id, today)
}

func getProject(db *gorm.DB, a domain.Actor, id string, today time.Time) (domain.Project, error) {
	if id == "" {
		return domain.Project{}, domain.ErrNotFound
	}
	page, err := projectOverview(db, a, domain.ProjectFilter{Sort: "name", Order: "asc", Page: domain.Page{Limit: 1}}, id, today)
	if err != nil {
		return domain.Project{}, err
	}
	if len(page.Items) == 0 {
		return domain.Project{}, domain.ErrNotFound
	}
	return page.Items[0], nil
}

func (r *ProjectRepository) List(ctx context.Context, a domain.Actor, f domain.ProjectFilter, today time.Time) (domain.PageResult[domain.Project], error) {
	return projectOverview(r.db.WithContext(ctx), a, f, "", today)
}

const projectLateSQL = `((t.status <> 'done' AND t.end_date < ?::date)
    OR (t.status = 'not_started' AND t.start_date < ?::date))`
const projectPercentSQL = `COALESCE(round(10000.0 * count(*) FILTER (WHERE t.status = 'done') / NULLIF(count(*),0)),0)::bigint`

func projectOverview(db *gorm.DB, a domain.Actor, f domain.ProjectFilter, id string, today time.Time) (domain.PageResult[domain.Project], error) {
	result := domain.PageResult[domain.Project]{Items: []domain.Project{}}
	args := []any{a.UserID}
	var sortKey, sortJoin string
	switch f.Sort {
	case "name":
		sortKey = "lower(p.name)"
	case "start_date":
		sortKey = "p.start_date"
	case "percent_done":
		sortKey = "metric.value"
		sortJoin = "CROSS JOIN LATERAL (SELECT " + projectPercentSQL + " AS value FROM tasks t WHERE t.project_id = p.id) metric"
	case "late_tasks":
		sortKey = "metric.value"
		sortJoin = "CROSS JOIN LATERAL (SELECT count(*) AS value FROM tasks t WHERE t.project_id = p.id AND " + projectLateSQL + ") metric"
		args = append(args, today, today)
	default:
		return result, domain.Invalid("invalid project sort")
	}
	query := `WITH ranked AS (
  SELECT p.id,p.number,p.name,p.start_date,p.end_date,COALESCE(m.is_admin,false) AS is_admin,` + sortKey + ` AS sort_key
  FROM projects p LEFT JOIN project_members m ON m.project_id=p.id AND m.user_id=?
  ` + sortJoin + ` WHERE (? OR m.user_id IS NOT NULL)`
	args = append(args, a.Role == domain.RoleSuperAdmin)
	if id != "" {
		query += " AND p.id = ?"
		args = append(args, id)
	}
	if f.Search != "" {
		pattern := containsPattern(f.Search)
		query += " AND (p.name ILIKE ? OR p.id ILIKE ?)"
		args = append(args, pattern, pattern)
	}
	if f.LateOnly {
		if f.Sort == "late_tasks" {
			query += " AND metric.value > 0"
		} else {
			query += " AND EXISTS (SELECT 1 FROM tasks t WHERE t.project_id=p.id AND " + projectLateSQL + ")"
			args = append(args, today, today)
		}
	}
	query += "), page AS (SELECT * FROM ranked"
	direction, operator := "ASC", ">"
	if f.Order == "desc" {
		direction, operator = "DESC", "<"
	}
	if f.Page.Cursor != "" {
		key, number, err := decodeProjectCursor(f)
		if err != nil {
			return result, err
		}
		query += " WHERE (sort_key,number) " + operator + " (?,?)"
		args = append(args, key, number)
	}
	query += " ORDER BY sort_key " + direction + ", number " + direction + " LIMIT ?) "
	args = append(args, f.Page.Size()+1)
	query += `SELECT p.id,p.name,p.start_date::text,p.end_date::text,p.is_admin,
   (SELECT count(*) FROM project_members m WHERE m.project_id=p.id),
   stats.not_started,stats.in_progress,stats.done,stats.percent_done,stats.late_tasks,stats.blocked_tasks,
   p.sort_key::text,p.number
  FROM page p
  CROSS JOIN LATERAL (
   SELECT count(*) FILTER (WHERE t.status='not_started') AS not_started,
    count(*) FILTER (WHERE t.status='in_progress') AS in_progress,
    count(*) FILTER (WHERE t.status='done') AS done,
    ` + projectPercentSQL + ` AS percent_done,
    count(*) FILTER (WHERE ` + projectLateSQL + `) AS late_tasks,
    count(*) FILTER (WHERE t.status <> 'done' AND EXISTS (
      SELECT 1 FROM task_dependencies d JOIN tasks prerequisite
        ON prerequisite.project_id=d.project_id AND prerequisite.number=d.prerequisite_number
      WHERE d.project_id=t.project_id AND d.task_number=t.number AND prerequisite.status <> 'done'
    )) AS blocked_tasks
   FROM tasks t WHERE t.project_id=p.id
  ) stats
  ORDER BY p.sort_key ` + direction + ", p.number " + direction
	args = append(args, today, today)
	rows, err := db.Raw(query, args...).Rows()
	if err != nil {
		return result, dbError(err)
	}
	defer rows.Close()
	var lastKey string
	var lastNumber int64
	for rows.Next() {
		if len(result.Items) == f.Page.Size() {
			result.NextCursor = domain.EncodeCursor(f.Sort, f.Order, lastKey, strconv.FormatInt(lastNumber, 10))
			break
		}
		var p domain.Project
		var percent int64
		if err := rows.Scan(&p.ID, &p.Name, &p.StartDate, &p.EndDate, &p.IsAdmin, &p.MemberCount,
			&p.TaskCounts.NotStarted, &p.TaskCounts.InProgress, &p.TaskCounts.Done, &percent, &p.LateTasks, &p.BlockedTasks, &lastKey, &lastNumber); err != nil {
			return result, err
		}
		p.PercentDone = float64(percent) / 100
		result.Items = append(result.Items, p)
	}
	return result, rows.Err()
}

func decodeProjectCursor(f domain.ProjectFilter) (any, int64, error) {
	parts, err := domain.DecodeCursor(f.Page.Cursor, 4)
	invalid := domain.Invalid("invalid cursor for this project sort")
	if err != nil || parts[0] != f.Sort || parts[1] != f.Order {
		return nil, 0, invalid
	}
	number, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || number < 1 {
		return nil, 0, invalid
	}
	var key any = parts[2]
	switch f.Sort {
	case "name":
		if parts[2] == "" || len(parts[2]) > 800 || strings.ContainsRune(parts[2], 0) {
			return nil, 0, invalid
		}
	case "start_date":
		key, err = domain.ParseDate(parts[2])
	case "percent_done", "late_tasks":
		n, parseErr := strconv.ParseInt(parts[2], 10, 64)
		if parseErr != nil || n < 0 {
			return nil, 0, invalid
		}
		key = n
	}
	if err != nil {
		return nil, 0, invalid
	}
	return key, number, nil
}
