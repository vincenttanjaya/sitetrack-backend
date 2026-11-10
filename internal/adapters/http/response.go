package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/vincenttanjaya/sitetrack-backend/internal/domain"
)

func writeJSON(w http.ResponseWriter, status int, body any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Once headers are sent, a disconnect cannot be reported as another response.
	_, _ = w.Write(append(encoded, '\n'))
	return nil
}

func writeError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusInternalServerError, "internal_error", "internal server error"
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, code, message = http.StatusBadRequest, "invalid_input", err.Error()
	case errors.Is(err, domain.ErrUnauthorized):
		w.Header().Set("WWW-Authenticate", "Bearer")
		status, code, message = http.StatusUnauthorized, "unauthorized", "authentication failed"
	case errors.Is(err, domain.ErrForbidden):
		status, code, message = http.StatusForbidden, "forbidden", "insufficient permissions"
	case errors.Is(err, domain.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "resource not found"
	case errors.Is(err, domain.ErrConflict):
		status, code, message = http.StatusConflict, "conflict", err.Error()
	default:
		logServerError(err)
	}
	_ = writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func fmtType(err error) string { return fmt.Sprintf("%T", err) }

func decodeJSON(w http.ResponseWriter, r *http.Request, dest any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return domain.Invalid("expected a JSON body with valid fields")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return domain.Invalid("expected one JSON value")
	}
	return nil
}

func parseQuery(r *http.Request, fields ...string) (url.Values, error) {
	if len(r.URL.RawQuery) > 8192 {
		return nil, domain.Invalid("query is too long")
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, domain.Invalid("invalid query encoding")
	}
	for key, values := range query {
		allowed := false
		for _, field := range fields {
			if key == field {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, domain.Invalid("unsupported query parameter")
		}
		if len(values) != 1 {
			return nil, domain.Invalid("query parameter must have one value")
		}
		if strings.ContainsRune(values[0], 0) {
			return nil, domain.Invalid("invalid query value")
		}
	}
	return query, nil
}

func parsePage(q url.Values) (domain.Page, error) {
	p := domain.Page{Cursor: q.Get("cursor")}
	if q.Has("limit") {
		n, err := strconv.Atoi(q.Get("limit"))
		if err != nil || n < 1 || n > domain.MaxPageSize {
			return p, domain.Invalid("limit must be between 1 and 100")
		}
		p.Limit = n
	}
	if q.Has("cursor") && (p.Cursor == "" || len(p.Cursor) > 2048) {
		return p, domain.Invalid("invalid cursor")
	}
	return p, nil
}

func parseBool(q url.Values, name string) (*bool, error) {
	if !q.Has(name) {
		return nil, nil
	}
	v := q.Get(name)
	if v != "true" && v != "false" {
		return nil, domain.Invalid(name + " must be true or false")
	}
	result := v == "true"
	return &result, nil
}

func positiveID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != raw {
		return 0, domain.Invalid("invalid numeric ID")
	}
	return id, nil
}
