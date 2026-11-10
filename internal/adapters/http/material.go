package httpapi

import (
	"net/http"

	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func (r *Router) registerMaterials(service *application.InventoryService) {
	r.authenticated("GET /v1/materials", func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		q, p, err := inventoryQuery(req, "search", "category", "sort", "order")
		if err != nil {
			return err
		}
		out, err := service.ListMaterials(req.Context(), actor, domain.MaterialFilter{
			Search: q.Get("search"), Category: q.Get("category"), Sort: q.Get("sort"), Order: q.Get("order"), Page: p,
		})
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, out)
	})
	r.authenticated("POST /v1/materials", func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		var input domain.MaterialInput
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		out, err := service.CreateMaterial(req.Context(), actor, input)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusCreated, out)
	})
	r.authenticated("GET /v1/materials/{materialID}", func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		id, err := positiveID(req.PathValue("materialID"))
		if err != nil {
			return err
		}
		out, err := service.GetMaterial(req.Context(), actor, id)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, out)
	})
	r.authenticated("PATCH /v1/materials/{materialID}", func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		id, err := positiveID(req.PathValue("materialID"))
		if err != nil {
			return err
		}
		var input domain.MaterialPatch
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		out, err := service.UpdateMaterial(req.Context(), actor, id, input)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, out)
	})
	r.authenticated("DELETE /v1/materials/{materialID}", func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
		id, err := positiveID(req.PathValue("materialID"))
		if err != nil {
			return err
		}
		if err := service.DeleteMaterial(req.Context(), actor, id); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}
