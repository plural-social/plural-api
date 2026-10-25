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
	"github.com/plural-social/plural-api/internal/moderation"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/post"
	"github.com/plural-social/plural-api/internal/postgres"
	"github.com/plural-social/plural-api/internal/social"
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

func Test_app_putFollowEndpoint(t *testing.T) {
	followee := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	byUsername := func(target uuid.UUID) func(context.Context, string) (uuid.UUID, error) {
		return func(context.Context, string) (uuid.UUID, error) { return target, nil }
	}

	type testCase struct {
		request         *http.Request
		accountStore    *accountStoreMock
		socialStore     *socialStoreMock
		moderationStore *moderationStoreMock
		expectedStatus  int
		expectedBody    map[string]any
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.accountStore.T = t
			testCase.accountStore.GetViewerStub = viewerStub
			testCase.socialStore.T = t
			if testCase.moderationStore == nil {
				testCase.moderationStore = &moderationStoreMock{IsBlockedEitherWayStub: notBlocked}
			}
			testCase.moderationStore.T = t
			a := &app{
				accountStore:    testCase.accountStore,
				socialStore:     testCase.socialStore,
				moderationStore: testCase.moderationStore,
				logger:          zap.NewNop(),
			}
			status, body := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
			expect.Equal(t, body, testCase.expectedBody)
		})
	}

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodPut, "/users/bob/follow", nil),
		accountStore:   &accountStoreMock{},
		socialStore:    &socialStoreMock{},
		expectedStatus: http.StatusUnauthorized,
		expectedBody:   map[string]any{"error": "authentication required", "type": "unauthenticated"},
	})
	run("InvalidUsername", testCase{
		request:        authedRequest(http.MethodPut, "/users/Bad-Name/follow", ""),
		accountStore:   &accountStoreMock{},
		socialStore:    &socialStoreMock{},
		expectedStatus: http.StatusNotFound,
		expectedBody:   map[string]any{"error": "not found", "type": errTypeNotFound},
	})
	run("UserNotFound", testCase{
		request: authedRequest(http.MethodPut, "/users/nobody/follow", ""),
		accountStore: &accountStoreMock{GetAccountUUIDStub: func(context.Context, string) (uuid.UUID, error) {
			return uuid.Nil, postgres.ErrAccountNotFound
		}},
		socialStore:    &socialStoreMock{},
		expectedStatus: http.StatusNotFound,
		expectedBody:   map[string]any{"error": "not found", "type": errTypeNotFound},
	})
	run("Self", testCase{
		request:        authedRequest(http.MethodPut, "/users/alice/follow", ""),
		accountStore:   &accountStoreMock{GetAccountUUIDStub: byUsername(testAccountUUID)},
		socialStore:    &socialStoreMock{},
		expectedStatus: http.StatusUnprocessableEntity,
		expectedBody:   map[string]any{"error": "you can't follow yourself", "type": social.ErrTypeCannotFollowSelf},
	})
	run("Blocked", testCase{
		request:      authedRequest(http.MethodPut, "/users/bob/follow", ""),
		accountStore: &accountStoreMock{GetAccountUUIDStub: byUsername(followee)},
		socialStore:  &socialStoreMock{},
		moderationStore: &moderationStoreMock{IsBlockedEitherWayStub: func(_ context.Context, a, b uuid.UUID) (bool, error) {
			expect.Equal(t, a, testAccountUUID)
			expect.Equal(t, b, followee)
			return true, nil
		}},
		expectedStatus: http.StatusUnprocessableEntity,
		expectedBody:   map[string]any{"error": "you can't follow this account", "type": moderation.ErrTypeBlocked},
	})
	run("FollowError", testCase{
		request:      authedRequest(http.MethodPut, "/users/bob/follow", ""),
		accountStore: &accountStoreMock{GetAccountUUIDStub: byUsername(followee)},
		socialStore: &socialStoreMock{FollowStub: func(context.Context, uuid.UUID, uuid.UUID) error {
			return expect.ErrTest
		}},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   map[string]any{"error": "internal server error"},
	})
	run("Success", testCase{
		request:      authedRequest(http.MethodPut, "/users/bob/follow", ""),
		accountStore: &accountStoreMock{GetAccountUUIDStub: byUsername(followee)},
		socialStore: &socialStoreMock{FollowStub: func(_ context.Context, follower, got uuid.UUID) error {
			expect.Equal(t, follower, testAccountUUID)
			expect.Equal(t, got, followee)
			return nil
		}},
		expectedStatus: http.StatusNoContent,
	})
}

func Test_app_deleteFollowEndpoint(t *testing.T) {
	followee := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	found := func(context.Context, string) (uuid.UUID, error) { return followee, nil }

	type testCase struct {
		request        *http.Request
		accountStore   *accountStoreMock
		socialStore    *socialStoreMock
		expectedStatus int
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.accountStore.T = t
			testCase.accountStore.GetViewerStub = viewerStub
			testCase.socialStore.T = t
			a := &app{accountStore: testCase.accountStore, socialStore: testCase.socialStore, logger: zap.NewNop()}
			status, _ := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
		})
	}

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodDelete, "/users/bob/follow", nil),
		accountStore:   &accountStoreMock{},
		socialStore:    &socialStoreMock{},
		expectedStatus: http.StatusUnauthorized,
	})
	run("UserNotFound", testCase{
		request: authedRequest(http.MethodDelete, "/users/nobody/follow", ""),
		accountStore: &accountStoreMock{GetAccountUUIDStub: func(context.Context, string) (uuid.UUID, error) {
			return uuid.Nil, postgres.ErrAccountNotFound
		}},
		socialStore:    &socialStoreMock{},
		expectedStatus: http.StatusNotFound,
	})
	run("UnfollowError", testCase{
		request:      authedRequest(http.MethodDelete, "/users/bob/follow", ""),
		accountStore: &accountStoreMock{GetAccountUUIDStub: found},
		socialStore: &socialStoreMock{UnfollowStub: func(context.Context, uuid.UUID, uuid.UUID) error {
			return expect.ErrTest
		}},
		expectedStatus: http.StatusInternalServerError,
	})
	run("Success", testCase{
		request:      authedRequest(http.MethodDelete, "/users/bob/follow", ""),
		accountStore: &accountStoreMock{GetAccountUUIDStub: found},
		socialStore: &socialStoreMock{UnfollowStub: func(_ context.Context, follower, got uuid.UUID) error {
			expect.Equal(t, follower, testAccountUUID)
			expect.Equal(t, got, followee)
			return nil
		}},
		expectedStatus: http.StatusNoContent,
	})
}
