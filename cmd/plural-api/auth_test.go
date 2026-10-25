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

func Test_bearerToken(t *testing.T) {
	type testCase struct {
		header        string
		expectedToken string
		expectedOK    bool
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if testCase.header != "" {
				r.Header.Set("Authorization", testCase.header)
			}
			token, ok := bearerToken(r)
			expect.Equal(t, token, testCase.expectedToken)
			expect.Equal(t, ok, testCase.expectedOK)
		})
	}

	run("Missing", testCase{})
	run("WrongScheme", testCase{header: "Basic x"})
	run("EmptyToken", testCase{header: "Bearer "})
	run("LowercaseScheme", testCase{header: "bearer abc", expectedToken: "abc", expectedOK: true})
	run("Valid", testCase{header: "Bearer abc", expectedToken: "abc", expectedOK: true})
}

func Test_app_requireViewer(t *testing.T) {
	viewer := account.Viewer{UUID: uuid.MustParse("0bc5b194-85ce-4e49-96be-0c91916e90ac"), Username: "alice", Role: account.RoleUser}

	type testCase struct {
		header         string
		accountStore   *accountStoreMock
		expectedStatus int
		expectedViewer *account.Viewer
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.accountStore.T = t
			a := &app{accountStore: testCase.accountStore, logger: zap.NewNop()}

			var actualViewer *account.Viewer
			h := a.requireViewer(func(_ *http.Request, v account.Viewer) (int, any) {
				actualViewer = &v
				return http.StatusOK, nil
			})
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if testCase.header != "" {
				r.Header.Set("Authorization", testCase.header)
			}
			status, _ := h(r)
			expect.Equal(t, status, testCase.expectedStatus)
			expect.Equal(t, actualViewer, testCase.expectedViewer)
		})
	}

	run("NoHeader", testCase{
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusUnauthorized,
	})
	run("SessionNotFound", testCase{
		header: "Bearer abc",
		accountStore: &accountStoreMock{
			GetViewerStub: func(context.Context, []byte) (account.Viewer, error) {
				return account.Viewer{}, postgres.ErrSessionNotFound
			},
		},
		expectedStatus: http.StatusUnauthorized,
	})
	run("StoreError", testCase{
		header: "Bearer abc",
		accountStore: &accountStoreMock{
			GetViewerStub: func(context.Context, []byte) (account.Viewer, error) {
				return account.Viewer{}, expect.ErrTest
			},
		},
		expectedStatus: http.StatusInternalServerError,
	})
	run("Valid", testCase{
		header: "Bearer abc",
		accountStore: &accountStoreMock{
			GetViewerStub: func(_ context.Context, tokenHash []byte) (account.Viewer, error) {
				expect.Equal(t, tokenHash, account.HashSessionToken("abc"))
				return viewer, nil
			},
		},
		expectedStatus: http.StatusOK,
		expectedViewer: &viewer,
	})
}

func Test_app_optionalViewer(t *testing.T) {
	viewer := account.Viewer{UUID: uuid.MustParse("0bc5b194-85ce-4e49-96be-0c91916e90ac"), Username: "alice", Role: account.RoleUser}

	type testCase struct {
		header         string
		accountStore   *accountStoreMock
		expectedStatus int
		expectedCalled bool
		expectedViewer *account.Viewer
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.accountStore.T = t
			a := &app{accountStore: testCase.accountStore, logger: zap.NewNop()}

			called := false
			var actualViewer *account.Viewer
			h := a.optionalViewer(func(_ *http.Request, v *account.Viewer) (int, any) {
				called = true
				actualViewer = v
				return http.StatusOK, nil
			})
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if testCase.header != "" {
				r.Header.Set("Authorization", testCase.header)
			}
			status, _ := h(r)
			expect.Equal(t, status, testCase.expectedStatus)
			expect.Equal(t, called, testCase.expectedCalled)
			expect.Equal(t, actualViewer, testCase.expectedViewer)
		})
	}

	run("NoHeader", testCase{
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusOK,
		expectedCalled: true,
	})
	run("InvalidToken", testCase{
		header: "Bearer abc",
		accountStore: &accountStoreMock{
			GetViewerStub: func(context.Context, []byte) (account.Viewer, error) {
				return account.Viewer{}, postgres.ErrSessionNotFound
			},
		},
		expectedStatus: http.StatusUnauthorized,
	})
	run("Valid", testCase{
		header: "Bearer abc",
		accountStore: &accountStoreMock{
			GetViewerStub: func(context.Context, []byte) (account.Viewer, error) {
				return viewer, nil
			},
		},
		expectedStatus: http.StatusOK,
		expectedCalled: true,
		expectedViewer: &viewer,
	})
}
