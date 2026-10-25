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

func Test_app_postAccountEndpoint(t *testing.T) {
	var (
		accountUUID = uuid.MustParse("0bc5b194-85ce-4e49-96be-0c91916e90ac")
		createdAt   = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
		validBody   = `{"username":" Lucas_F ","email":" Lucas@Example.com ","password":"correct horse battery staple"}`
	)

	type testCase struct {
		body           string
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

			recorder := httptest.NewRecorder()
			a.routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(testCase.body)))

			var actualBody map[string]any
			expect.ErrorNil(t, json.Unmarshal(recorder.Body.Bytes(), &actualBody))
			expect.Equal(t, recorder.Code, testCase.expectedStatus)
			expect.Equal(t, actualBody, testCase.expectedBody)
		})
	}

	run("MalformedJSON", testCase{
		body:           `{"username":`,
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   map[string]any{"error": "decoding request body: unexpected EOF"},
	})
	run("UnknownField", testCase{
		body:           `{"admin":true}`,
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   map[string]any{"error": `decoding request body: json: unknown field "admin"`},
	})
	run("ValidationFailures", testCase{
		body:           `{"username":"no spaces!","email":"not-an-email","password":"short"}`,
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusBadRequest,
		expectedBody: map[string]any{
			"error": "invalid request body",
			"type":  account.ErrTypeValidation,
			"details": map[string]any{
				"username": "must be 1-30 characters of a-z, 0-9 or _",
				"email":    "must be a valid email address",
				"password": "must be 8-128 characters",
			},
		},
	})
	run("UsernameConflict", testCase{
		body: validBody,
		accountStore: &accountStoreMock{
			InsertAccountStub: func(ctx context.Context, newAccount account.NewAccount) (account.Account, error) {
				return account.Account{}, postgres.UsernameConflictError{Username: newAccount.Username}
			},
		},
		expectedStatus: http.StatusConflict,
		expectedBody: map[string]any{
			"error": "username 'lucas_f' is already taken",
			"type":  account.ErrTypeUsernameConflict,
		},
	})
	run("EmailConflict", testCase{
		body: validBody,
		accountStore: &accountStoreMock{
			InsertAccountStub: func(ctx context.Context, newAccount account.NewAccount) (account.Account, error) {
				return account.Account{}, postgres.EmailConflictError{}
			},
		},
		expectedStatus: http.StatusConflict,
		expectedBody: map[string]any{
			"error": "email is already registered",
			"type":  account.ErrTypeEmailConflict,
		},
	})
	run("InsertAccountError", testCase{
		body: validBody,
		accountStore: &accountStoreMock{
			InsertAccountStub: func(ctx context.Context, newAccount account.NewAccount) (account.Account, error) {
				return account.Account{}, expect.ErrTest
			},
		},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   map[string]any{"error": "internal server error"},
	})
	run("Success", testCase{
		body: validBody,
		accountStore: &accountStoreMock{
			InsertAccountStub: func(ctx context.Context, newAccount account.NewAccount) (account.Account, error) {
				expect.Equal(t, newAccount.Username, "lucas_f")
				expect.Equal(t, newAccount.Email, "lucas@example.com")
				matches := expect.Must(account.VerifyPassword("correct horse battery staple", newAccount.PasswordHash))(t)
				expect.Equal(t, matches, true)
				return account.Account{UUID: accountUUID, Username: newAccount.Username, CreatedAt: createdAt}, nil
			},
		},
		expectedStatus: http.StatusCreated,
		expectedBody: map[string]any{
			"uuid":      accountUUID.String(),
			"username":  "lucas_f",
			"createdAt": "2026-10-25T12:00:00Z",
		},
	})
}
