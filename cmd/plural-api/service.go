package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/post"
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
	case errors.Is(err, postgres.ErrAccountNotFound),
		errors.Is(err, postgres.ErrMediaNotFound),
		errors.Is(err, postgres.ErrPostNotFound),
		errors.Is(err, postgres.ErrCommentNotFound):
		return http.StatusNotFound, errNotFound
	case errors.Is(err, postgres.ErrMediaAlreadyUsed):
		return http.StatusConflict, ErrorResponse{Message: "media already used by another post", Type: post.ErrTypeMediaAlreadyUsed}
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

// fileURL returns the absolute URL of a stored key, or "" for an empty key.
// With -public.url set it uses that base. Otherwise it uses the scheme and
// host of the request, so every client gets a URL it can reach.
func (a *app) fileURL(r *http.Request, key string) string {
	if key == "" {
		return ""
	}
	base := a.publicURL
	if base == "" {
		scheme := "http"
		if r.TLS != nil || (a.trustProxyHeaders && r.Header.Get("X-Forwarded-Proto") == "https") {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return base + "/files/" + key
}

// mediaResponse converts a stored row into the JSON shape. URLs are only set
// when Status is ready.
func (a *app) mediaResponse(r *http.Request, m media.Media) media.Response {
	response := media.Response{
		UUID:          m.UUID,
		Kind:          m.Kind,
		Status:        m.Status,
		Width:         m.Width,
		Height:        m.Height,
		DurationMS:    m.DurationMS,
		FailureReason: m.FailureReason,
	}
	if m.Status == media.StatusReady {
		response.URL = a.fileURL(r, m.DisplayKey)
		response.ThumbnailURL = a.fileURL(r, m.ThumbnailKey)
	}
	return response
}

// viewerUUID returns nil for anonymous requests.
func viewerUUID(viewer *account.Viewer) *uuid.UUID {
	if viewer == nil {
		return nil
	}
	return &viewer.UUID
}

// parseUUIDPath reads a path value as a UUID. ok is false when it is not one.
func parseUUIDPath(r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	return id, err == nil
}
