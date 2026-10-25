package main

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/blob"
	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/postgres"
)

var testMediaUUID = uuid.MustParse("5f1d3c0e-8a52-4d7e-9c1b-2a6f0b7e4d11")

// testPNG returns a small valid PNG upload body.
func testPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	expect.ErrorNil(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 40, 20))))
	return buf.Bytes()
}

func uploadRequest(contentType string, body []byte) *http.Request {
	request := authedRequest(http.MethodPost, "/media", string(body))
	request.Header.Set("Content-Type", contentType)
	return request
}

func Test_app_postMediaEndpoint(t *testing.T) {
	var (
		validImage = testPNG(t)
		errTooBig  = map[string]any{"error": "the file is too large", "type": media.ErrTypeMediaTooLarge}
	)

	type testCase struct {
		request        *http.Request
		maxImageBytes  int64
		mediaStore     *mediaStoreMock
		blobStore      *blobStoreMock
		expectedStatus int
		expectedBody   map[string]any
		expectedDelete []string
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.mediaStore.T = t
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
				accountStore:  &accountStoreMock{T: t, GetViewerStub: viewerStub},
				mediaStore:    testCase.mediaStore,
				blobStore:     testCase.blobStore,
				logger:        zap.NewNop(),
				maxImageBytes: testCase.maxImageBytes,
				newUUID:       func() uuid.UUID { return testMediaUUID },
			}
			if a.maxImageBytes == 0 {
				a.maxImageBytes = media.ImageMaxBytes
			}

			status, body := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
			expect.Equal(t, body, testCase.expectedBody)
			expect.Equal(t, deleted, testCase.expectedDelete)
		})
	}

	displayKey, thumbKey := media.DisplayKey(testMediaUUID), media.ThumbnailKey(testMediaUUID)
	putOK := func(context.Context, string, io.Reader, int64, string) error { return nil }
	internalError := map[string]any{"error": "internal server error"}

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodPost, "/media", bytes.NewReader(validImage)),
		mediaStore:     &mediaStoreMock{},
		blobStore:      &blobStoreMock{},
		expectedStatus: http.StatusUnauthorized,
		expectedBody:   map[string]any{"error": "authentication required", "type": "unauthenticated"},
	})
	run("UnsupportedType", testCase{
		request:        uploadRequest("video/mp4", validImage),
		mediaStore:     &mediaStoreMock{},
		blobStore:      &blobStoreMock{},
		expectedStatus: http.StatusUnsupportedMediaType,
		expectedBody: map[string]any{
			"error": "unsupported media type, use JPEG, PNG or WebP",
			"type":  media.ErrTypeUnsupportedMediaType,
		},
	})
	run("ContentLengthTooLarge", testCase{
		request:        uploadRequest("image/png", validImage),
		maxImageBytes:  10,
		mediaStore:     &mediaStoreMock{},
		blobStore:      &blobStoreMock{},
		expectedStatus: http.StatusRequestEntityTooLarge,
		expectedBody:   errTooBig,
	})
	t.Run("BodyTooLarge", func(t *testing.T) {
		// A streamed body has no Content-Length, so only the reader enforces the limit.
		request := uploadRequest("image/png", nil)
		request.Body = io.NopCloser(bytes.NewReader(validImage))
		request.ContentLength = -1
		a := &app{
			accountStore:  &accountStoreMock{T: t, GetViewerStub: viewerStub},
			logger:        zap.NewNop(),
			maxImageBytes: 10,
		}
		status, body := serveJSON(t, a, request)
		expect.Equal(t, status, http.StatusRequestEntityTooLarge)
		expect.Equal(t, body, errTooBig)
	})
	run("InvalidImage", testCase{
		request:        uploadRequest("image/png", []byte("not an image")),
		mediaStore:     &mediaStoreMock{},
		blobStore:      &blobStoreMock{},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   map[string]any{"error": "the file is not a valid image", "type": media.ErrTypeInvalidMedia},
	})
	run("PutDisplayError", testCase{
		request:        uploadRequest("image/png", validImage),
		mediaStore:     &mediaStoreMock{},
		blobStore:      &blobStoreMock{PutStub: func(context.Context, string, io.Reader, int64, string) error { return expect.ErrTest }},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   internalError,
	})
	run("PutThumbnailError", testCase{
		request:    uploadRequest("image/png", validImage),
		mediaStore: &mediaStoreMock{},
		blobStore: &blobStoreMock{PutStub: func(_ context.Context, key string, _ io.Reader, _ int64, _ string) error {
			if key == thumbKey {
				return expect.ErrTest
			}
			return nil
		}},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   internalError,
		expectedDelete: []string{displayKey},
	})
	run("InsertError", testCase{
		request: uploadRequest("image/png", validImage),
		mediaStore: &mediaStoreMock{InsertMediaStub: func(context.Context, media.NewMedia) (media.Media, error) {
			return media.Media{}, expect.ErrTest
		}},
		blobStore:      &blobStoreMock{PutStub: putOK},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   internalError,
		expectedDelete: []string{displayKey, thumbKey},
	})
	run("Success", testCase{
		request: uploadRequest("image/png; charset=binary", validImage),
		mediaStore: &mediaStoreMock{InsertMediaStub: func(_ context.Context, newMedia media.NewMedia) (media.Media, error) {
			expect.Equal(t, newMedia.UUID, testMediaUUID)
			expect.Equal(t, newMedia.AccountUUID, testAccountUUID)
			expect.Equal(t, newMedia.Kind, media.KindImage)
			expect.Equal(t, newMedia.Status, media.StatusReady)
			expect.Equal(t, newMedia.DisplayKey, displayKey)
			expect.Equal(t, newMedia.ThumbnailKey, thumbKey)
			expect.Equal(t, newMedia.Width, 40)
			expect.Equal(t, newMedia.Height, 20)
			return media.Media{
				UUID: newMedia.UUID, Kind: newMedia.Kind, Status: newMedia.Status,
				DisplayKey: newMedia.DisplayKey, ThumbnailKey: newMedia.ThumbnailKey,
				Width: newMedia.Width, Height: newMedia.Height,
			}, nil
		}},
		blobStore:      &blobStoreMock{PutStub: putOK},
		expectedStatus: http.StatusCreated,
		expectedBody: map[string]any{
			"uuid":         testMediaUUID.String(),
			"kind":         "image",
			"status":       "ready",
			"url":          "http://example.com/files/" + displayKey,
			"thumbnailUrl": "http://example.com/files/" + thumbKey,
			"width":        40.0,
			"height":       20.0,
			"durationMs":   0.0,
		},
	})
}

