package domain

import "time"

type ProjectInput struct {
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type Project struct {
	ID string `json:"id"`
	ProjectInput
	IsAdmin      bool       `json:"is_admin"`
	MemberCount  int64      `json:"member_count"`
	TaskCounts   TaskCounts `json:"task_counts"`
	PercentDone  float64    `json:"percent_done"`
	LateTasks    int64      `json:"late_tasks"`
	BlockedTasks int64      `json:"blocked_tasks"`
}

type TaskCounts struct {
	NotStarted int64 `json:"not_started"`
	InProgress int64 `json:"in_progress"`
	Done       int64 `json:"done"`
}

type ProjectFilter struct {
	Search, Sort, Order string
	LateOnly            bool
	Page                Page
}

type Member struct {
	UserID   int64     `json:"user_id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	IsAdmin  bool      `json:"is_admin"`
	JoinedAt time.Time `json:"joined_at"`
}
