package domain

import "time"

type Invitation struct {
	ID             int64      `json:"id"`
	ProjectID      string     `json:"project_id"`
	ProjectName    string     `json:"project_name"`
	RecipientID    int64      `json:"recipient_id"`
	RecipientName  string     `json:"recipient_name"`
	RecipientEmail string     `json:"recipient_email"`
	SenderID       int64      `json:"sender_id"`
	SenderName     string     `json:"sender_name"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	ReadAt         *time.Time `json:"read_at"`
	RespondedAt    *time.Time `json:"responded_at"`
}

type Notifications struct {
	PageResult[Invitation]
	UnreadCount int64 `json:"unread_count"`
}
