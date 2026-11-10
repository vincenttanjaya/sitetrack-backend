package httpapi

import (
	"net/http"

	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func (r *Router) registerStock(service *application.InventoryService) {
	r.authenticated("GET /v1/company/store", func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		if !actor.CanAccessCompanyStore() {
			return domain.ErrForbidden
		}
		q, p, err := inventoryQuery(req, "search", "category", "status", "sort", "order")
		if err != nil {
			return err
		}
		out, err := service.ListCompanyStock(req.Context(), actor, domain.CompanyStockFilter{
			Search: q.Get("search"), Category: q.Get("category"), Status: domain.StockStatus(q.Get("status")),
			Sort: q.Get("sort"), Order: q.Get("order"), Page: p,
		})
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, out)
	})
	r.authenticated("GET /v1/projects/{projectID}/store", func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		q, p, err := inventoryQuery(req, "search", "category", "sort", "order")
		if err != nil {
			return err
		}
		out, err := service.ListProjectStock(req.Context(), actor, req.PathValue("projectID"), domain.ProjectStockFilter{
			Search: q.Get("search"), Category: q.Get("category"), Sort: q.Get("sort"), Order: q.Get("order"), Page: p,
		})
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, out)
	})
	r.authenticated("POST /v1/company/store/{materialID}/add-stock", func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		if !actor.CanAccessCompanyStore() {
			return domain.ErrForbidden
		}
		id, err := positiveID(req.PathValue("materialID"))
		if err != nil {
			return err
		}
		var input struct {
			Quantity *domain.Quantity `json:"quantity"`
		}
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		if input.Quantity == nil {
			return domain.Invalid("quantity is required")
		}
		out, err := service.AddStock(req.Context(), actor, id, *input.Quantity)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, out)
	})
	r.authenticated("PUT /v1/company/store/{materialID}/reorder-point", func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		if !actor.CanAccessCompanyStore() {
			return domain.ErrForbidden
		}
		id, err := positiveID(req.PathValue("materialID"))
		if err != nil {
			return err
		}
		var input struct {
			ReorderPoint *domain.Quantity `json:"reorder_point"`
		}
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		if input.ReorderPoint == nil {
			return domain.Invalid("reorder_point is required")
		}
		out, err := service.SetReorderPoint(req.Context(), actor, id, *input.ReorderPoint)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, out)
	})
}
