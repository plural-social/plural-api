package main

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/account"
)

func (a *app) setUpLikeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("PUT /posts/{uuid}/like", a.endpoint(a.requireViewer(a.putLikeEndpoint)))
	mux.HandleFunc("DELETE /posts/{uuid}/like", a.endpoint(a.requireViewer(a.deleteLikeEndpoint)))
}

func (a *app) putLikeEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	return a.changeLike(r, viewer, a.socialStore.LikePost)
}

func (a *app) deleteLikeEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	return a.changeLike(r, viewer, a.socialStore.UnlikePost)
}

// changeLike checks the viewer can see the post, then applies change. The
// visibility check stops likes on deleted or hidden posts.
func (a *app) changeLike(r *http.Request, viewer account.Viewer, change func(ctx context.Context, postUUID, accountUUID uuid.UUID) error) (int, any) {
	ctx := r.Context()
	postUUID, ok := parseUUIDPath(r, "uuid")
	if !ok {
		return http.StatusNotFound, errNotFound
	}
	if _, err := a.postStore.GetPost(ctx, postUUID, &viewer.UUID); err != nil {
		return a.handleError(err)
	}
	if err := change(ctx, postUUID, viewer.UUID); err != nil {
		return a.handleError(err)
	}
	return http.StatusNoContent, nil
}
