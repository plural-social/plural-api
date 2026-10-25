package main

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/plural-social/plural-api/internal/account"
)

func (a *app) setUpMeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /me", a.endpoint(a.requireViewer(a.getMeEndpoint)))
	mux.HandleFunc("PATCH /me", a.endpoint(a.requireViewer(a.patchMeEndpoint)))
}

func (a *app) getMeEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	me, err := a.accountStore.GetMe(r.Context(), viewer.UUID)
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusOK, me
}

func (a *app) patchMeEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	ctx := r.Context()

	request, err := decodeJSONBody[account.UpdateMeRequest](r)
	if err != nil {
		return http.StatusBadRequest, ErrorResponse{Message: err.Error()}
	}
	update := account.ProfileUpdate{
		DisplayName: trimmed(request.DisplayName),
		Bio:         trimmed(request.Bio),
	}

	if failures := validateUpdateMeRequest(update); len(failures) > 0 {
		return http.StatusBadRequest, ErrorResponse{
			Message: "invalid request body",
			Type:    account.ErrTypeValidation,
			Details: failures,
		}
	}

	if err := a.accountStore.UpdateProfile(ctx, viewer.UUID, update); err != nil {
		return a.handleError(err)
	}
	me, err := a.accountStore.GetMe(ctx, viewer.UUID)
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusOK, me
}

func validateUpdateMeRequest(update account.ProfileUpdate) map[string]any {
	failures := map[string]any{}
	if update.DisplayName == nil && update.Bio == nil {
		failures["body"] = "nothing to update"
		return failures
	}
	if name := update.DisplayName; name != nil {
		if utf8.RuneCountInString(*name) > account.DisplayNameMaxLength {
			failures["displayName"] = "must be at most 50 characters"
		} else if strings.ContainsAny(*name, "\r\n") {
			failures["displayName"] = "must be a single line"
		}
	}
	if bio := update.Bio; bio != nil && utf8.RuneCountInString(*bio) > account.BioMaxLength {
		failures["bio"] = "must be at most 300 characters"
	}
	return failures
}

// trimmed returns a pointer to s without surrounding spaces, keeping nil as nil.
func trimmed(s *string) *string {
	if s == nil {
		return nil
	}
	return new(strings.TrimSpace(*s))
}
