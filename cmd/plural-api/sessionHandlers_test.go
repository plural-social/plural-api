package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/postgres"
)

var (
	testAccountUUID = uuid.MustParse("0bc5b194-85ce-4e49-96be-0c91916e90ac")
	testNow         = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	testCreatedAt   = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
)

func Test_app_postSessionEndpoint(t *testing.T) {
	passwordHash := expect.Must(account.HashPassword("password123"))(t)
	validBody := `{"login":" Alice@Example.com ","password":"password123"}`
	credentials := account.Credentials{AccountUUID: testAccountUUID, PasswordHash: passwordHash}
	me := account.Me{UUID: testAccountUUID, Username: "alice", Email: "alice@example.com", Role: account.RoleUser, CreatedAt: testCreatedAt}

	type testCase struct {
		body           string
		accountStore   *accountStoreMock
		expectedStatus int
		check          func(t *testing.T, body map[string]any)
		expectedBody   map[string]any
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.accountStore.T = t
			a := &app{accountStore: testCase.accountStore, logger: zap.NewNop(), now: func() time.Time { return testNow }}

			request := httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(testCase.body))
			request.Header.Set("User-Agent", "test-agent")
			recorder := httptest.NewRecorder()
			a.routes().ServeHTTP(recorder, request)

			var actualBody map[string]any
			expect.ErrorNil(t, json.Unmarshal(recorder.Body.Bytes(), &actualBody))
			expect.Equal(t, recorder.Code, testCase.expectedStatus)
			if testCase.check != nil {
				testCase.check(t, actualBody)
				return
			}
			expect.Equal(t, actualBody, testCase.expectedBody)
		})
	}

	invalidCredentials := map[string]any{"error": "invalid login or password", "type": account.ErrTypeInvalidCredentials}
	internalError := map[string]any{"error": "internal server error"}

	run("MalformedJSON", testCase{
		body:           `{"login":`,
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   map[string]any{"error": "decoding request body: unexpected EOF"},
	})
	run("ValidationFailures", testCase{
		body:           `{"login":"  ","password":""}`,
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusBadRequest,
		expectedBody: map[string]any{
			"error": "invalid request body",
			"type":  account.ErrTypeValidation,
			"details": map[string]any{
				"login":    "must not be empty",
				"password": "must be 1-128 characters",
			},
		},
	})
	run("AccountNotFound", testCase{
		body: validBody,
		accountStore: &accountStoreMock{
			GetCredentialsStub: func(context.Context, string) (account.Credentials, error) {
				return account.Credentials{}, postgres.ErrAccountNotFound
			},
		},
		expectedStatus: http.StatusUnauthorized,
		expectedBody:   invalidCredentials,
	})
	run("CredentialsError", testCase{
		body: validBody,
		accountStore: &accountStoreMock{
			GetCredentialsStub: func(context.Context, string) (account.Credentials, error) {
				return account.Credentials{}, expect.ErrTest
			},
		},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   internalError,
	})
	run("WrongPassword", testCase{
		body: `{"login":"alice","password":"wrong"}`,
		accountStore: &accountStoreMock{
			GetCredentialsStub: func(context.Context, string) (account.Credentials, error) {
				return credentials, nil
			},
		},
		expectedStatus: http.StatusUnauthorized,
		expectedBody:   invalidCredentials,
	})
	run("InsertSessionError", testCase{
		body: validBody,
		accountStore: &accountStoreMock{
			GetCredentialsStub: func(context.Context, string) (account.Credentials, error) {
				return credentials, nil
			},
			InsertSessionStub: func(context.Context, account.NewSession) error { return expect.ErrTest },
		},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   internalError,
	})
	run("GetMeError", testCase{
		body: validBody,
		accountStore: &accountStoreMock{
			GetCredentialsStub: func(context.Context, string) (account.Credentials, error) {
				return credentials, nil
			},
			InsertSessionStub: func(context.Context, account.NewSession) error { return nil },
			GetMeStub: func(context.Context, uuid.UUID) (account.Me, error) {
				return account.Me{}, expect.ErrTest
			},
		},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   internalError,
	})

	var insertedHash []byte
	run("Success", testCase{
		body: validBody,
		accountStore: &accountStoreMock{
			GetCredentialsStub: func(_ context.Context, login string) (account.Credentials, error) {
				expect.Equal(t, login, "alice@example.com")
				return credentials, nil
			},
			InsertSessionStub: func(_ context.Context, session account.NewSession) error {
				insertedHash = session.TokenHash
				expect.Equal(t, session.AccountUUID, testAccountUUID)
				expect.Equal(t, session.UserAgent, "test-agent")
				expect.Equal(t, session.ExpiresAt, testNow.Add(account.SessionLifetime))
				return nil
			},
			GetMeStub: func(_ context.Context, accountUUID uuid.UUID) (account.Me, error) {
				expect.Equal(t, accountUUID, testAccountUUID)
				return me, nil
			},
		},
		expectedStatus: http.StatusCreated,
		check: func(t *testing.T, body map[string]any) {
			token, _ := body["token"].(string)
			expect.Equal(t, len(token), 43)
			expect.Equal(t, insertedHash, account.HashSessionToken(token))
			expect.Equal[any](t, body["expiresAt"], "2027-01-23T12:00:00Z")
			expect.Equal[any](t, body["me"], map[string]any{
				"uuid":        testAccountUUID.String(),
				"username":    "alice",
				"displayName": "",
				"email":       "alice@example.com",
				"bio":         "",
				"role":        "user",
				"createdAt":   "2026-10-01T08:00:00Z",
			})
		},
	})
}

func Test_app_deleteCurrentSessionEndpoint(t *testing.T) {
	viewerStub := func(context.Context, []byte) (account.Viewer, error) {
		return account.Viewer{UUID: testAccountUUID, Username: "alice", Role: account.RoleUser}, nil
	}

	type testCase struct {
		header         string
		accountStore   *accountStoreMock
		expectedStatus int
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.accountStore.T = t
			a := &app{accountStore: testCase.accountStore, logger: zap.NewNop()}

			request := httptest.NewRequest(http.MethodDelete, "/sessions/current", nil)
			if testCase.header != "" {
				request.Header.Set("Authorization", testCase.header)
			}
			recorder := httptest.NewRecorder()
			a.routes().ServeHTTP(recorder, request)
			expect.Equal(t, recorder.Code, testCase.expectedStatus)
		})
	}

	run("Unauthenticated", testCase{
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusUnauthorized,
	})
	run("DeleteError", testCase{
		header: "Bearer abc",
		accountStore: &accountStoreMock{
			GetViewerStub:     viewerStub,
			DeleteSessionStub: func(context.Context, []byte) error { return expect.ErrTest },
		},
		expectedStatus: http.StatusInternalServerError,
	})
	run("Success", testCase{
		header: "Bearer abc",
		accountStore: &accountStoreMock{
			GetViewerStub: viewerStub,
			DeleteSessionStub: func(_ context.Context, tokenHash []byte) error {
				expect.Equal(t, tokenHash, account.HashSessionToken("abc"))
				return nil
			},
		},
		expectedStatus: http.StatusNoContent,
	})
}
