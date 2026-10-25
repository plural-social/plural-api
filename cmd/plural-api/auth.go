package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/postgres"
)

type (
	viewerHandler         func(r *http.Request, viewer account.Viewer) (int, any)
	optionalViewerHandler func(r *http.Request, viewer *account.Viewer) (int, any)
)

var errUnauthenticated = ErrorResponse{Message: "authentication required", Type: account.ErrTypeUnauthenticated}

// bearerToken returns the token from "Authorization: Bearer <token>".
// The scheme match is case-insensitive. ok is false when the header is missing
// or malformed.
func bearerToken(r *http.Request) (token string, ok bool) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

// requireViewer returns 401 when there is no valid token.
func (a *app) requireViewer(h viewerHandler) handler {
	return func(r *http.Request) (int, any) {
		token, ok := bearerToken(r)
		if !ok {
			return http.StatusUnauthorized, errUnauthenticated
		}
		return a.withViewer(r, token, h)
	}
}

// optionalViewer passes nil when there is no Authorization header. A header
// with an invalid or expired token still returns 401, so clients notice
// expired sessions.
func (a *app) optionalViewer(h optionalViewerHandler) handler {
	return func(r *http.Request) (int, any) {
		if r.Header.Get("Authorization") == "" {
			return h(r, nil)
		}
		token, ok := bearerToken(r)
		if !ok {
			return http.StatusUnauthorized, errUnauthenticated
		}
		return a.withViewer(r, token, func(r *http.Request, viewer account.Viewer) (int, any) {
			return h(r, &viewer)
		})
	}
}

func (a *app) withViewer(r *http.Request, token string, h viewerHandler) (int, any) {
	viewer, err := a.accountStore.GetViewer(r.Context(), account.HashSessionToken(token))
	if errors.Is(err, postgres.ErrSessionNotFound) {
		return http.StatusUnauthorized, errUnauthenticated
	}
	if err != nil {
		return a.handleError(err)
	}
	return h(r, viewer)
}
