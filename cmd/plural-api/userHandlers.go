package main

import (
	"net/http"

	"github.com/plural-social/plural-api/internal/account"
)

func (a *app) setUpUserRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /users/{username}", a.endpoint(a.optionalViewer(a.getUserEndpoint)))
	mux.HandleFunc("GET /users/{username}/posts", a.endpoint(a.optionalViewer(a.getUserPostsEndpoint)))
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

func (a *app) getUserPostsEndpoint(r *http.Request, viewer *account.Viewer) (int, any) {
	ctx := r.Context()

	username := r.PathValue("username")
	if !usernamePattern.MatchString(username) {
		return http.StatusNotFound, errNotFound
	}
	cursor, limit, errResponse := pageParams(r)
	if errResponse != nil {
		return http.StatusBadRequest, *errResponse
	}

	profile, err := a.accountStore.GetProfile(ctx, username, viewerUUID(viewer))
	if err != nil {
		return a.handleError(err)
	}
	rows, err := a.postStore.ListUserPosts(ctx, profile.UUID, viewerUUID(viewer), cursor, limit)
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusOK, a.postsPage(r, rows, limit)
}
