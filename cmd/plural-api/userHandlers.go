package main

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/account"
)

func (a *app) setUpUserRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /users/{username}", a.endpoint(a.optionalViewer(a.getUserEndpoint)))
}

func (a *app) getUserEndpoint(r *http.Request, viewer *account.Viewer) (int, any) {
	username := r.PathValue("username")
	// A name that cannot exist is a 404 without a database round trip.
	if !usernamePattern.MatchString(username) {
		return http.StatusNotFound, errNotFound
	}

	profile, err := a.accountStore.GetProfile(r.Context(), username, viewerUUID(viewer))
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusOK, profile
}

// viewerUUID returns nil for anonymous requests.
func viewerUUID(viewer *account.Viewer) *uuid.UUID {
	if viewer == nil {
		return nil
	}
	return &viewer.UUID
}
