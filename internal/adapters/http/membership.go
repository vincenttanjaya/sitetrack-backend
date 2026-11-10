package httpapi

import (
	"net/http"

	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func (r *Router) registerMembers(s *application.ProjectService) {
	r.authenticated("GET /v1/projects/{projectID}/members", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		p, err := paginationOnly(req)
		if err != nil {
			return err
		}
		result, err := s.Members(req.Context(), a, req.PathValue("projectID"), p)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, result)
	})
	r.authenticated("PUT /v1/projects/{projectID}/members/{userID}", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		id, err := positiveID(req.PathValue("userID"))
		if err != nil {
			return err
		}
		var input struct {
			IsAdmin *bool `json:"is_admin"`
		}
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		if input.IsAdmin == nil {
			return domain.Invalid("is_admin is required")
		}
		if err := s.SetMemberAdmin(req.Context(), a, req.PathValue("projectID"), id, *input.IsAdmin); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
	r.authenticated("DELETE /v1/projects/{projectID}/members/{userID}", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		id, err := positiveID(req.PathValue("userID"))
		if err != nil {
			return err
		}
		if err := s.RemoveMember(req.Context(), a, req.PathValue("projectID"), id); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

func paginationOnly(req *http.Request) (domain.Page, error) {
	q, err := parseQuery(req, "limit", "cursor")
	if err != nil {
		return domain.Page{}, err
	}
	return parsePage(q)
}
