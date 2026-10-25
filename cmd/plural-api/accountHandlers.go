package main

import (
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/plural-social/plural-api/internal/account"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{1,30}$`)

func (a *app) setUpAccountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /accounts", a.endpoint(a.rateLimited(a.signupLimiter, a.postAccountEndpoint)))
}

func (a *app) postAccountEndpoint(r *http.Request) (int, any) {
	ctx := r.Context()

	request, err := decodeJSONBody[account.CreateAccountRequest](r)
	if err != nil {
		return http.StatusBadRequest, ErrorResponse{Message: err.Error()}
	}
	request.Username = strings.ToLower(strings.TrimSpace(request.Username))
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))

	if failures := validateCreateAccountRequest(request); len(failures) > 0 {
		return http.StatusBadRequest, ErrorResponse{
			Message: "invalid request body",
			Type:    account.ErrTypeValidation,
			Details: failures,
		}
	}

	passwordHash, err := account.HashPassword(request.Password)
	if err != nil {
		return a.handleError(fmt.Errorf("hashing password: %w", err))
	}

	created, err := a.accountStore.InsertAccount(ctx, account.NewAccount{
		Username:     request.Username,
		Email:        request.Email,
		PasswordHash: passwordHash,
	})
	if err != nil {
		return a.handleError(err)
	}

	return http.StatusCreated, created
}

func validateCreateAccountRequest(request account.CreateAccountRequest) map[string]any {
	failures := map[string]any{}
	if !usernamePattern.MatchString(request.Username) {
		failures["username"] = "must be 1-30 characters of a-z, 0-9 or _"
	}
	if address, err := mail.ParseAddress(request.Email); err != nil || address.Address != request.Email {
		failures["email"] = "must be a valid email address"
	}
	// The upper bound caps hashing cost for oversized inputs.
	if length := utf8.RuneCountInString(request.Password); length < 8 || length > 128 {
		failures["password"] = "must be 8-128 characters"
	}
	return failures
}
