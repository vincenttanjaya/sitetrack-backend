package httpapi

import (
	"net/http"

	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func (r *Router) registerInvitations(s *application.ProjectService) {
	r.authenticated("GET /v1/projects/{projectID}/invitations", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		p, err := paginationOnly(req)
		if err != nil {
			return err
		}
		result, err := s.Invitations(req.Context(), a, req.PathValue("projectID"), p)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, result)
	})
	r.authenticated("POST /v1/projects/{projectID}/invitations", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		var input struct {
			Email string `json:"email"`
		}
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		result, err := s.Invite(req.Context(), a, req.PathValue("projectID"), input.Email)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusCreated, result)
	})
	r.authenticated("DELETE /v1/projects/{projectID}/invitations/{invitationID}", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		id, err := positiveID(req.PathValue("invitationID"))
		if err != nil {
			return err
		}
		if err := s.Withdraw(req.Context(), a, req.PathValue("projectID"), id); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
	r.authenticated("GET /v1/notifications", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		p, err := paginationOnly(req)
		if err != nil {
			return err
		}
		result, err := s.Notifications(req.Context(), a, p)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, result)
	})
	r.authenticated("GET /v1/notifications/unread-count", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		if _, err := parseQuery(req); err != nil {
			return err
		}
		count, err := s.UnreadCount(req.Context(), a)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]int64{"unread_count": count})
	})
	r.authenticated("POST /v1/notifications/{invitationID}/read", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		id, err := positiveID(req.PathValue("invitationID"))
		if err != nil {
			return err
		}
		if err := s.ReadNotification(req.Context(), a, id); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
	r.authenticated("POST /v1/notifications/{invitationID}/respond", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		id, err := positiveID(req.PathValue("invitationID"))
		if err != nil {
			return err
		}
		var input struct {
			Response string `json:"response"`
		}
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		if input.Response != "accept" && input.Response != "reject" {
			return domain.Invalid("response must be accept or reject")
		}
		result, err := s.Respond(req.Context(), a, id, input.Response == "accept")
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, result)
	})
}
