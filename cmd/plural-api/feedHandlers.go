package main

import (
	"net/http"

	"github.com/plural-social/plural-api/internal/account"
)

func (a *app) setUpFeedRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /feed/following", a.endpoint(a.requireViewer(a.getFollowingFeedEndpoint)))
	mux.HandleFunc("GET /feed/explore", a.endpoint(a.optionalViewer(a.getExploreFeedEndpoint)))
}

// getFollowingFeedEndpoint lists posts by the viewer and the people they follow.
func (a *app) getFollowingFeedEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	cursor, limit, errResponse := pageParams(r)
	if errResponse != nil {
		return http.StatusBadRequest, *errResponse
	}
	rows, err := a.socialStore.ListFollowingFeed(r.Context(), viewer.UUID, cursor, limit)
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusOK, a.postPage(r, rows, limit)
}

// getExploreFeedEndpoint lists every post on the instance. Anonymous viewers may read it.
func (a *app) getExploreFeedEndpoint(r *http.Request, viewer *account.Viewer) (int, any) {
	cursor, limit, errResponse := pageParams(r)
	if errResponse != nil {
		return http.StatusBadRequest, *errResponse
	}
	rows, err := a.socialStore.ListExploreFeed(r.Context(), viewerUUID(viewer), cursor, limit)
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusOK, a.postPage(r, rows, limit)
}
