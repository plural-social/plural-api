package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/post"
	"github.com/plural-social/plural-api/internal/postgres"
)

func Test_app_postPostEndpoint(t *testing.T) {
	validBody := `{"mediaUuid":"` + testMediaUUID.String() + `","caption":"  first post "}`
	ownedMedia := func(status string) *mediaStoreMock {
		return &mediaStoreMock{GetMediaStub: func(context.Context, uuid.UUID) (media.Media, error) {
			return media.Media{UUID: testMediaUUID, AccountUUID: testAccountUUID, Status: status}, nil
		}}
	}
	validationError := func(field, message string) map[string]any {
		return map[string]any{
			"error": "invalid request body", "type": "validation",
			"details": map[string]any{field: message},
		}
	}
	internalError := map[string]any{"error": "internal server error"}
	insertOK := func(_ context.Context, newPost post.NewPost) (uuid.UUID, error) {
		expect.Equal(t, newPost, post.NewPost{AccountUUID: testAccountUUID, MediaUUID: testMediaUUID, Caption: "first post"})
		return testPostUUID, nil
	}

	type testCase struct {
		request        *http.Request
		mediaStore     *mediaStoreMock
		postStore      *postStoreMock
		expectedStatus int
		expectedBody   map[string]any
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.mediaStore.T = t
			testCase.postStore.T = t
			a := &app{
				accountStore: &accountStoreMock{T: t, GetViewerStub: viewerStub},
				mediaStore:   testCase.mediaStore,
				postStore:    testCase.postStore,
				logger:       zap.NewNop(),
			}
			status, body := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
			expect.Equal(t, body, testCase.expectedBody)
		})
	}

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodPost, "/posts", strings.NewReader(validBody)),
		mediaStore:     &mediaStoreMock{},
		postStore:      &postStoreMock{},
		expectedStatus: http.StatusUnauthorized,
		expectedBody:   map[string]any{"error": "authentication required", "type": "unauthenticated"},
	})
	run("MalformedJSON", testCase{
		request:        authedRequest(http.MethodPost, "/posts", `{"mediaUuid":`),
		mediaStore:     &mediaStoreMock{},
		postStore:      &postStoreMock{},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   map[string]any{"error": "decoding request body: unexpected EOF"},
	})
	run("InvalidMediaUUID", testCase{
		request:        authedRequest(http.MethodPost, "/posts", `{"mediaUuid":"nope"}`),
		mediaStore:     &mediaStoreMock{},
		postStore:      &postStoreMock{},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   validationError("mediaUuid", "must be a valid UUID"),
	})
	run("CaptionTooLong", testCase{
		request:        authedRequest(http.MethodPost, "/posts", `{"mediaUuid":"`+testMediaUUID.String()+`","caption":"`+strings.Repeat("é", 2201)+`"}`),
		mediaStore:     &mediaStoreMock{},
		postStore:      &postStoreMock{},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   validationError("caption", "must be at most 2200 characters"),
	})
	mediaNotFound := map[string]any{"error": "media not found", "type": post.ErrTypeMediaNotFound}
	run("MediaNotFound", testCase{
		request: authedRequest(http.MethodPost, "/posts", validBody),
		mediaStore: &mediaStoreMock{GetMediaStub: func(context.Context, uuid.UUID) (media.Media, error) {
			return media.Media{}, postgres.ErrMediaNotFound
		}},
		postStore:      &postStoreMock{},
		expectedStatus: http.StatusUnprocessableEntity,
		expectedBody:   mediaNotFound,
	})
	run("MediaOwnedByOther", testCase{
		request: authedRequest(http.MethodPost, "/posts", validBody),
		mediaStore: &mediaStoreMock{GetMediaStub: func(context.Context, uuid.UUID) (media.Media, error) {
			return media.Media{AccountUUID: uuid.New(), Status: media.StatusReady}, nil
		}},
		postStore:      &postStoreMock{},
		expectedStatus: http.StatusUnprocessableEntity,
		expectedBody:   mediaNotFound,
	})
	run("MediaFailed", testCase{
		request:        authedRequest(http.MethodPost, "/posts", validBody),
		mediaStore:     ownedMedia(media.StatusFailed),
		postStore:      &postStoreMock{},
		expectedStatus: http.StatusUnprocessableEntity,
		expectedBody:   map[string]any{"error": "media processing failed, upload it again", "type": post.ErrTypeMediaFailed},
	})
	run("GetMediaError", testCase{
		request: authedRequest(http.MethodPost, "/posts", validBody),
		mediaStore: &mediaStoreMock{GetMediaStub: func(context.Context, uuid.UUID) (media.Media, error) {
			return media.Media{}, expect.ErrTest
		}},
		postStore:      &postStoreMock{},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   internalError,
	})
	run("MediaAlreadyUsed", testCase{
		request:    authedRequest(http.MethodPost, "/posts", validBody),
		mediaStore: ownedMedia(media.StatusReady),
		postStore: &postStoreMock{InsertPostStub: func(context.Context, post.NewPost) (uuid.UUID, error) {
			return uuid.Nil, postgres.ErrMediaAlreadyUsed
		}},
		expectedStatus: http.StatusConflict,
		expectedBody:   map[string]any{"error": "media already used by another post", "type": post.ErrTypeMediaAlreadyUsed},
	})
	run("InsertError", testCase{
		request:    authedRequest(http.MethodPost, "/posts", validBody),
		mediaStore: ownedMedia(media.StatusReady),
		postStore: &postStoreMock{InsertPostStub: func(context.Context, post.NewPost) (uuid.UUID, error) {
			return uuid.Nil, expect.ErrTest
		}},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   internalError,
	})
	run("GetPostError", testCase{
		request:    authedRequest(http.MethodPost, "/posts", validBody),
		mediaStore: ownedMedia(media.StatusReady),
		postStore: &postStoreMock{
			InsertPostStub: insertOK,
			GetPostStub: func(context.Context, uuid.UUID, *uuid.UUID) (post.Post, error) {
				return post.Post{}, expect.ErrTest
			},
		},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   internalError,
	})

	processing := testPostJSON()
	processing["media"] = map[string]any{
		"uuid": testMediaUUID.String(), "kind": "video", "status": "processing",
		"width": 0.0, "height": 0.0, "durationMs": 0.0,
	}
	run("ProcessingMediaAllowed", testCase{
		request:    authedRequest(http.MethodPost, "/posts", validBody),
		mediaStore: ownedMedia(media.StatusProcessing),
		postStore: &postStoreMock{
			InsertPostStub: insertOK,
			GetPostStub: func(context.Context, uuid.UUID, *uuid.UUID) (post.Post, error) {
				p := testPost()
				p.Media = media.Media{UUID: testMediaUUID, Kind: media.KindVideo, Status: media.StatusProcessing}
				return p, nil
			},
		},
		expectedStatus: http.StatusCreated,
		expectedBody:   processing,
	})
	run("Success", testCase{
		request:    authedRequest(http.MethodPost, "/posts", validBody),
		mediaStore: ownedMedia(media.StatusReady),
		postStore: &postStoreMock{
			InsertPostStub: insertOK,
			GetPostStub: func(_ context.Context, postUUID uuid.UUID, viewer *uuid.UUID) (post.Post, error) {
				expect.Equal(t, postUUID, testPostUUID)
				expect.Equal(t, viewer, &testAccountUUID)
				p := testPost()
				p.Author.AvatarKey = testAvatarKey
				return p, nil
			},
		},
		expectedStatus: http.StatusCreated,
		expectedBody:   withAuthorAvatar(testPostJSON(), "http://example.com/files/"+testAvatarKey),
	})
}

