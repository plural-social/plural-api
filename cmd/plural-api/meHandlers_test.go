package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/postgres"
)

func Test_app_getMeEndpoint(t *testing.T) {
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

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodGet, "/me", nil),
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusUnauthorized,
		expectedBody:   map[string]any{"error": "authentication required", "type": account.ErrTypeUnauthenticated},
	})
	run("GetMeError", testCase{
		request: authedRequest(http.MethodGet, "/me", ""),
		accountStore: &accountStoreMock{
			GetViewerStub: viewerStub,
			GetMeStub:     func(context.Context, uuid.UUID) (account.Me, error) { return account.Me{}, expect.ErrTest },
		},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   map[string]any{"error": "internal server error"},
	})
	run("Success", testCase{
		request: authedRequest(http.MethodGet, "/me", ""),
		accountStore: &accountStoreMock{
			GetViewerStub: viewerStub,
			GetMeStub: func(_ context.Context, accountUUID uuid.UUID) (account.Me, error) {
				expect.Equal(t, accountUUID, testAccountUUID)
				return testMe, nil
			},
		},
		expectedStatus: http.StatusOK,
		expectedBody:   testMeJSON,
	})
}

func Test_app_patchMeEndpoint(t *testing.T) {
	validationError := func(field, message string) map[string]any {
		return map[string]any{
			"error":   "invalid request body",
			"type":    account.ErrTypeValidation,
			"details": map[string]any{field: message},
		}
	}

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

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodPatch, "/me", strings.NewReader(`{"bio":"hi"}`)),
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusUnauthorized,
		expectedBody:   map[string]any{"error": "authentication required", "type": account.ErrTypeUnauthenticated},
	})
	run("MalformedJSON", testCase{
		request:        authedRequest(http.MethodPatch, "/me", `{"bio":`),
		accountStore:   &accountStoreMock{GetViewerStub: viewerStub},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   map[string]any{"error": "decoding request body: unexpected EOF"},
	})
	run("NothingToUpdate", testCase{
		request:        authedRequest(http.MethodPatch, "/me", `{}`),
		accountStore:   &accountStoreMock{GetViewerStub: viewerStub},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   validationError("body", "nothing to update"),
	})
	run("DisplayNameTooLong", testCase{
		// 51 runes, 102 bytes: the limit counts characters, not bytes.
		request:        authedRequest(http.MethodPatch, "/me", `{"displayName":"`+strings.Repeat("é", 51)+`"}`),
		accountStore:   &accountStoreMock{GetViewerStub: viewerStub},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   validationError("displayName", "must be at most 50 characters"),
	})
	run("DisplayNameNewline", testCase{
		request:        authedRequest(http.MethodPatch, "/me", `{"displayName":"Bob\nB"}`),
		accountStore:   &accountStoreMock{GetViewerStub: viewerStub},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   validationError("displayName", "must be a single line"),
	})
	run("BioTooLong", testCase{
		request:        authedRequest(http.MethodPatch, "/me", `{"bio":"`+strings.Repeat("a", 301)+`"}`),
		accountStore:   &accountStoreMock{GetViewerStub: viewerStub},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   validationError("bio", "must be at most 300 characters"),
	})
	run("UpdateError", testCase{
		request: authedRequest(http.MethodPatch, "/me", `{"bio":"hi"}`),
		accountStore: &accountStoreMock{
			GetViewerStub:     viewerStub,
			UpdateProfileStub: func(context.Context, uuid.UUID, account.ProfileUpdate) error { return expect.ErrTest },
		},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   map[string]any{"error": "internal server error"},
	})
	run("GetMeError", testCase{
		request: authedRequest(http.MethodPatch, "/me", `{"bio":"hi"}`),
		accountStore: &accountStoreMock{
			GetViewerStub:     viewerStub,
			UpdateProfileStub: func(context.Context, uuid.UUID, account.ProfileUpdate) error { return nil },
			GetMeStub:         func(context.Context, uuid.UUID) (account.Me, error) { return account.Me{}, expect.ErrTest },
		},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   map[string]any{"error": "internal server error"},
	})
	run("OnlyBio", testCase{
		request: authedRequest(http.MethodPatch, "/me", `{"bio":"hi"}`),
		accountStore: &accountStoreMock{
			GetViewerStub: viewerStub,
			UpdateProfileStub: func(_ context.Context, _ uuid.UUID, update account.ProfileUpdate) error {
				expect.Equal(t, update, account.ProfileUpdate{Bio: new("hi")})
				return nil
			},
			GetMeStub: func(context.Context, uuid.UUID) (account.Me, error) { return testMe, nil },
		},
		expectedStatus: http.StatusOK,
		expectedBody:   testMeJSON,
	})
	run("Success", testCase{
		request: authedRequest(http.MethodPatch, "/me", `{"displayName":"  Bob B ","bio":" hi  "}`),
		accountStore: &accountStoreMock{
			GetViewerStub: viewerStub,
			UpdateProfileStub: func(_ context.Context, accountUUID uuid.UUID, update account.ProfileUpdate) error {
				expect.Equal(t, accountUUID, testAccountUUID)
				expect.Equal(t, update, account.ProfileUpdate{DisplayName: new("Bob B"), Bio: new("hi")})
				return nil
			},
			GetMeStub: func(context.Context, uuid.UUID) (account.Me, error) { return testMe, nil },
		},
		expectedStatus: http.StatusOK,
		expectedBody:   testMeJSON,
	})
}

