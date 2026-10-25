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
