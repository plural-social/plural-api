package main

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/social"
)

func (a *app) setUpUserRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /users/{username}", a.endpoint(a.optionalViewer(a.getUserEndpoint)))
	mux.HandleFunc("GET /users/{username}/posts", a.endpoint(a.optionalViewer(a.getUserPostsEndpoint)))
	mux.HandleFunc("PUT /users/{username}/follow", a.endpoint(a.requireViewer(a.putFollowEndpoint)))
	mux.HandleFunc("DELETE /users/{username}/follow", a.endpoint(a.requireViewer(a.deleteFollowEndpoint)))
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
	return http.StatusOK, a.postPage(r, rows, limit)
}

// resolveUsername turns the {username} path value into an account UUID.
// A non-zero errStatus means the caller should return errStatus and errBody as is.
func (a *app) resolveUsername(r *http.Request) (accountUUID uuid.UUID, errStatus int, errBody any) {
	username := r.PathValue("username")
	if !usernamePattern.MatchString(username) {
		return uuid.Nil, http.StatusNotFound, errNotFound
	}
	accountUUID, err := a.accountStore.GetAccountUUID(r.Context(), username)
	if err != nil {
		status, body := a.handleError(err)
		return uuid.Nil, status, body
	}
	return accountUUID, 0, nil
}

func (a *app) putFollowEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	followeeUUID, status, body := a.resolveUsername(r)
	if status != 0 {
		return status, body
	}
	if followeeUUID == viewer.UUID {
		return http.StatusUnprocessableEntity, ErrorResponse{
			Message: "you can't follow yourself",
			Type:    social.ErrTypeCannotFollowSelf,
		}
	}
	if err := a.socialStore.Follow(r.Context(), viewer.UUID, followeeUUID); err != nil {
		return a.handleError(err)
	}
	return http.StatusNoContent, nil
}

func (a *app) deleteFollowEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	followeeUUID, status, body := a.resolveUsername(r)
	if status != 0 {
		return status, body
	}
	if err := a.socialStore.Unfollow(r.Context(), viewer.UUID, followeeUUID); err != nil {
		return a.handleError(err)
	}
	return http.StatusNoContent, nil
}