// testAvatarKey is the thumbnail key of testMediaUUID, which the store would return for an avatar.
var testAvatarKey = media.ThumbnailKey(testMediaUUID)

func Test_app_putAvatarEndpoint(t *testing.T) {
	body := `{"mediaUuid":"` + testMediaUUID.String() + `"}`
	meWithAvatar := testMe
	meWithAvatar.AvatarKey = testAvatarKey
	expectedMe := map[string]any{}
	for k, v := range testMeJSON {
		expectedMe[k] = v
	}
	expectedMe["avatarUrl"] = "http://example.com/files/" + testAvatarKey

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
			testCase.accountStore.GetViewerStub = viewerStub
			a := &app{accountStore: testCase.accountStore, logger: zap.NewNop()}
			status, got := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
			expect.Equal(t, got, testCase.expectedBody)
		})
	}

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodPut, "/me/avatar", strings.NewReader(body)),
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusUnauthorized,
		expectedBody:   map[string]any{"error": "authentication required", "type": account.ErrTypeUnauthenticated},
	})
	run("InvalidUUID", testCase{
		request:        authedRequest(http.MethodPut, "/me/avatar", `{"mediaUuid":"nope"}`),
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusBadRequest,
		expectedBody: map[string]any{
			"error": "invalid request body", "type": "validation",
			"details": map[string]any{"mediaUuid": "must be a valid UUID"},
		},
	})
	run("InvalidMedia", testCase{
		request: authedRequest(http.MethodPut, "/me/avatar", body),
		accountStore: &accountStoreMock{SetAvatarStub: func(context.Context, uuid.UUID, uuid.UUID) error {
			return postgres.ErrInvalidAvatarMedia
		}},
		expectedStatus: http.StatusUnprocessableEntity,
		expectedBody: map[string]any{
			"error": "use a ready image you uploaded that is not used by a post",
			"type":  account.ErrTypeInvalidAvatarMedia,
		},
	})
	run("SetError", testCase{
		request: authedRequest(http.MethodPut, "/me/avatar", body),
		accountStore: &accountStoreMock{SetAvatarStub: func(context.Context, uuid.UUID, uuid.UUID) error {
			return expect.ErrTest
		}},
		expectedStatus: http.StatusInternalServerError,
		expectedBody:   map[string]any{"error": "internal server error"},
	})
	run("Success", testCase{
		request: authedRequest(http.MethodPut, "/me/avatar", body),
		accountStore: &accountStoreMock{
			SetAvatarStub: func(_ context.Context, accountUUID, mediaUUID uuid.UUID) error {
				expect.Equal(t, accountUUID, testAccountUUID)
				expect.Equal(t, mediaUUID, testMediaUUID)
				return nil
			},
			GetMeStub: func(context.Context, uuid.UUID) (account.Me, error) { return meWithAvatar, nil },
		},
		expectedStatus: http.StatusOK,
		expectedBody:   expectedMe,
	})
}