func Test_app_getMediaEndpoint(t *testing.T) {
	ready := media.Media{
		UUID: testMediaUUID, AccountUUID: testAccountUUID, Kind: media.KindImage, Status: media.StatusReady,
		DisplayKey: media.DisplayKey(testMediaUUID), ThumbnailKey: media.ThumbnailKey(testMediaUUID), Width: 40, Height: 20,
	}
	notFound := map[string]any{"error": "not found", "type": errTypeNotFound}

	type testCase struct {
		path           string
		mediaStore     *mediaStoreMock
		expectedStatus int
		expectedBody   map[string]any
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.mediaStore.T = t
			a := &app{
				accountStore: &accountStoreMock{T: t, GetViewerStub: viewerStub},
				mediaStore:   testCase.mediaStore,
				logger:       zap.NewNop(),
			}
			status, body := serveJSON(t, a, authedRequest(http.MethodGet, testCase.path, ""))
			expect.Equal(t, status, testCase.expectedStatus)
			expect.Equal(t, body, testCase.expectedBody)
		})
	}

	path := "/media/" + testMediaUUID.String()
	run("InvalidUUID", testCase{
		path:           "/media/nope",
		mediaStore:     &mediaStoreMock{},
		expectedStatus: http.StatusNotFound,
		expectedBody:   notFound,
	})
	run("NotFound", testCase{
		path: path,
		mediaStore: &mediaStoreMock{GetMediaStub: func(context.Context, uuid.UUID) (media.Media, error) {
			return media.Media{}, postgres.ErrMediaNotFound
		}},
		expectedStatus: http.StatusNotFound,
		expectedBody:   notFound,
	})
	run("OtherOwner", testCase{
		path: path,
		mediaStore: &mediaStoreMock{GetMediaStub: func(context.Context, uuid.UUID) (media.Media, error) {
			other := ready
			other.AccountUUID = uuid.New()
			return other, nil
		}},
		expectedStatus: http.StatusNotFound,
		expectedBody:   notFound,
	})
	run("Success", testCase{
		path: path,
		mediaStore: &mediaStoreMock{GetMediaStub: func(context.Context, uuid.UUID) (media.Media, error) {
			return ready, nil
		}},
		expectedStatus: http.StatusOK,
		expectedBody: map[string]any{
			"uuid":         testMediaUUID.String(),
			"kind":         "image",
			"status":       "ready",
			"url":          "http://example.com/files/" + ready.DisplayKey,
			"thumbnailUrl": "http://example.com/files/" + ready.ThumbnailKey,
			"width":        40.0,
			"height":       20.0,
			"durationMs":   0.0,
		},
	})
}

