package main

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/postgres"
	"github.com/plural-social/plural-api/internal/social"
)

func (a *app) setUpCommentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /posts/{uuid}/comments", a.endpoint(a.optionalViewer(a.getCommentsEndpoint)))
	mux.HandleFunc("POST /posts/{uuid}/comments", a.endpoint(a.requireViewer(a.postCommentEndpoint)))
	mux.HandleFunc("DELETE /comments/{uuid}", a.endpoint(a.requireViewer(a.deleteCommentEndpoint)))
}

func (a *app) getCommentsEndpoint(r *http.Request, viewer *account.Viewer) (int, any) {
	ctx := r.Context()

	postUUID, ok := parseUUIDPath(r, "uuid")
	if !ok {
		return http.StatusNotFound, errNotFound
	}
	cursor, limit, errResponse := pageParams(r)
	if errResponse != nil {
		return http.StatusBadRequest, *errResponse
	}
	// Comments are visible exactly when their post is.
	if _, err := a.postStore.GetPost(ctx, postUUID, viewerUUID(viewer)); err != nil {
		return a.handleError(err)
	}

	rows, err := a.socialStore.ListComments(ctx, postUUID, viewerUUID(viewer), cursor, limit)
	if err != nil {
		return a.handleError(err)
	}
	page := pagination.NewPage(rows, limit, func(c social.Comment) pagination.Cursor {
		return pagination.Cursor{CreatedAt: c.CreatedAt, UUID: c.UUID}
	})
	for i, comment := range page.Items {
		page.Items[i] = a.commentResponse(r, comment)
	}
	return http.StatusOK, page
}

func (a *app) postCommentEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	ctx := r.Context()

	postUUID, ok := parseUUIDPath(r, "uuid")
	if !ok {
		return http.StatusNotFound, errNotFound
	}
	request, err := decodeJSONBody[social.CreateCommentRequest](r)
	if err != nil {
		return http.StatusBadRequest, ErrorResponse{Message: err.Error()}
	}
	request.Body = strings.TrimSpace(request.Body)
	if failures := validateCreateCommentRequest(request); len(failures) > 0 {
		return http.StatusBadRequest, ErrorResponse{
			Message: "invalid request body",
			Type:    account.ErrTypeValidation,
			Details: failures,
		}
	}

	if _, err := a.postStore.GetPost(ctx, postUUID, &viewer.UUID); err != nil {
		return a.handleError(err)
	}
	commentUUID, err := a.socialStore.InsertComment(ctx, social.NewComment{
		PostUUID:    postUUID,
		AccountUUID: viewer.UUID,
		Body:        request.Body,
	})
	if err != nil {
		return a.handleError(err)
	}
	created, err := a.socialStore.GetComment(ctx, commentUUID, &viewer.UUID)
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusCreated, a.commentResponse(r, created)
}

func validateCreateCommentRequest(request social.CreateCommentRequest) map[string]any {
	failures := map[string]any{}
	if length := utf8.RuneCountInString(request.Body); length < 1 || length > social.CommentMaxLength {
		failures["body"] = "must be 1-500 characters"
	}
	return failures
}

// deleteCommentEndpoint lets a comment's author, the author of the post it is
// on, or a moderator remove it.
func (a *app) deleteCommentEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	ctx := r.Context()

	commentUUID, ok := parseUUIDPath(r, "uuid")
	if !ok {
		return http.StatusNotFound, errNotFound
	}
	comment, err := a.socialStore.GetComment(ctx, commentUUID, &viewer.UUID)
	if err != nil {
		return a.handleError(err)
	}
	if viewer.UUID != comment.Author.UUID && viewer.UUID != comment.PostAuthorUUID && !account.IsModerator(viewer.Role) {
		return http.StatusForbidden, ErrorResponse{Message: "you can't delete this comment", Type: errTypeForbidden}
	}
	if err := a.socialStore.SoftDeleteComment(ctx, commentUUID); err != nil {
		// Another request deleted it first: the end state is the same.
		if errors.Is(err, postgres.ErrCommentNotFound) {
			return http.StatusNoContent, nil
		}
		return a.handleError(err)
	}
	return http.StatusNoContent, nil
}
