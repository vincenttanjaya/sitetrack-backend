package httpapi

import (
	"net/http"
	"net/url"

	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func (r *Router) RegisterInventoryRoutes(inventory *application.InventoryService) {
	r.registerMaterials(inventory)
	r.registerStock(inventory)
	r.registerDeliveries(inventory, "/v1/company/deliveries", true)
	r.registerDeliveries(inventory, "/v1/projects/{projectID}/deliveries", false)
}

func inventoryQuery(req *http.Request, fields ...string) (url.Values, domain.Page, error) {
	query, err := parseQuery(req, append(fields, "limit", "cursor")...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	for _, field := range fields {
		if query.Has(field) && query.Get(field) == "" && field != "search" && field != "category" {
			return nil, domain.Page{}, domain.Invalid(field + " must not be empty")
		}
	}
	page, err := parsePage(query)
	return query, page, err
}