// nopSeekCloser adapts a bytes.Reader to the blob.Object reader interface.
type nopSeekCloser struct{ *bytes.Reader }

func (nopSeekCloser) Close() error { return nil }

func Test_app_serveFile(t *testing.T) {
	const key = "media/5f1d3c0e-8a52-4d7e-9c1b-2a6f0b7e4d11/display.jpg"
	content := []byte("0123456789")
	object := func(context.Context, string) (*blob.Object, error) {
		return &blob.Object{
			ReadSeekCloser: nopSeekCloser{bytes.NewReader(content)},
			Size:           int64(len(content)),
			ContentType:    "image/jpeg",
			ETag:           "abc",
		}, nil
	}

	type testCase struct {
		path           string
		rangeHeader    string
		blobStore      *blobStoreMock
		expectedStatus int
		expectedBody   string
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.blobStore.T = t
			a := &app{blobStore: testCase.blobStore, logger: zap.NewNop()}
			request := httptest.NewRequest(http.MethodGet, testCase.path, nil)
			if testCase.rangeHeader != "" {
				request.Header.Set("Range", testCase.rangeHeader)
			}
			recorder := httptest.NewRecorder()
			a.routes().ServeHTTP(recorder, request)

			expect.Equal(t, recorder.Code, testCase.expectedStatus)
			if testCase.expectedBody != "" {
				expect.Equal(t, recorder.Body.String(), testCase.expectedBody)
				header := recorder.Header()
				expect.Equal(t, header.Get("Content-Type"), "image/jpeg")
				expect.Equal(t, header.Get("Cache-Control"), "public, max-age=31536000, immutable")
				expect.Equal(t, header.Get("X-Content-Type-Options"), "nosniff")
				expect.Equal(t, header.Get("ETag"), `"abc"`)
			}
		})
	}

	run("NotMediaPrefix", testCase{path: "/files/secret/display.jpg", blobStore: &blobStoreMock{}, expectedStatus: http.StatusNotFound})
	// ServeMux redirects to the cleaned path before the handler runs.
	run("DotDotRedirected", testCase{path: "/files/media/a/../../b.jpg", blobStore: &blobStoreMock{}, expectedStatus: http.StatusTemporaryRedirect})
	run("Original", testCase{path: "/files/media/abc/original", blobStore: &blobStoreMock{}, expectedStatus: http.StatusNotFound})
	run("BlobNotFound", testCase{
		path: "/files/" + key,
		blobStore: &blobStoreMock{GetStub: func(context.Context, string) (*blob.Object, error) {
			return nil, blob.ErrNotFound
		}},
		expectedStatus: http.StatusNotFound,
	})
	run("BlobError", testCase{
		path: "/files/" + key,
		blobStore: &blobStoreMock{GetStub: func(context.Context, string) (*blob.Object, error) {
			return nil, expect.ErrTest
		}},
		expectedStatus: http.StatusInternalServerError,
	})
	run("Success", testCase{
		path:           "/files/" + key,
		blobStore:      &blobStoreMock{GetStub: object},
		expectedStatus: http.StatusOK,
		expectedBody:   string(content),
	})
	run("Range", testCase{
		path:           "/files/" + key,
		rangeHeader:    "bytes=0-3",
		blobStore:      &blobStoreMock{GetStub: object},
		expectedStatus: http.StatusPartialContent,
		expectedBody:   "0123",
	})
}

// isPublicFileKey is the second line of defense if a key reaches the handler uncleaned.
func Test_isPublicFileKey(t *testing.T) {
	for key, expected := range map[string]bool{
		"media/abc/display.jpg": true,
		"media/abc/video.mp4":   true,
		"secret/abc":            false,
		"media/a/../b.jpg":      false,
		"media//abc.jpg":        false,
		"media/abc/original":    false,
	} {
		t.Run(key, func(t *testing.T) { expect.Equal(t, isPublicFileKey(key), expected) })
	}
}
