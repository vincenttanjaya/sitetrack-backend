package postgres

import (
	"sort"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"gorm.io/gorm"
)

type numberedTaskReference struct {
	Number int64
	ID     string
	Name   string
	Status domain.TaskStatus
}

func (t numberedTaskReference) reference() domain.TaskReference {
	return domain.TaskReference{ID: t.ID, Name: t.Name, Status: t.Status}
}

type taskEdge struct {
	TaskNumber         int64
	PrerequisiteNumber int64
}

func readTaskDetail(db *gorm.DB, projectID, taskID, today string) (domain.TaskDetail, error) {
	var task domain.TaskDetail
	var number int64
	err := db.Raw(`SELECT number, id, name, status, to_char(start_date, 'YYYY-MM-DD'), to_char(end_date, 'YYYY-MM-DD')
FROM tasks WHERE project_id = ? AND id = ?`, projectID, taskID).Row().Scan(
		&number, &task.ID, &task.Name, &task.Status, &task.StartDate, &task.EndDate,
	)
	if err != nil {
		return domain.TaskDetail{}, dbError(err)
	}

	var edges []taskEdge
	if err := db.Raw(`SELECT task_number, prerequisite_number FROM task_dependencies WHERE project_id = ?`, projectID).Scan(&edges).Error; err != nil {
		return domain.TaskDetail{}, dbError(err)
	}
	forward := make(map[int64][]int64)
	reverse := make(map[int64][]int64)
	for _, edge := range edges {
		forward[edge.TaskNumber] = append(forward[edge.TaskNumber], edge.PrerequisiteNumber)
		reverse[edge.PrerequisiteNumber] = append(reverse[edge.PrerequisiteNumber], edge.TaskNumber)
	}

	task.Prerequisites = []domain.TaskDistance{}
	task.Dependents = []domain.TaskDistance{}
	task.DirectDependencies = []domain.TaskReference{}
	task.DependentCount = len(reverse[number])
	if len(forward[number])+task.DependentCount > 0 {
		var rows []numberedTaskReference
		if err := db.Raw(`SELECT number, id, name, status FROM tasks WHERE project_id = ?`, projectID).Scan(&rows).Error; err != nil {
			return domain.TaskDetail{}, dbError(err)
		}
		references := make(map[int64]domain.TaskReference, len(rows))
		for _, row := range rows {
			references[row.Number] = row.reference()
		}

		task.Prerequisites = taskDistances(number, forward, references)
		task.Dependents = taskDistances(number, reverse, references)
		for _, item := range task.Prerequisites {
			if item.Distance > 1 {
				break
			}
			task.DirectDependencies = append(task.DirectDependencies, item.TaskReference)
		}
	}
	return task.WithStatus(task.Status, today), nil
}

func taskDistances(number int64, adjacency map[int64][]int64, references map[int64]domain.TaskReference) []domain.TaskDistance {
	type hop struct {
		number   int64
		distance int
	}
	queue := []hop{{number: number}}
	seen := map[int64]bool{number: true}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		for _, neighbor := range adjacency[current.number] {
			if !seen[neighbor] {
				seen[neighbor] = true
				queue = append(queue, hop{number: neighbor, distance: current.distance + 1})
			}
		}
	}
	hops := queue[1:]
	sort.Slice(hops, func(i, j int) bool {
		if hops[i].distance != hops[j].distance {
			return hops[i].distance < hops[j].distance
		}
		return hops[i].number < hops[j].number
	})
	items := make([]domain.TaskDistance, 0, len(hops))
	for _, hop := range hops {
		items = append(items, domain.TaskDistance{TaskReference: references[hop.number], Distance: hop.distance})
	}
	return items
}
