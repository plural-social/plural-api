package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/account"
)

func Test_app_getMeEndpoint(t *testing.T) {
	viewerStub := func(context.Context, []byte) (account.Viewer, error) {
		return account.Viewer{UUID: testAccountUUID, Username: "alice", Role: account.RoleUser}, nil
	}

	type testCase struct {
		header         string
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

			request := httptest.NewRequest(http.MethodGet, "/me", nil)
			if testCase.header != "" {
				request.Header.Set("Authorization", testCase.header)
			}
			recorder := httptest.NewRecorder()
			a.routes().ServeHTTP(recorder, request)

			var actualBody map[string]any
			expect.ErrorNil(t, json.Unmarshal(recorder.Body.Bytes(), &actualBody))
			expect.Equal(t, recorder.Code, testCase.expectedStatus)
			expect.Equal(t, actualBody, testCase.expectedBody)
		})
	}

	run("Unauthenticated", testCase{
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusUnauthorized,
		expectedBody:   map[string]any{"error": "authentication required", "type": account.ErrTypeUnauthenticated},
	})
	run("GetMeError", testCase{
		header: "Bearer abc",
		accountStore: &accountStoreMock{
			GetViewerStub: viewerStub,
			GetMeStub:     func(context.Context, uuid.UUID) (account.Me, error) { return account.Me{}, expect.ErrTest },
		},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   map[string]any{"error": "internal server error"},
	})
	run("Success", testCase{
		header: "Bearer abc",
		accountStore: &accountStoreMock{
			GetViewerStub: viewerStub,
			GetMeStub: func(_ context.Context, accountUUID uuid.UUID) (account.Me, error) {
				expect.Equal(t, accountUUID, testAccountUUID)
				return account.Me{UUID: testAccountUUID, Username: "alice", Email: "alice@example.com", Role: account.RoleUser, CreatedAt: testCreatedAt}, nil
			},
		},
		expectedStatus: http.StatusOK,
		expectedBody: map[string]any{
			"uuid":        testAccountUUID.String(),
			"username":    "alice",
			"displayName": "",
			"email":       "alice@example.com",
			"bio":         "",
			"role":        "user",
			"createdAt":   "2026-10-01T08:00:00Z",
		},
	})
}
