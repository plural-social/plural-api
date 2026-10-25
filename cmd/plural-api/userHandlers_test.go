package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/post"
	"github.com/plural-social/plural-api/internal/postgres"
)

func Test_app_getUserEndpoint(t *testing.T) {
	profile := account.Profile{
		UserSummary: account.UserSummary{UUID: testAccountUUID, Username: "alice", DisplayName: "Alice"},
		Bio:         "hello",
		CreatedAt:   testCreatedAt,
	}
	profileJSON := map[string]any{
		"uuid":           testAccountUUID.String(),
		"username":       "alice",
		"displayName":    "Alice",
		"bio":            "hello",
		"followerCount":  0.0,
		"followingCount": 0.0,
		"postCount":      0.0,
		"isFollowing":    false,
		"isBlocked":      false,
		"isSelf":         false,
		"createdAt":      "2026-10-01T08:00:00Z",
	}
	notFound := map[string]any{"error": "not found", "type": errTypeNotFound}

	type testCase struct {
		request        *http.Request
		accountStore   *accountStoreMock
		expectedStatus int
		expectedBody   map[string]any
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.accountStore.T = t
			a := &app{accountStore: testCase.accountStore, logger: zap.NewNop()}
			status, body := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
			expect.Equal(t, body, testCase.expectedBody)
		})
	}

	run("InvalidUsername", testCase{
		request:        httptest.NewRequest(http.MethodGet, "/users/Bad-Name", nil),
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusNotFound,
		expectedBody:   notFound,
	})
	run("NotFound", testCase{
		request: httptest.NewRequest(http.MethodGet, "/users/nobody", nil),
		accountStore: &accountStoreMock{
			GetProfileStub: func(context.Context, string, *uuid.UUID) (account.Profile, error) {
				return account.Profile{}, postgres.ErrAccountNotFound
			},
		},
		expectedStatus: http.StatusNotFound,
		expectedBody:   notFound,
	})
	run("StoreError", testCase{
		request: httptest.NewRequest(http.MethodGet, "/users/alice", nil),
		accountStore: &accountStoreMock{
			GetProfileStub: func(context.Context, string, *uuid.UUID) (account.Profile, error) {
				return account.Profile{}, expect.ErrTest
			},
		},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   map[string]any{"error": "internal server error"},
	})
	run("Anonymous", testCase{
		request: httptest.NewRequest(http.MethodGet, "/users/alice", nil),
		accountStore: &accountStoreMock{
			GetProfileStub: func(_ context.Context, username string, viewerUUID *uuid.UUID) (account.Profile, error) {
				expect.Equal(t, username, "alice")
				expect.Equal(t, viewerUUID, nil)
				return profile, nil
			},
		},
		expectedStatus: http.StatusOK,
		expectedBody:   profileJSON,
	})
	run("Viewer", testCase{
		request: authedRequest(http.MethodGet, "/users/alice", ""),
		accountStore: &accountStoreMock{
			GetViewerStub: viewerStub,
			GetProfileStub: func(_ context.Context, _ string, viewerUUID *uuid.UUID) (account.Profile, error) {
				expect.Equal(t, viewerUUID, &testAccountUUID)
				return profile, nil
			},
		},
		expectedStatus: http.StatusOK,
		expectedBody:   profileJSON,
	})
}

func Test_app_getUserPostsEndpoint(t *testing.T) {
	cursor := pagination.Cursor{CreatedAt: testCreatedAt, UUID: testPostUUID}
	profile := account.Profile{UserSummary: account.UserSummary{UUID: testAccountUUID, Username: "alice"}}
	profileStub := func(context.Context, string, *uuid.UUID) (account.Profile, error) { return profile, nil }

	type testCase struct {
		path           string
		accountStore   *accountStoreMock
		postStore      *postStoreMock
		expectedStatus int
		check          func(t *testing.T, body map[string]any)
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.accountStore.T = t
			testCase.postStore.T = t
			a := &app{accountStore: testCase.accountStore, postStore: testCase.postStore, logger: zap.NewNop()}
			status, body := serveJSON(t, a, httptest.NewRequest(http.MethodGet, testCase.path, nil))
			expect.Equal(t, status, testCase.expectedStatus)
			if testCase.check != nil {
				testCase.check(t, body)
			}
		})
	}

	run("InvalidUsername", testCase{
		path: "/users/Bad-Name/posts", accountStore: &accountStoreMock{}, postStore: &postStoreMock{},
		expectedStatus: http.StatusNotFound,
	})
	run("InvalidCursor", testCase{
		path: "/users/alice/posts?cursor=%21%21", accountStore: &accountStoreMock{}, postStore: &postStoreMock{},
		expectedStatus: http.StatusBadRequest,
		check: func(t *testing.T, body map[string]any) {
			expect.Equal[any](t, body["type"], pagination.ErrTypeInvalidPagination)
		},
	})
	run("InvalidLimit", testCase{
		path: "/users/alice/posts?limit=99", accountStore: &accountStoreMock{}, postStore: &postStoreMock{},
		expectedStatus: http.StatusBadRequest,
	})
	run("UserNotFound", testCase{
		path: "/users/nobody/posts",
		accountStore: &accountStoreMock{GetProfileStub: func(context.Context, string, *uuid.UUID) (account.Profile, error) {
			return account.Profile{}, postgres.ErrAccountNotFound
		}},
		postStore:      &postStoreMock{},
		expectedStatus: http.StatusNotFound,
	})
	run("ListError", testCase{
		path:         "/users/alice/posts",
		accountStore: &accountStoreMock{GetProfileStub: profileStub},
		postStore: &postStoreMock{ListUserPostsStub: func(context.Context, uuid.UUID, *uuid.UUID, pagination.Cursor, int) ([]post.Post, error) {
			return nil, expect.ErrTest
		}},
		expectedStatus: http.StatusInternalServerError,
	})
	run("Empty", testCase{
		path:         "/users/alice/posts",
		accountStore: &accountStoreMock{GetProfileStub: profileStub},
		postStore: &postStoreMock{ListUserPostsStub: func(context.Context, uuid.UUID, *uuid.UUID, pagination.Cursor, int) ([]post.Post, error) {
			return nil, nil
		}},
		expectedStatus: http.StatusOK,
		check: func(t *testing.T, body map[string]any) {
			expect.Equal[any](t, body, map[string]any{"items": []any{}})
		},
	})
	run("MoreThanLimit", testCase{
		path:         "/users/alice/posts?limit=2&cursor=" + pagination.Encode(cursor),
		accountStore: &accountStoreMock{GetProfileStub: profileStub},
		postStore: &postStoreMock{ListUserPostsStub: func(_ context.Context, accountUUID uuid.UUID, viewer *uuid.UUID, got pagination.Cursor, limit int) ([]post.Post, error) {
			expect.Equal(t, accountUUID, testAccountUUID)
			expect.Equal(t, viewer, nil)
			expect.Equal(t, got, cursor)
			expect.Equal(t, limit, 2)
			// The store returns limit+1 rows; the handler drops the extra one.
			return []post.Post{testPost(), testPost(), testPost()}, nil
		}},
		expectedStatus: http.StatusOK,
		check: func(t *testing.T, body map[string]any) {
			items, _ := body["items"].([]any)
			expect.Equal(t, len(items), 2)
			expect.Equal[any](t, body["nextCursor"], pagination.Encode(pagination.Cursor{CreatedAt: testCreatedAt, UUID: testPostUUID}))
		},
	})
}
