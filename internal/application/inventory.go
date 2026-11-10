package application

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
	"github.com/vincenttanjaya/sitetrack-backend/internal/ports"
)

type InventoryService struct {
	repo     ports.InventoryRepository
	location *time.Location
}

func NewInventory(repo ports.InventoryRepository, location *time.Location) *InventoryService {
	if location == nil {
		location = time.UTC
	}
	return &InventoryService{repo: repo, location: location}
}

func inventoryActor(actor domain.Actor) error {
	if actor.UserID <= 0 || !actor.Role.Valid() {
		return domain.ErrUnauthorized
	}
	return nil
}

func companyStoreActor(actor domain.Actor) error {
	if err := inventoryActor(actor); err != nil {
		return err
	}
	if !actor.CanAccessCompanyStore() {
		return domain.ErrForbidden
	}
	return nil
}

func inventoryText(input, field string, max int, optional bool) (string, error) {
	value := strings.TrimSpace(input)
	if (!optional && value == "") || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || utf8.RuneCountInString(value) > max {
		return "", domain.Invalid("invalid " + field)
	}
	return value, nil
}

func inventoryProjectIDValid(id string) bool {
	return id != "" && len(id) <= 64 && utf8.ValidString(id) && !strings.ContainsRune(id, 0)
}

func inventoryID(id int64) error {
	if id <= 0 {
		return domain.Invalid("invalid numeric ID")
	}
	return nil
}

func inventoryListOptions(search, category, sort, order *string, sorts ...string) error {
	var err error
	if *search, err = inventoryText(*search, "search", 200, true); err != nil {
		return err
	}
	if *category, err = inventoryText(*category, "category", 120, true); err != nil {
		return err
	}
	if *sort == "" {
		*sort = sorts[0]
	}
	if !slices.Contains(sorts, *sort) {
		return domain.Invalid("unsupported sort")
	}
	if *order == "" {
		*order = "asc"
	}
	if *order != "asc" && *order != "desc" {
		return domain.Invalid("order must be asc or desc")
	}
	return nil
}