func Test_app_getPostEndpoint(t *testing.T) {
	notFound := map[string]any{"error": "not found", "type": errTypeNotFound}

	type testCase struct {
		request        *http.Request
		postStore      *postStoreMock
		expectedStatus int
		expectedBody   map[string]any
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.postStore.T = t
			a := &app{
				accountStore: &accountStoreMock{T: t, GetViewerStub: viewerStub},
				postStore:    testCase.postStore,
				logger:       zap.NewNop(),
			}
			status, body := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
			expect.Equal(t, body, testCase.expectedBody)
		})
	}

	path := "/posts/" + testPostUUID.String()
	run("InvalidUUID", testCase{
		request:        httptest.NewRequest(http.MethodGet, "/posts/nope", nil),
		postStore:      &postStoreMock{},
		expectedStatus: http.StatusNotFound,
		expectedBody:   notFound,
	})
	run("NotFound", testCase{
		request: httptest.NewRequest(http.MethodGet, path, nil),
		postStore: &postStoreMock{GetPostStub: func(context.Context, uuid.UUID, *uuid.UUID) (post.Post, error) {
			return post.Post{}, postgres.ErrPostNotFound
		}},
		expectedStatus: http.StatusNotFound,
		expectedBody:   notFound,
	})
	run("Anonymous", testCase{
		request: httptest.NewRequest(http.MethodGet, path, nil),
		postStore: &postStoreMock{GetPostStub: func(_ context.Context, _ uuid.UUID, viewer *uuid.UUID) (post.Post, error) {
			expect.Equal(t, viewer, nil)
			return testPost(), nil
		}},
		expectedStatus: http.StatusOK,
		expectedBody:   testPostJSON(),
	})
	run("Viewer", testCase{
		request: authedRequest(http.MethodGet, path, ""),
		postStore: &postStoreMock{GetPostStub: func(_ context.Context, _ uuid.UUID, viewer *uuid.UUID) (post.Post, error) {
			expect.Equal(t, viewer, &testAccountUUID)
			return testPost(), nil
		}},
		expectedStatus: http.StatusOK,
		expectedBody:   testPostJSON(),
	})
}