func Test_app_deleteAvatarEndpoint(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		a := &app{
			accountStore: &accountStoreMock{
				T:             t,
				GetViewerStub: viewerStub,
				RemoveAvatarStub: func(_ context.Context, accountUUID uuid.UUID) error {
					expect.Equal(t, accountUUID, testAccountUUID)
					return nil
				},
				GetMeStub: func(context.Context, uuid.UUID) (account.Me, error) { return testMe, nil },
			},
			logger: zap.NewNop(),
		}
		status, got := serveJSON(t, a, authedRequest(http.MethodDelete, "/me/avatar", ""))
		expect.Equal(t, status, http.StatusOK)
		_, hasAvatar := got["avatarUrl"]
		expect.Equal(t, hasAvatar, false)
	})
}

func Test_app_deleteMeEndpoint(t *testing.T) {
	hash := expect.Must(account.HashPassword("password123"))(t)
	keys := []string{"media/a/display.jpg", "media/a/thumb.jpg", "media/b/original"}
	hashStub := func(context.Context, uuid.UUID) (string, error) { return hash, nil }
	body := `{"password":"password123"}`

	type testCase struct {
		request        *http.Request
		accountStore   *accountStoreMock
		expectedStatus int
		expectedBody   map[string]any
		expectedDelete []string
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.accountStore.T = t
			testCase.accountStore.GetViewerStub = viewerStub
			var deleted []string
			a := &app{
				accountStore: testCase.accountStore,
				blobStore: &blobStoreMock{T: t, DeleteStub: func(_ context.Context, key string) error {
					deleted = append(deleted, key)
					// A storage failure must not stop the other files from being deleted.
					if key == keys[0] {
						return expect.ErrTest
					}
					return nil
				}},
				logger: zap.NewNop(),
			}
			status, got := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
			if testCase.expectedBody != nil {
				expect.Equal(t, got, testCase.expectedBody)
			}
			expect.Equal(t, deleted, testCase.expectedDelete)
		})
	}

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodDelete, "/me", strings.NewReader(body)),
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusUnauthorized,
	})
	run("MalformedJSON", testCase{
		request:        authedRequest(http.MethodDelete, "/me", `{"password":`),
		accountStore:   &accountStoreMock{},
		expectedStatus: http.StatusBadRequest,
	})
	run("WrongPassword", testCase{
		request:        authedRequest(http.MethodDelete, "/me", `{"password":"nope"}`),
		accountStore:   &accountStoreMock{GetPasswordHashStub: hashStub},
		expectedStatus: http.StatusUnauthorized,
		expectedBody:   map[string]any{"error": "invalid login or password", "type": account.ErrTypeInvalidCredentials},
	})
	run("DeleteError", testCase{
		request: authedRequest(http.MethodDelete, "/me", body),
		accountStore: &accountStoreMock{
			GetPasswordHashStub: hashStub,
			ListMediaKeysStub:   func(context.Context, uuid.UUID) ([]string, error) { return keys, nil },
			DeleteAccountStub:   func(context.Context, uuid.UUID) error { return expect.ErrTest },
		},
		expectedStatus: http.StatusInternalServerError,
		// Files stay: the account still exists, so its content must too.
	})
	run("Success", testCase{
		request: authedRequest(http.MethodDelete, "/me", body),
		accountStore: &accountStoreMock{
			GetPasswordHashStub: hashStub,
			ListMediaKeysStub:   func(context.Context, uuid.UUID) ([]string, error) { return keys, nil },
			DeleteAccountStub: func(_ context.Context, accountUUID uuid.UUID) error {
				expect.Equal(t, accountUUID, testAccountUUID)
				return nil
			},
		},
		expectedStatus: http.StatusNoContent,
		expectedDelete: keys,
	})
}
