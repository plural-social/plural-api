package main

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/postgres"
)

func (a *app) setUpMeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /me", a.endpoint(a.requireViewer(a.getMeEndpoint)))
	mux.HandleFunc("DELETE /me", a.endpoint(a.rateLimited(a.sessionLimiter, a.requireViewer(a.deleteMeEndpoint))))
	mux.HandleFunc("PATCH /me", a.endpoint(a.requireViewer(a.patchMeEndpoint)))
	mux.HandleFunc("PUT /me/avatar", a.endpoint(a.requireViewer(a.putAvatarEndpoint)))
	mux.HandleFunc("DELETE /me/avatar", a.endpoint(a.requireViewer(a.deleteAvatarEndpoint)))
}

func (a *app) getMeEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	me, err := a.accountStore.GetMe(r.Context(), viewer.UUID)
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusOK, a.meResponse(r, me)
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
	return http.StatusOK, a.meResponse(r, me)
}

// deleteMeEndpoint permanently deletes the viewer's account, content and files.
func (a *app) deleteMeEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	ctx := r.Context()

	request, err := decodeJSONBody[account.DeleteMeRequest](r)
	if err != nil {
		return http.StatusBadRequest, ErrorResponse{Message: err.Error()}
	}

	hash, err := a.accountStore.GetPasswordHash(ctx, viewer.UUID)
	if err != nil {
		return a.handleError(err)
	}
	matches, err := account.VerifyPassword(request.Password, hash)
	if err != nil {
		return a.handleError(err)
	}
	if !matches {
		return http.StatusUnauthorized, errInvalidCredentials
	}

	// Keys are read before the rows go: afterwards nothing says which files were theirs.
	keys, err := a.accountStore.ListMediaKeys(ctx, viewer.UUID)
	if err != nil {
		return a.handleError(err)
	}
	if err := a.accountStore.DeleteAccount(ctx, viewer.UUID); err != nil {
		return a.handleError(err)
	}
	a.deleteBlobs(ctx, keys...)
	return http.StatusNoContent, nil
}

func (a *app) putAvatarEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	ctx := r.Context()

	request, err := decodeJSONBody[account.SetAvatarRequest](r)
	if err != nil {
		return http.StatusBadRequest, ErrorResponse{Message: err.Error()}
	}
	mediaUUID, err := uuid.Parse(request.MediaUUID)
	if err != nil {
		return http.StatusBadRequest, ErrorResponse{
			Message: "invalid request body",
			Type:    account.ErrTypeValidation,
			Details: map[string]any{"mediaUuid": "must be a valid UUID"},
		}
	}

	err = a.accountStore.SetAvatar(ctx, viewer.UUID, mediaUUID)
	if errors.Is(err, postgres.ErrInvalidAvatarMedia) {
		return http.StatusUnprocessableEntity, ErrorResponse{
			Message: "use a ready image you uploaded that is not used by a post",
			Type:    account.ErrTypeInvalidAvatarMedia,
		}
	}
	if err != nil {
		return a.handleError(err)
	}
	return a.respondWithMe(r, viewer)
}

func (a *app) deleteAvatarEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	if err := a.accountStore.RemoveAvatar(r.Context(), viewer.UUID); err != nil {
		return a.handleError(err)
	}
	return a.respondWithMe(r, viewer)
}

// respondWithMe reads the viewer's account back and returns it as a 200.
func (a *app) respondWithMe(r *http.Request, viewer account.Viewer) (int, any) {
	me, err := a.accountStore.GetMe(r.Context(), viewer.UUID)
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusOK, a.meResponse(r, me)
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
