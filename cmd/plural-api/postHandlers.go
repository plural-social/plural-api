package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/post"
	"github.com/plural-social/plural-api/internal/postgres"
)

func (a *app) setUpPostRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /posts", a.endpoint(a.requireViewer(a.postPostEndpoint)))
	mux.HandleFunc("GET /posts/{uuid}", a.endpoint(a.optionalViewer(a.getPostEndpoint)))
	mux.HandleFunc("DELETE /posts/{uuid}", a.endpoint(a.requireViewer(a.deletePostEndpoint)))
}

// postResponse converts a stored post to JSON, building media URLs.
func (a *app) postResponse(r *http.Request, p post.Post) post.Response {
	return post.Response{
		UUID:         p.UUID,
		Author:       p.Author,
		Caption:      p.Caption,
		Media:        a.mediaResponse(r, p.Media),
		LikeCount:    p.LikeCount,
		CommentCount: p.CommentCount,
		LikedByMe:    p.LikedByMe,
		CreatedAt:    p.CreatedAt,
	}
}

// postPage converts up to limit+1 stored rows (as returned by list queries)
// to a page of JSON posts, with a cursor when more rows exist.
func (a *app) postPage(r *http.Request, rows []post.Post, limit int) pagination.Page[post.Response] {
	page := pagination.NewPage(rows, limit, func(p post.Post) pagination.Cursor {
		return pagination.Cursor{CreatedAt: p.CreatedAt, UUID: p.UUID}
	})
	items := make([]post.Response, len(page.Items))
	for i, p := range page.Items {
		items[i] = a.postResponse(r, p)
	}
	return pagination.Page[post.Response]{Items: items, NextCursor: page.NextCursor}
}

func (a *app) postPostEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	ctx := r.Context()

	request, err := decodeJSONBody[post.CreatePostRequest](r)
	if err != nil {
		return http.StatusBadRequest, ErrorResponse{Message: err.Error()}
	}
	request.Caption = strings.TrimSpace(request.Caption)

	mediaUUID, failures := validateCreatePostRequest(request)
	if len(failures) > 0 {
		return http.StatusBadRequest, ErrorResponse{
			Message: "invalid request body",
			Type:    account.ErrTypeValidation,
			Details: failures,
		}
	}

	errMediaNotFound := ErrorResponse{Message: "media not found", Type: post.ErrTypeMediaNotFound}
	m, err := a.mediaStore.GetMedia(ctx, mediaUUID)
	if errors.Is(err, postgres.ErrMediaNotFound) {
		return http.StatusUnprocessableEntity, errMediaNotFound
	}
	if err != nil {
		return a.handleError(err)
	}
	// Someone else's media looks the same as missing media.
	if m.AccountUUID != viewer.UUID {
		return http.StatusUnprocessableEntity, errMediaNotFound
	}
	// Processing media is allowed: videos are attached before transcoding ends.
	if m.Status == media.StatusFailed {
		return http.StatusUnprocessableEntity, ErrorResponse{
			Message: "media processing failed, upload it again",
			Type:    post.ErrTypeMediaFailed,
		}
	}

	postUUID, err := a.postStore.InsertPost(ctx, post.NewPost{
		AccountUUID: viewer.UUID,
		MediaUUID:   mediaUUID,
		Caption:     request.Caption,
	})
	if err != nil {
		return a.handleError(err)
	}
	created, err := a.postStore.GetPost(ctx, postUUID, &viewer.UUID)
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusCreated, a.postResponse(r, created)
}

// validateCreatePostRequest returns the parsed media UUID and any field failures.
func validateCreatePostRequest(request post.CreatePostRequest) (uuid.UUID, map[string]any) {
	failures := map[string]any{}
	mediaUUID, err := uuid.Parse(request.MediaUUID)
	if err != nil {
		failures["mediaUuid"] = "must be a valid UUID"
	}
	if utf8.RuneCountInString(request.Caption) > post.CaptionMaxLength {
		failures["caption"] = "must be at most 2200 characters"
	}
	return mediaUUID, failures
}

func (a *app) getPostEndpoint(r *http.Request, viewer *account.Viewer) (int, any) {
	postUUID, ok := parseUUIDPath(r, "uuid")
	if !ok {
		return http.StatusNotFound, errNotFound
	}
	p, err := a.postStore.GetPost(r.Context(), postUUID, viewerUUID(viewer))
	if err != nil {
		return a.handleError(err)
	}
	return http.StatusOK, a.postResponse(r, p)
}

func (a *app) deletePostEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	ctx := r.Context()

	postUUID, ok := parseUUIDPath(r, "uuid")
	if !ok {
		return http.StatusNotFound, errNotFound
	}
	// A post by another account is "not found" here, so callers cannot probe for it.
	m, err := a.postStore.SoftDeletePost(ctx, postUUID, viewer.UUID)
	if err != nil {
		return a.handleError(err)
	}
	a.deleteMediaFiles(ctx, m)
	return http.StatusNoContent, nil
}

// deleteMediaFiles removes a media item's stored files. Failures are logged and
// skipped: the post is already hidden, and leftovers are orphans, not leaks of
// live content.
func (a *app) deleteMediaFiles(ctx context.Context, m media.Media) {
	for _, key := range []string{m.DisplayKey, m.ThumbnailKey, m.OriginalKey} {
		if key == "" {
			continue
		}
		if err := a.blobStore.Delete(ctx, key); err != nil {
			a.logger.Warn("Error deleting media file", zap.String("key", key), zap.Error(err))
		}
	}
}
