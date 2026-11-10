package domain

type TaskStatus string

const (
	TaskNotStarted TaskStatus = "not_started"
	TaskInProgress TaskStatus = "in_progress"
	TaskDone       TaskStatus = "done"
)

func (s TaskStatus) Valid() bool {
	return s == TaskNotStarted || s == TaskInProgress || s == TaskDone
}

type TaskAction string

const (
	TaskStart  TaskAction = "start"
	TaskFinish TaskAction = "done"
	TaskReopen TaskAction = "reopen"
)

type TaskInput struct {
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type DependencyInput struct {
	PrerequisiteID string `json:"prerequisite_id"`
}

type TaskReference struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Status TaskStatus `json:"status"`
}

type Task struct {
	TaskReference
	StartDate          string          `json:"start_date"`
	EndDate            string          `json:"end_date"`
	Late               bool            `json:"late"`
	Blocked            bool            `json:"blocked"`
	DirectDependencies []TaskReference `json:"direct_dependencies"`
	DependentCount     int             `json:"dependent_count"`
}

type TaskDistance struct {
	TaskReference
	Distance int `json:"distance"`
}

type TaskDetail struct {
	Task
	Prerequisites  []TaskDistance `json:"prerequisites"`
	Dependents     []TaskDistance `json:"dependents"`
	AllowedActions []TaskAction   `json:"allowed_actions"`
}

type TaskListQuery struct {
	Page    Page
	Search  string
	Status  TaskStatus
	Late    *bool
	Blocked *bool
	From    string
	To      string
	Sort    string
	Order   string
}

func (t TaskDetail) WithStatus(status TaskStatus, today string) TaskDetail {
	t.Status = status
	t.Late = (status != TaskDone && t.EndDate < today) || (status == TaskNotStarted && t.StartDate < today)
	t.Blocked = false
	startedDependent := false
	for _, prerequisite := range t.Prerequisites {
		if prerequisite.Status != TaskDone {
			t.Blocked = status != TaskDone
			break
		}
	}
	for _, dependent := range t.Dependents {
		if dependent.Status != TaskNotStarted {
			startedDependent = true
			break
		}
	}

	t.AllowedActions = make([]TaskAction, 0, 1)
	switch {
	case status == TaskNotStarted && !t.Blocked:
		t.AllowedActions = append(t.AllowedActions, TaskStart)
	case status == TaskInProgress:
		t.AllowedActions = append(t.AllowedActions, TaskFinish)
	case status == TaskDone && !startedDependent:
		t.AllowedActions = append(t.AllowedActions, TaskReopen)
	}
	return t
}

func (t TaskDetail) ApplyAction(action TaskAction, today string) (TaskDetail, error) {
	if len(t.AllowedActions) == 0 || t.AllowedActions[0] != action {
		return TaskDetail{}, Conflict("action is unavailable for this task")
	}
	if action == TaskFinish {
		return t.WithStatus(TaskDone, today), nil
	}
	return t.WithStatus(TaskInProgress, today), nil
}
