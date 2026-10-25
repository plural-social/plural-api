package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/post"
)

func Test_app_getFollowingFeedEndpoint(t *testing.T) {
	type testCase struct {
		request        *http.Request
		socialStore    *socialStoreMock
		expectedStatus int
		check          func(t *testing.T, body map[string]any)
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.socialStore.T = t
			a := &app{
				accountStore: &accountStoreMock{T: t, GetViewerStub: viewerStub},
				socialStore:  testCase.socialStore,
				logger:       zap.NewNop(),
			}
			status, body := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
			if testCase.check != nil {
				testCase.check(t, body)
			}
		})
	}

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodGet, "/feed/following", nil),
		socialStore:    &socialStoreMock{},
		expectedStatus: http.StatusUnauthorized,
	})
	run("InvalidCursor", testCase{
		request:        authedRequest(http.MethodGet, "/feed/following?cursor=%21%21", ""),
		socialStore:    &socialStoreMock{},
		expectedStatus: http.StatusBadRequest,
	})
	run("StoreError", testCase{
		request: authedRequest(http.MethodGet, "/feed/following", ""),
		socialStore: &socialStoreMock{ListFollowingFeedStub: func(context.Context, uuid.UUID, pagination.Cursor, int) ([]post.Post, error) {
			return nil, expect.ErrTest
		}},
		expectedStatus: http.StatusInternalServerError,
	})
	run("Success", testCase{
		request: authedRequest(http.MethodGet, "/feed/following?limit=1", ""),
		socialStore: &socialStoreMock{ListFollowingFeedStub: func(_ context.Context, viewer uuid.UUID, cursor pagination.Cursor, limit int) ([]post.Post, error) {
			expect.Equal(t, viewer, testAccountUUID)
			expect.Equal(t, cursor.IsZero(), true)
			expect.Equal(t, limit, 1)
			return []post.Post{testPost(), testPost()}, nil
		}},
		expectedStatus: http.StatusOK,
		check: func(t *testing.T, body map[string]any) {
			items, _ := body["items"].([]any)
			expect.Equal(t, len(items), 1)
			expect.Equal[any](t, body["nextCursor"], pagination.Encode(pagination.Cursor{CreatedAt: testCreatedAt, UUID: testPostUUID}))
		},
	})
}

func Test_app_getExploreFeedEndpoint(t *testing.T) {
	stub := func(expectedViewer *uuid.UUID) func(context.Context, *uuid.UUID, pagination.Cursor, int) ([]post.Post, error) {
		return func(_ context.Context, viewer *uuid.UUID, _ pagination.Cursor, _ int) ([]post.Post, error) {
			expect.Equal(t, viewer, expectedViewer)
			return []post.Post{testPost()}, nil
		}
	}

	type testCase struct {
		request        *http.Request
		socialStore    *socialStoreMock
		expectedStatus int
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.socialStore.T = t
			a := &app{
				accountStore: &accountStoreMock{T: t, GetViewerStub: viewerStub},
				socialStore:  testCase.socialStore,
				logger:       zap.NewNop(),
			}
			status, _ := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
		})
	}

	run("InvalidLimit", testCase{
		request:        httptest.NewRequest(http.MethodGet, "/feed/explore?limit=0", nil),
		socialStore:    &socialStoreMock{},
		expectedStatus: http.StatusBadRequest,
	})
	run("StoreError", testCase{
		request: httptest.NewRequest(http.MethodGet, "/feed/explore", nil),
		socialStore: &socialStoreMock{ListExploreFeedStub: func(context.Context, *uuid.UUID, pagination.Cursor, int) ([]post.Post, error) {
			return nil, expect.ErrTest
		}},
		expectedStatus: http.StatusInternalServerError,
	})
	run("Anonymous", testCase{
		request:        httptest.NewRequest(http.MethodGet, "/feed/explore", nil),
		socialStore:    &socialStoreMock{ListExploreFeedStub: stub(nil)},
		expectedStatus: http.StatusOK,
	})
	run("Viewer", testCase{
		request:        authedRequest(http.MethodGet, "/feed/explore", ""),
		socialStore:    &socialStoreMock{ListExploreFeedStub: stub(&testAccountUUID)},
		expectedStatus: http.StatusOK,
	})
}
