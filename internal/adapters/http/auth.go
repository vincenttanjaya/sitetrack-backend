package httpapi

import (
	"net/http"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func (r *Router) registerAuth() {
	r.public("POST /v1/auth/register", func(w http.ResponseWriter, req *http.Request) error {
		var input domain.RegisterInput
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		user, err := r.auth.Register(req.Context(), input)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusCreated, user)
	})
	r.public("POST /v1/auth/login", func(w http.ResponseWriter, req *http.Request) error {
		var input struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		pair, err := r.auth.Login(req.Context(), input.Email, input.Password)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, pair)
	})
	r.public("POST /v1/auth/refresh", func(w http.ResponseWriter, req *http.Request) error {
		var input struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := decodeJSON(w, req, &input); err != nil {
			return err
		}
		pair, err := r.auth.Refresh(req.Context(), input.RefreshToken)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, pair)
	})
	r.authenticated("POST /v1/auth/logout", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		if err := r.auth.Logout(req.Context(), a); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
	r.authenticated("GET /v1/me", func(w http.ResponseWriter, req *http.Request, a domain.Actor) error {
		user, err := r.auth.Profile(req.Context(), a)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, user)
	})
}
