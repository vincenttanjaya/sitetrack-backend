package httpapi

import (
	"net/http"

	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func (r *Router) registerDeliveries(service *application.InventoryService, path string, company bool) {
	register := func(method, suffix string, handler func(http.ResponseWriter, *http.Request, domain.Actor, domain.DeliveryScope) error) {
		r.authenticated(method+" "+path+suffix, func(w http.ResponseWriter, req *http.Request, actor domain.Actor) error {
			scope := domain.CompanyDeliveryScope()
			if company {
				if !actor.CanAccessCompanyStore() {
					return domain.ErrForbidden
				}
			} else {
				scope = domain.ProjectDeliveryScope(req.PathValue("projectID"))
			}
			return handler(w, req, actor, scope)
		})
	}
	register("GET", "", func(w http.ResponseWriter, req *http.Request, actor domain.Actor, scope domain.DeliveryScope) error {
		fields := []string{"direction", "status", "start_date", "end_date"}
		if company {
			fields = append(fields, "project_id")
		}
		q, p, err := inventoryQuery(req, fields...)
		if err != nil {
			return err
		}
		out, err := service.ListDeliveries(req.Context(), actor, scope, domain.DeliveryFilter{
			ProjectID: q.Get("project_id"), Direction: domain.DeliveryDirection(q.Get("direction")), Status: domain.DeliveryStatus(q.Get("status")),
			StartDate: q.Get("start_date"), EndDate: q.Get("end_date"), Page: p,
		})
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, out)
	})
	register("GET", "/{deliveryID}", func(w http.ResponseWriter, req *http.Request, actor domain.Actor, scope domain.DeliveryScope) error {
		id, err := positiveID(req.PathValue("deliveryID"))
		if err != nil {
			return err
		}
		out, err := service.GetDelivery(req.Context(), actor, scope, id)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, out)
	})
	register("POST", "", func(w http.ResponseWriter, req *http.Request, actor domain.Actor, scope domain.DeliveryScope) error {
		var destinationProject string
		var lines []domain.DeliveryLineInput
		if company {
			var body struct {
				ProjectID string                     `json:"project_id"`
				Lines     []domain.DeliveryLineInput `json:"lines"`
			}
			if err := decodeJSON(w, req, &body); err != nil {
				return err
			}
			destinationProject, lines = body.ProjectID, body.Lines
		} else {
			var body struct {
				Lines []domain.DeliveryLineInput `json:"lines"`
			}
			if err := decodeJSON(w, req, &body); err != nil {
				return err
			}
			lines = body.Lines
		}
		out, err := service.InitiateDelivery(req.Context(), actor, scope, destinationProject, lines)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusCreated, out)
	})
	for _, action := range []domain.DeliveryAction{domain.RecordDelivery, domain.RejectDelivery, domain.CancelDelivery} {
		register("POST", "/{deliveryID}/"+string(action), func(w http.ResponseWriter, req *http.Request, actor domain.Actor, scope domain.DeliveryScope) error {
			id, err := positiveID(req.PathValue("deliveryID"))
			if err != nil {
				return err
			}
			out, err := service.ResolveDelivery(req.Context(), actor, scope, id, action)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, out)
		})
	}
}
