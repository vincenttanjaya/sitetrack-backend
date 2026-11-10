package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/vincenttanjaya/sitetrack-backend/internal/application"
	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

type Router struct {
	mux  *http.ServeMux
	auth *application.AuthService
}

func NewRouter(auth *application.AuthService, ready func(context.Context) error) *Router {
	r := &Router{mux: http.NewServeMux(), auth: auth}
	r.public("GET /healthz", func(w http.ResponseWriter, _ *http.Request) error {
		return writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.public("GET /readyz", func(w http.ResponseWriter, req *http.Request) error {
		ctx, cancel := context.WithTimeout(req.Context(), time.Second)
		defer cancel()
		if ready != nil {
			if err := ready(ctx); err != nil {
				return writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			}
		}
		return writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.registerAuth()
	return r
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	r.mux.ServeHTTP(w, req.WithContext(ctx))
}

func (r *Router) public(pattern string, handler func(http.ResponseWriter, *http.Request) error) {
	r.mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		if err := handler(w, req); err != nil {
			writeError(w, err)
		}
	})
}

func (r *Router) authenticated(pattern string, handler func(http.ResponseWriter, *http.Request, domain.Actor) error) {
	r.public(pattern, func(w http.ResponseWriter, req *http.Request) error {
		fields := strings.Fields(req.Header.Get("Authorization"))
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			return domain.ErrUnauthorized
		}
		actor, err := r.auth.Authenticate(req.Context(), fields[1])
		if err != nil {
			return err
		}
		return handler(w, req, actor)
	})
}

func logServerError(err error) {
	slog.Error("request failed", "error_type", fmtType(err))
}
