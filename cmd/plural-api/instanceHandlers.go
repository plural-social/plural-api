package main

import "net/http"

type Instance struct {
	Domain  string `json:"domain"`
	Name    string `json:"name"`
	Version string `json:"version"`
	// TermsURL is shown at sign-up. Empty means the instance has no terms page.
	TermsURL string `json:"termsUrl,omitzero"`
}

func (a *app) setUpInstanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /instance", a.endpoint(a.getInstanceEndpoint))
}

func (a *app) getInstanceEndpoint(r *http.Request) (int, any) {
	return http.StatusOK, Instance{Domain: a.instanceDomain, Name: a.instanceName, Version: version, TermsURL: a.termsURL}
}
