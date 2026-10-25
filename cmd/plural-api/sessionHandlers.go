package main

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/postgres"
)

const maxUserAgentBytes = 200

var errInvalidCredentials = ErrorResponse{
	Message: "invalid login or password",
	Type:    account.ErrTypeInvalidCredentials,
}

func (a *app) setUpSessionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /sessions", a.endpoint(a.rateLimited(a.sessionLimiter, a.postSessionEndpoint)))
	mux.HandleFunc("DELETE /sessions/current", a.endpoint(a.requireViewer(a.deleteCurrentSessionEndpoint)))
}

func (a *app) postSessionEndpoint(r *http.Request) (int, any) {
	ctx := r.Context()

	request, err := decodeJSONBody[account.CreateSessionRequest](r)
	if err != nil {
		return http.StatusBadRequest, ErrorResponse{Message: err.Error()}
	}
	request.Login = strings.ToLower(strings.TrimSpace(request.Login))

	if failures := validateCreateSessionRequest(request); len(failures) > 0 {
		return http.StatusBadRequest, ErrorResponse{
			Message: "invalid request body",
			Type:    account.ErrTypeValidation,
			Details: failures,
		}
	}

	credentials, err := a.accountStore.GetCredentials(ctx, request.Login)
	if errors.Is(err, postgres.ErrAccountNotFound) {
		account.VerifyDummyPassword(request.Password)
		return http.StatusUnauthorized, errInvalidCredentials
	}
	if err != nil {
		return a.handleError(err)
	}

	matches, err := account.VerifyPassword(request.Password, credentials.PasswordHash)
	if err != nil {
		return a.handleError(err)
	}
	if !matches {
		return http.StatusUnauthorized, errInvalidCredentials
	}

	token, tokenHash, err := account.NewSessionToken()
	if err != nil {
		return a.handleError(err)
	}
	expiresAt := a.now().Add(account.SessionLifetime).UTC()
	if err := a.accountStore.InsertSession(ctx, account.NewSession{
		TokenHash:   tokenHash,
		AccountUUID: credentials.AccountUUID,
		UserAgent:   truncateUTF8(r.UserAgent(), maxUserAgentBytes),
		ExpiresAt:   expiresAt,
	}); err != nil {
		return a.handleError(err)
	}

	me, err := a.accountStore.GetMe(ctx, credentials.AccountUUID)
	if err != nil {
		return a.handleError(err)
	}

	return http.StatusCreated, account.SessionResponse{Token: token, ExpiresAt: expiresAt, Me: a.meResponse(r, me)}
}

func validateCreateSessionRequest(request account.CreateSessionRequest) map[string]any {
	failures := map[string]any{}
	if request.Login == "" {
		failures["login"] = "must not be empty"
	}
	if length := utf8.RuneCountInString(request.Password); length < 1 || length > 128 {
		failures["password"] = "must be 1-128 characters"
	}
	return failures
}

// truncateUTF8 cuts s to at most n bytes without splitting a character.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func (a *app) deleteCurrentSessionEndpoint(r *http.Request, _ account.Viewer) (int, any) {
	token, ok := bearerToken(r)
	if !ok {
		return http.StatusUnauthorized, errUnauthenticated
	}
	if err := a.accountStore.DeleteSession(r.Context(), account.HashSessionToken(token)); err != nil {
		return a.handleError(err)
	}
	return http.StatusNoContent, nil
}
