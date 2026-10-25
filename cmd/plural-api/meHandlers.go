package main

import (
	"net/http"

	"github.com/plural-social/plural-api/internal/account"
)

func (a *app) setUpMeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /me", a.endpoint(a.requireViewer(a.getMeEndpoint)))
}

func (a *app) getMeEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	me, err := a.accountStore.GetMe(r.Context(), viewer.UUID)
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusOK, me
}
