package postgres

import (
	"encoding/json"
	"fmt"
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

type taskGraphNode struct {
	reference domain.TaskReference
	neighbors []int64
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

	// Recursive cardinality estimates can reflect the dense parts of a project
	// and trigger JIT compilation even for a tiny reachable component.
	if err := db.Exec("SET LOCAL jit = off").Error; err != nil {
		return domain.TaskDetail{}, dbError(err)
	}
	forward, err := readTaskGraph(db, projectID, number, false)
	if err != nil {
		return domain.TaskDetail{}, err
	}
	reverse, err := readTaskGraph(db, projectID, number, true)
	if err != nil {
		return domain.TaskDetail{}, err
	}

	task.Prerequisites = taskDistances(number, forward)
	task.Dependents = taskDistances(number, reverse)
	task.DependentCount = len(reverse[number].neighbors)
	task.DirectDependencies = make([]domain.TaskReference, 0, len(forward[number].neighbors))
	for _, item := range task.Prerequisites {
		if item.Distance > 1 {
			break
		}
		task.DirectDependencies = append(task.DirectDependencies, item.TaskReference)
	}
	return task.WithStatus(task.Status, today), nil
}

func readTaskGraph(db *gorm.DB, projectID string, number int64, reverse bool) (map[int64]taskGraphNode, error) {
	source, target := "task_number", "prerequisite_number"
	if reverse {
		source, target = target, source
	}
	// UNION visits each vertex once even when many paths reach it. The
	// parameterized lookup uses the edge index in the chosen direction.
	// Return adjacency once per reachable vertex for the shortest-path walk.
	query := fmt.Sprintf(`WITH RECURSIVE reachable(number) AS (
    SELECT CAST(? AS bigint)
    UNION
    SELECT adjacent.number FROM reachable r
    CROSS JOIN LATERAL (
        SELECT %[2]s AS number FROM task_dependencies
        WHERE project_id = ? AND %[1]s = r.number
        OFFSET 0
    ) adjacent
)
SELECT r.number, t.id, t.name, t.status, to_json(ARRAY(
    SELECT %[2]s FROM task_dependencies WHERE project_id = t.project_id AND %[1]s = r.number
))
FROM reachable r JOIN tasks t ON t.project_id = ? AND t.number = r.number`, source, target)
	rows, err := db.Raw(query, number, projectID, projectID).Rows()
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()

	graph := make(map[int64]taskGraphNode)
	for rows.Next() {
		var vertex int64
		var node taskGraphNode
		var neighbors []byte
		if err := rows.Scan(&vertex, &node.reference.ID, &node.reference.Name, &node.reference.Status, &neighbors); err != nil {
			return nil, dbError(err)
		}
		if err := json.Unmarshal(neighbors, &node.neighbors); err != nil {
			return nil, fmt.Errorf("decode task graph adjacency: %w", err)
		}
		graph[vertex] = node
	}
	return graph, dbError(rows.Err())
}

func taskDistances(number int64, graph map[int64]taskGraphNode) []domain.TaskDistance {
	type hop struct {
		number   int64
		distance int
	}
	queue := []hop{{number: number}}
	seen := map[int64]bool{number: true}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		for _, neighbor := range graph[current.number].neighbors {
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
		items = append(items, domain.TaskDistance{TaskReference: graph[hop.number].reference, Distance: hop.distance})
	}
	return items
}
