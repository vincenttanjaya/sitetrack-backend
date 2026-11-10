package httpapi

import (
	"net/http"

	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func (r *Router) RegisterProjectRoutes(s *application.ProjectService) {
	r.authenticated("GET /v1/projects", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		q, err := parseQuery(req, "q", "late_only", "sort", "order", "limit", "cursor")
		if err != nil {
			return err
		}
		p, err := parsePage(q)
		if err != nil {
			return err
		}
		late, err := parseBool(q, "late_only")
		if err != nil {
			return err
		}
		if (q.Has("sort") && q.Get("sort") == "") || (q.Has("order") && q.Get("order") == "") {
			return domain.Invalid("sort and order cannot be empty")
		}
		result, err := s.List(req.Context(), a, domain.ProjectFilter{
			Search: q.Get("q"), Sort: q.Get("sort"), Order: q.Get("order"), LateOnly: late != nil && *late, Page: p,
		})
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, result)
	})
	r.authenticated("POST /v1/projects", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		var input domain.ProjectInput
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		result, err := s.Create(req.Context(), a, input)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusCreated, result)
	})
	r.authenticated("GET /v1/projects/{projectID}", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		result, err := s.Get(req.Context(), a, req.PathValue("projectID"))
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, result)
	})
	r.authenticated("PUT /v1/projects/{projectID}", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		var input domain.ProjectInput
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		result, err := s.Update(req.Context(), a, req.PathValue("projectID"), input)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, result)
	})
	r.authenticated("DELETE /v1/projects/{projectID}", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		if err := s.Delete(req.Context(), a, req.PathValue("projectID")); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
	r.registerMembers(s)
	r.registerInvitations(s)
}
