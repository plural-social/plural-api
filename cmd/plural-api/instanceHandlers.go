package main

import "net/http"

type Instance struct {
	Domain  string `json:"domain"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (a *app) setUpInstanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /instance", a.endpoint(a.getInstanceEndpoint))
}

func (a *app) getInstanceEndpoint(r *http.Request) (int, any) {
	return http.StatusOK, Instance{Domain: a.instanceDomain, Name: a.instanceName, Version: version}
}
