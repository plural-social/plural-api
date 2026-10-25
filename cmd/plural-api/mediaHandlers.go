package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/blob"
	"github.com/plural-social/plural-api/internal/media"
)

func (a *app) setUpMediaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /media", a.endpoint(a.rateLimited(a.uploadLimiter, a.requireViewer(a.postMediaEndpoint))))
	mux.HandleFunc("GET /media/{uuid}", a.endpoint(a.requireViewer(a.getMediaEndpoint)))
	mux.HandleFunc("GET /files/{key...}", a.serveFile)
}

// postMediaEndpoint takes raw image bytes (not JSON), re-encodes them and
// stores a display size and a thumbnail.
func (a *app) postMediaEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	ctx := r.Context()

	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if !slices.Contains(media.ImageContentTypes, contentType) {
		return http.StatusUnsupportedMediaType, ErrorResponse{
			Message: "unsupported media type, use JPEG, PNG or WebP",
			Type:    media.ErrTypeUnsupportedMediaType,
		}
	}

	errTooLarge := ErrorResponse{Message: "the file is too large", Type: media.ErrTypeMediaTooLarge}
	if r.ContentLength > a.maxImageBytes {
		return http.StatusRequestEntityTooLarge, errTooLarge
	}
	data, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, a.maxImageBytes))
	if errors.As(err, new(*http.MaxBytesError)) {
		return http.StatusRequestEntityTooLarge, errTooLarge
	}
	if err != nil {
		return http.StatusBadRequest, ErrorResponse{Message: "reading request body"}
	}

	processed, err := media.ProcessImage(data)
	if errors.Is(err, media.ErrInvalidImage) {
		return http.StatusBadRequest, ErrorResponse{
			Message: "the file is not a valid image",
			Type:    media.ErrTypeInvalidMedia,
		}
	}
	if err != nil {
		return a.handleError(err)
	}

	mediaUUID := a.newUUID()
	displayKey, thumbnailKey := media.DisplayKey(mediaUUID), media.ThumbnailKey(mediaUUID)
	if err := a.putJPEG(ctx, displayKey, processed.Display); err != nil {
		return a.handleError(err)
	}
	if err := a.putJPEG(ctx, thumbnailKey, processed.Thumbnail); err != nil {
		a.deleteBlobs(ctx, displayKey)
		return a.handleError(err)
	}

	inserted, err := a.mediaStore.InsertMedia(ctx, media.NewMedia{
		UUID:         mediaUUID,
		AccountUUID:  viewer.UUID,
		Kind:         media.KindImage,
		Status:       media.StatusReady,
		ContentType:  "image/jpeg",
		DisplayKey:   displayKey,
		ThumbnailKey: thumbnailKey,
		Width:        processed.Width,
		Height:       processed.Height,
		SizeBytes:    int64(len(processed.Display)),
	})
	if err != nil {
		a.deleteBlobs(ctx, displayKey, thumbnailKey)
		return a.handleError(err)
	}

	return http.StatusCreated, a.mediaResponse(r, inserted)
}

func (a *app) putJPEG(ctx context.Context, key string, data []byte) error {
	return a.blobStore.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "image/jpeg")
}

// deleteBlobs removes files written for a failed upload. Failures only leave
// orphaned files behind, so they are logged and not returned.
func (a *app) deleteBlobs(ctx context.Context, keys ...string) {
	for _, key := range keys {
		if err := a.blobStore.Delete(ctx, key); err != nil {
			a.logger.Warn("Error deleting orphaned file", zap.String("key", key), zap.Error(err))
		}
	}
}

// getMediaEndpoint only shows media to its owner: unattached media is private.
func (a *app) getMediaEndpoint(r *http.Request, viewer account.Viewer) (int, any) {
	mediaUUID, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		return http.StatusNotFound, errNotFound
	}
	m, err := a.mediaStore.GetMedia(r.Context(), mediaUUID)
	if err != nil {
		return a.handleError(err)
	}
	if m.AccountUUID != viewer.UUID {
		return http.StatusNotFound, errNotFound
	}
	return http.StatusOK, a.mediaResponse(r, m)
}

// serveFile streams a stored file. http.ServeContent handles Range and
// conditional requests, which video players rely on.
func (a *app) serveFile(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !isPublicFileKey(key) {
		http.NotFound(w, r)
		return
	}

	object, err := a.blobStore.Get(r.Context(), key)
	if errors.Is(err, blob.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.logger.Error("Error reading file", zap.String("key", key), zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer object.Close()

	header := w.Header()
	header.Set("Content-Type", object.ContentType)
	// Keys never change content, so clients and CDNs may cache them forever.
	header.Set("Cache-Control", "public, max-age=31536000, immutable")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("ETag", `"`+strings.Trim(object.ETag, `"`)+`"`)
	http.ServeContent(w, r, "", object.LastModified, object)
}

// isPublicFileKey allows only processed media files. Uploaded originals stay
// private: they may still carry metadata such as GPS.
func isPublicFileKey(key string) bool {
	return strings.HasPrefix(key, "media/") &&
		!strings.Contains(key, "..") &&
		path.Clean(key) == key &&
		!strings.HasSuffix(key, "/original")
}
