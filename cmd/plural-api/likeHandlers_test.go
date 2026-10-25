package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/post"
	"github.com/plural-social/plural-api/internal/postgres"
)

// runLikeTests covers the behavior PUT and DELETE share: both resolve the post
// first, then call the matching store method.
func runLikeTests(t *testing.T, method string, stubStore func(*socialStoreMock, func(context.Context, uuid.UUID, uuid.UUID) error)) {
	path := "/posts/" + testPostUUID.String() + "/like"
	postFound := func(_ context.Context, _ uuid.UUID, viewer *uuid.UUID) (post.Post, error) {
		expect.Equal(t, viewer, &testAccountUUID)
		return testPost(), nil
	}

	type testCase struct {
		request        *http.Request
		postStore      *postStoreMock
		change         func(context.Context, uuid.UUID, uuid.UUID) error
		expectedStatus int
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.postStore.T = t
			store := &socialStoreMock{T: t}
			if testCase.change != nil {
				stubStore(store, testCase.change)
			}
			a := &app{
				accountStore: &accountStoreMock{T: t, GetViewerStub: viewerStub},
				postStore:    testCase.postStore,
				socialStore:  store,
				logger:       zap.NewNop(),
			}
			status, _ := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
		})
	}

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(method, path, nil),
		postStore:      &postStoreMock{},
		expectedStatus: http.StatusUnauthorized,
	})
	run("InvalidUUID", testCase{
		request:        authedRequest(method, "/posts/nope/like", ""),
		postStore:      &postStoreMock{},
		expectedStatus: http.StatusNotFound,
	})
	run("PostNotFound", testCase{
		request: authedRequest(method, path, ""),
		postStore: &postStoreMock{GetPostStub: func(context.Context, uuid.UUID, *uuid.UUID) (post.Post, error) {
			return post.Post{}, postgres.ErrPostNotFound
		}},
		expectedStatus: http.StatusNotFound,
	})
	run("StoreError", testCase{
		request:        authedRequest(method, path, ""),
		postStore:      &postStoreMock{GetPostStub: postFound},
		change:         func(context.Context, uuid.UUID, uuid.UUID) error { return expect.ErrTest },
		expectedStatus: http.StatusInternalServerError,
	})
	run("Success", testCase{
		request:   authedRequest(method, path, ""),
		postStore: &postStoreMock{GetPostStub: postFound},
		change: func(_ context.Context, postUUID, accountUUID uuid.UUID) error {
			expect.Equal(t, postUUID, testPostUUID)
			expect.Equal(t, accountUUID, testAccountUUID)
			return nil
		},
		expectedStatus: http.StatusNoContent,
	})
}

func Test_app_putLikeEndpoint(t *testing.T) {
	runLikeTests(t, http.MethodPut, func(m *socialStoreMock, f func(context.Context, uuid.UUID, uuid.UUID) error) { m.LikePostStub = f })
}

func Test_app_deleteLikeEndpoint(t *testing.T) {
	runLikeTests(t, http.MethodDelete, func(m *socialStoreMock, f func(context.Context, uuid.UUID, uuid.UUID) error) { m.UnlikePostStub = f })
}
