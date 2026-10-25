package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/postgres"
)

const maxRequestBodyBytes = 1 << 20

// Error types shared by every domain.
const (
	errTypeNotFound  = "notFound"
	errTypeForbidden = "forbidden"
)

var errNotFound = ErrorResponse{Message: "not found", Type: errTypeNotFound}

// handler returns a status code and a body that is encoded as JSON.
type handler func(r *http.Request) (int, any)

type ErrorResponse struct {
	// Message is a human-readable description; it may change at any time.
	Message string `json:"error"`
	// Type is a stable, machine-readable identifier for the kind of error.
	Type    string         `json:"type,omitzero"`
	Details map[string]any `json:"details,omitempty"`
}

func (a *app) endpoint(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, body := h(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body == nil {
			return
		}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			a.logger.Error("Error encoding response", zap.Error(err))
		}
	}
}

func decodeJSONBody[T any](r *http.Request) (T, error) {
	var body T
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxRequestBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return body, fmt.Errorf("decoding request body: %w", err)
	}
	return body, nil
}

func (a *app) handleError(err error) (int, any) {
	var usernameConflictErr postgres.UsernameConflictError
	switch {
	case errors.As(err, &usernameConflictErr):
		return http.StatusConflict, ErrorResponse{Message: err.Error(), Type: account.ErrTypeUsernameConflict}
	case errors.As(err, new(postgres.EmailConflictError)):
		return http.StatusConflict, ErrorResponse{Message: err.Error(), Type: account.ErrTypeEmailConflict}
	case errors.Is(err, postgres.ErrAccountNotFound):
		return http.StatusNotFound, errNotFound
	default:
		a.logger.Error("Unhandled error", zap.Error(err))
		return http.StatusInternalServerError, ErrorResponse{Message: "internal server error"}
	}
}

// pageParams reads ?cursor= and ?limit=. On error it returns a 400 response
// that the handler should return as is.
func pageParams(r *http.Request) (pagination.Cursor, int, *ErrorResponse) {
	cursor, err := pagination.Decode(r.URL.Query().Get("cursor"))
	if err != nil {
		return pagination.Cursor{}, 0, &ErrorResponse{Message: err.Error(), Type: pagination.ErrTypeInvalidPagination}
	}
	limit, err := pagination.ParseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		return pagination.Cursor{}, 0, &ErrorResponse{Message: err.Error(), Type: pagination.ErrTypeInvalidPagination}
	}
	return cursor, limit, nil
}
