package domain

import "time"

type DeliveryDirection string

const (
	CompanyToProject DeliveryDirection = "company_to_project"
	ProjectToCompany DeliveryDirection = "project_to_company"
)

func (d DeliveryDirection) Valid() bool {
	return d == CompanyToProject || d == ProjectToCompany
}

type DeliveryStatus string

const (
	DeliveryInTransit DeliveryStatus = "in_transit"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryRejected  DeliveryStatus = "rejected"
	DeliveryCancelled DeliveryStatus = "cancelled"
)

func (s DeliveryStatus) Valid() bool {
	return s == DeliveryInTransit || s == DeliveryDelivered || s == DeliveryRejected || s == DeliveryCancelled
}

type DeliveryAction string

const (
	RecordDelivery DeliveryAction = "record"
	RejectDelivery DeliveryAction = "reject"
	CancelDelivery DeliveryAction = "cancel"
)

func (a DeliveryAction) Result() DeliveryStatus {
	switch a {
	case RecordDelivery:
		return DeliveryDelivered
	case RejectDelivery:
		return DeliveryRejected
	case CancelDelivery:
		return DeliveryCancelled
	default:
		return ""
	}
}

// DeliveryScope binds a route to one acting store. The company store is the zero value.
type DeliveryScope struct{ ProjectID string }

func CompanyDeliveryScope() DeliveryScope          { return DeliveryScope{} }
func ProjectDeliveryScope(id string) DeliveryScope { return DeliveryScope{ProjectID: id} }
func (s DeliveryScope) IsCompany() bool            { return s.ProjectID == "" }

func (s DeliveryScope) CanResolve(direction DeliveryDirection, action DeliveryAction) bool {
	if !direction.Valid() || action.Result() == "" {
		return false
	}
	actingAsSender := s.IsCompany() == (direction == CompanyToProject)
	return actingAsSender == (action == CancelDelivery)
}

type DeliveryLineInput struct {
	MaterialID int64    `json:"material_id"`
	Quantity   Quantity `json:"quantity"`
}

type DeliveryLine struct {
	MaterialID int64    `json:"material_id"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	Unit       string   `json:"unit"`
	Quantity   Quantity `json:"quantity"`
}

type DeliveryProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DeliveryPerson struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Delivery struct {
	ID             int64             `json:"id"`
	Project        DeliveryProject   `json:"project"`
	Direction      DeliveryDirection `json:"direction"`
	Status         DeliveryStatus    `json:"status"`
	Lines          []DeliveryLine    `json:"lines"`
	SentAt         time.Time         `json:"sent_at"`
	SentBy         DeliveryPerson    `json:"sent_by"`
	ReceivedAt     *time.Time        `json:"received_at"`
	ReceivedBy     *DeliveryPerson   `json:"received_by"`
	ResolvedAt     *time.Time        `json:"resolved_at"`
	ResolvedBy     *DeliveryPerson   `json:"resolved_by"`
	AllowedActions []DeliveryAction  `json:"allowed_actions"`
}

type DeliveryFilter struct {
	ProjectID          string
	Direction          DeliveryDirection
	Status             DeliveryStatus
	StartDate, EndDate string
	BusinessTimezone   string
	// Coarse bounds let PostgreSQL use sent_at indexes before testing the local date.
	CoarseSentOnOrAfter, CoarseSentBefore *time.Time
	Page                                  Page
}

const MaxDeliveryLines = 100