func Test_app_deletePostEndpoint(t *testing.T) {
	path := "/posts/" + testPostUUID.String()
	deletable := testPost().Media

	type testCase struct {
		request        *http.Request
		postStore      *postStoreMock
		blobStore      *blobStoreMock
		expectedStatus int
		expectedDelete []string
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.postStore.T = t
			testCase.blobStore.T = t
			var deleted []string
			deleteStub := testCase.blobStore.DeleteStub
			testCase.blobStore.DeleteStub = func(ctx context.Context, key string) error {
				deleted = append(deleted, key)
				if deleteStub != nil {
					return deleteStub(ctx, key)
				}
				return nil
			}
			a := &app{
				accountStore: &accountStoreMock{T: t, GetViewerStub: viewerStub},
				postStore:    testCase.postStore,
				blobStore:    testCase.blobStore,
				logger:       zap.NewNop(),
			}
			status, _ := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
			expect.Equal(t, deleted, testCase.expectedDelete)
		})
	}

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodDelete, path, nil),
		postStore:      &postStoreMock{},
		blobStore:      &blobStoreMock{},
		expectedStatus: http.StatusUnauthorized,
	})
	run("InvalidUUID", testCase{
		request:        authedRequest(http.MethodDelete, "/posts/nope", ""),
		postStore:      &postStoreMock{},
		blobStore:      &blobStoreMock{},
		expectedStatus: http.StatusNotFound,
	})
	run("NotFound", testCase{
		request: authedRequest(http.MethodDelete, path, ""),
		postStore: &postStoreMock{SoftDeletePostStub: func(context.Context, uuid.UUID, uuid.UUID) (media.Media, error) {
			return media.Media{}, postgres.ErrPostNotFound
		}},
		blobStore:      &blobStoreMock{},
		expectedStatus: http.StatusNotFound,
	})
	run("StoreError", testCase{
		request: authedRequest(http.MethodDelete, path, ""),
		postStore: &postStoreMock{SoftDeletePostStub: func(context.Context, uuid.UUID, uuid.UUID) (media.Media, error) {
			return media.Media{}, expect.ErrTest
		}},
		blobStore:      &blobStoreMock{},
		expectedStatus: http.StatusInternalServerError,
	})
	softDelete := func(_ context.Context, postUUID, accountUUID uuid.UUID) (media.Media, error) {
		expect.Equal(t, postUUID, testPostUUID)
		expect.Equal(t, accountUUID, testAccountUUID)
		return deletable, nil
	}
	run("BlobDeleteError", testCase{
		request:        authedRequest(http.MethodDelete, path, ""),
		postStore:      &postStoreMock{SoftDeletePostStub: softDelete},
		blobStore:      &blobStoreMock{DeleteStub: func(context.Context, string) error { return expect.ErrTest }},
		expectedStatus: http.StatusNoContent,
		expectedDelete: []string{deletable.DisplayKey, deletable.ThumbnailKey},
	})
	run("Success", testCase{
		request:        authedRequest(http.MethodDelete, path, ""),
		postStore:      &postStoreMock{SoftDeletePostStub: softDelete},
		blobStore:      &blobStoreMock{},
		expectedStatus: http.StatusNoContent,
		expectedDelete: []string{deletable.DisplayKey, deletable.ThumbnailKey},
	})
}

// withAuthorAvatar sets avatarUrl on the author of a decoded post body.
func withAuthorAvatar(body map[string]any, url string) map[string]any {
	author, _ := body["author"].(map[string]any)
	author["avatarUrl"] = url
	return body
}
