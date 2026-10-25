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
	"github.com/plural-social/plural-api/internal/moderation"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/post"
	"github.com/plural-social/plural-api/internal/postgres"
	"github.com/plural-social/plural-api/internal/social"
)

var testReportUUID = uuid.MustParse("3e2d1c0b-9a8f-4e7d-8c6b-5a4f3e2d1c0b")

// moderationDeps are the stores for moderationApp. Nil stores get an empty mock
// that fails the test if called.
type moderationDeps struct {
	role            string
	accountStore    *accountStoreMock
	postStore       *postStoreMock
	socialStore     *socialStoreMock
	moderationStore *moderationStoreMock
	blobStore       *blobStoreMock
}

// moderationApp builds an app whose viewer is alice with deps.role (default user).
func moderationApp(t *testing.T, deps moderationDeps) *app {
	t.Helper()
	if deps.role == "" {
		deps.role = account.RoleUser
	}
	if deps.accountStore == nil {
		deps.accountStore = &accountStoreMock{}
	}
	if deps.postStore == nil {
		deps.postStore = &postStoreMock{}
	}
	if deps.socialStore == nil {
		deps.socialStore = &socialStoreMock{}
	}
	if deps.moderationStore == nil {
		deps.moderationStore = &moderationStoreMock{}
	}
	if deps.blobStore == nil {
		deps.blobStore = &blobStoreMock{}
	}
	deps.accountStore.T, deps.postStore.T, deps.socialStore.T, deps.moderationStore.T, deps.blobStore.T = t, t, t, t, t
	deps.accountStore.GetViewerStub = viewerWithRole(deps.role)
	return &app{
		accountStore:    deps.accountStore,
		postStore:       deps.postStore,
		socialStore:     deps.socialStore,
		moderationStore: deps.moderationStore,
		blobStore:       deps.blobStore,
		logger:          zap.NewNop(),
	}
}

func Test_app_requireModerator(t *testing.T) {
	for role, expected := range map[string]int{
		account.RoleUser:      http.StatusForbidden,
		account.RoleModerator: http.StatusOK,
		account.RoleAdmin:     http.StatusOK,
	} {
		t.Run(role, func(t *testing.T) {
			a := moderationApp(t, moderationDeps{role: role})
			h := a.requireModerator(func(*http.Request, account.Viewer) (int, any) { return http.StatusOK, nil })
			status, _ := h(authedRequest(http.MethodGet, "/", ""))
			expect.Equal(t, status, expected)
		})
	}
}

func Test_app_requireAdmin(t *testing.T) {
	for role, expected := range map[string]int{
		account.RoleModerator: http.StatusForbidden,
		account.RoleAdmin:     http.StatusOK,
	} {
		t.Run(role, func(t *testing.T) {
			a := moderationApp(t, moderationDeps{role: role})
			h := a.requireAdmin(func(*http.Request, account.Viewer) (int, any) { return http.StatusOK, nil })
			status, _ := h(authedRequest(http.MethodGet, "/", ""))
			expect.Equal(t, status, expected)
		})
	}
}

func Test_app_putBlockEndpoint(t *testing.T) {
	other := otherAccountUUID
	resolveTo := func(id uuid.UUID) func(context.Context, string) (uuid.UUID, error) {
		return func(context.Context, string) (uuid.UUID, error) { return id, nil }
	}

	type testCase struct {
		request        *http.Request
		deps           moderationDeps
		expectedStatus int
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			status, _ := serveJSON(t, moderationApp(t, testCase.deps), testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
		})
	}

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodPut, "/users/bob/block", nil),
		expectedStatus: http.StatusUnauthorized,
	})
	run("UserNotFound", testCase{
		request: authedRequest(http.MethodPut, "/users/nobody/block", ""),
		deps: moderationDeps{accountStore: &accountStoreMock{GetAccountUUIDStub: func(context.Context, string) (uuid.UUID, error) {
			return uuid.Nil, postgres.ErrAccountNotFound
		}}},
		expectedStatus: http.StatusNotFound,
	})
	run("Self", testCase{
		request:        authedRequest(http.MethodPut, "/users/alice/block", ""),
		deps:           moderationDeps{accountStore: &accountStoreMock{GetAccountUUIDStub: resolveTo(testAccountUUID)}},
		expectedStatus: http.StatusUnprocessableEntity,
	})
	run("StoreError", testCase{
		request: authedRequest(http.MethodPut, "/users/bob/block", ""),
		deps: moderationDeps{
			accountStore:    &accountStoreMock{GetAccountUUIDStub: resolveTo(other)},
			moderationStore: &moderationStoreMock{BlockAccountStub: func(context.Context, uuid.UUID, uuid.UUID) error { return expect.ErrTest }},
		},
		expectedStatus: http.StatusInternalServerError,
	})
	run("Success", testCase{
		request: authedRequest(http.MethodPut, "/users/bob/block", ""),
		deps: moderationDeps{
			accountStore: &accountStoreMock{GetAccountUUIDStub: resolveTo(other)},
			moderationStore: &moderationStoreMock{BlockAccountStub: func(_ context.Context, blocker, blocked uuid.UUID) error {
				expect.Equal(t, blocker, testAccountUUID)
				expect.Equal(t, blocked, other)
				return nil
			}},
		},
		expectedStatus: http.StatusNoContent,
	})
}

func Test_app_deleteBlockEndpoint(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		a := moderationApp(t, moderationDeps{
			accountStore: &accountStoreMock{GetAccountUUIDStub: func(context.Context, string) (uuid.UUID, error) { return otherAccountUUID, nil }},
			moderationStore: &moderationStoreMock{UnblockAccountStub: func(_ context.Context, blocker, blocked uuid.UUID) error {
				expect.Equal(t, blocker, testAccountUUID)
				expect.Equal(t, blocked, otherAccountUUID)
				return nil
			}},
		})
		status, _ := serveJSON(t, a, authedRequest(http.MethodDelete, "/users/bob/block", ""))
		expect.Equal(t, status, http.StatusNoContent)
	})
}

func Test_app_getBlocksEndpoint(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		blocked := moderation.BlockedAccount{
			UserSummary: account.UserSummary{UUID: otherAccountUUID, Username: "bob", AvatarKey: testAvatarKey},
			BlockedAt:   testCreatedAt,
		}
		a := moderationApp(t, moderationDeps{moderationStore: &moderationStoreMock{
			ListBlocksStub: func(_ context.Context, blocker uuid.UUID, _ pagination.Cursor, limit int) ([]moderation.BlockedAccount, error) {
				expect.Equal(t, blocker, testAccountUUID)
				expect.Equal(t, limit, 1)
				return []moderation.BlockedAccount{blocked, blocked}, nil
			},
		}})
		status, body := serveJSON(t, a, authedRequest(http.MethodGet, "/me/blocks?limit=1", ""))
		expect.Equal(t, status, http.StatusOK)
		expect.Equal[any](t, body["items"], []any{map[string]any{
			"uuid": otherAccountUUID.String(), "username": "bob", "displayName": "",
			"avatarUrl": "http://example.com/files/" + testAvatarKey,
		}})
		expect.Equal[any](t, body["nextCursor"], pagination.Encode(pagination.Cursor{CreatedAt: testCreatedAt, UUID: otherAccountUUID}))
	})
}

func Test_app_postReportEndpoint(t *testing.T) {
	reportBody := func(targetType string, target uuid.UUID) string {
		return `{"targetType":"` + targetType + `","targetUuid":"` + target.String() + `","reason":"spam","details":" too much "}`
	}
	invalidTarget := map[string]any{"error": "this can't be reported", "type": moderation.ErrTypeInvalidReportTarget}
	postBy := func(author uuid.UUID) func(context.Context, uuid.UUID, *uuid.UUID) (post.Post, error) {
		return func(context.Context, uuid.UUID, *uuid.UUID) (post.Post, error) {
			p := testPost()
			p.Author.UUID = author
			return p, nil
		}
	}

	type testCase struct {
		request        *http.Request
		deps           moderationDeps
		expectedStatus int
		expectedBody   map[string]any
		check          func(t *testing.T, body map[string]any)
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			status, body := serveJSON(t, moderationApp(t, testCase.deps), testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
			if testCase.expectedBody != nil {
				expect.Equal(t, body, testCase.expectedBody)
			}
			if testCase.check != nil {
				testCase.check(t, body)
			}
		})
	}

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodPost, "/reports", strings.NewReader(reportBody("post", testPostUUID))),
		expectedStatus: http.StatusUnauthorized,
	})
	run("ValidationFailures", testCase{
		request:        authedRequest(http.MethodPost, "/reports", `{"targetType":"video","targetUuid":"x","reason":"meh","details":"`+strings.Repeat("a", 1001)+`"}`),
		expectedStatus: http.StatusBadRequest,
		expectedBody: map[string]any{
			"error": "invalid request body", "type": "validation",
			"details": map[string]any{
				"targetType": "must be post, comment or account",
				"targetUuid": "must be a valid UUID",
				"reason":     "must be one of spam, harassment, nudity, violence, illegal, other",
				"details":    "must be at most 1000 characters",
			},
		},
	})
	run("PostNotFound", testCase{
		request: authedRequest(http.MethodPost, "/reports", reportBody("post", testPostUUID)),
		deps: moderationDeps{postStore: &postStoreMock{GetPostStub: func(context.Context, uuid.UUID, *uuid.UUID) (post.Post, error) {
			return post.Post{}, postgres.ErrPostNotFound
		}}},
		expectedStatus: http.StatusUnprocessableEntity,
		expectedBody:   invalidTarget,
	})
	run("OwnPost", testCase{
		request:        authedRequest(http.MethodPost, "/reports", reportBody("post", testPostUUID)),
		deps:           moderationDeps{postStore: &postStoreMock{GetPostStub: postBy(testAccountUUID)}},
		expectedStatus: http.StatusUnprocessableEntity,
		expectedBody:   invalidTarget,
	})
	run("OwnComment", testCase{
		request: authedRequest(http.MethodPost, "/reports", reportBody("comment", testCommentUUID)),
		deps: moderationDeps{socialStore: &socialStoreMock{GetCommentStub: func(context.Context, uuid.UUID, *uuid.UUID) (social.Comment, error) {
			return testComment(), nil // written by alice, the viewer
		}}},
		expectedStatus: http.StatusUnprocessableEntity,
		expectedBody:   invalidTarget,
	})
	run("Self", testCase{
		request: authedRequest(http.MethodPost, "/reports", reportBody("account", testAccountUUID)),
		deps: moderationDeps{accountStore: &accountStoreMock{GetUserSummaryStub: func(context.Context, uuid.UUID) (account.UserSummary, error) {
			return account.UserSummary{UUID: testAccountUUID}, nil
		}}},
		expectedStatus: http.StatusUnprocessableEntity,
		expectedBody:   invalidTarget,
	})
	run("InsertError", testCase{
		request: authedRequest(http.MethodPost, "/reports", reportBody("post", testPostUUID)),
		deps: moderationDeps{
			postStore: &postStoreMock{GetPostStub: postBy(otherAccountUUID)},
			moderationStore: &moderationStoreMock{InsertReportStub: func(context.Context, moderation.NewReport) (uuid.UUID, error) {
				return uuid.Nil, expect.ErrTest
			}},
		},
		expectedStatus: http.StatusInternalServerError,
	})
	run("Success", testCase{
		request: authedRequest(http.MethodPost, "/reports", reportBody("post", testPostUUID)),
		deps: moderationDeps{
			postStore: &postStoreMock{GetPostStub: postBy(otherAccountUUID)},
			moderationStore: &moderationStoreMock{
				InsertReportStub: func(_ context.Context, report moderation.NewReport) (uuid.UUID, error) {
					expect.Equal(t, report, moderation.NewReport{
						ReporterUUID: testAccountUUID, TargetType: "post", TargetUUID: testPostUUID, Reason: "spam", Details: "too much",
					})
					return testReportUUID, nil
				},
				GetReportStub: func(context.Context, uuid.UUID) (moderation.Report, error) {
					return moderation.Report{
						UUID: testReportUUID, Status: moderation.StatusOpen, TargetType: "post", TargetUUID: testPostUUID,
						TargetPreview: &moderation.TargetPreview{Username: "bob", Text: "secret"},
					}, nil
				},
			},
		},
		expectedStatus: http.StatusCreated,
		check: func(t *testing.T, body map[string]any) {
			_, hasPreview := body["targetPreview"]
			expect.Equal(t, hasPreview, false)
			expect.Equal[any](t, body["status"], "open")
		},
	})
}

func Test_app_getAdminReportsEndpoint(t *testing.T) {
	listing := func(expectedStatus string) func(context.Context, string, pagination.Cursor, int) ([]moderation.Report, error) {
		return func(_ context.Context, status string, _ pagination.Cursor, _ int) ([]moderation.Report, error) {
			expect.Equal(t, status, expectedStatus)
			return []moderation.Report{{
				UUID: testReportUUID, Status: status,
				TargetPreview: &moderation.TargetPreview{Username: "bob", Text: "caption", ThumbnailKey: testAvatarKey},
			}}, nil
		}
	}

	t.Run("NotModerator", func(t *testing.T) {
		status, _ := serveJSON(t, moderationApp(t, moderationDeps{}), authedRequest(http.MethodGet, "/admin/reports", ""))
		expect.Equal(t, status, http.StatusForbidden)
	})
	t.Run("InvalidStatus", func(t *testing.T) {
		a := moderationApp(t, moderationDeps{role: account.RoleModerator})
		status, _ := serveJSON(t, a, authedRequest(http.MethodGet, "/admin/reports?status=pending", ""))
		expect.Equal(t, status, http.StatusBadRequest)
	})
	t.Run("DefaultOpen", func(t *testing.T) {
		a := moderationApp(t, moderationDeps{role: account.RoleModerator, moderationStore: &moderationStoreMock{ListReportsStub: listing("open")}})
		status, _ := serveJSON(t, a, authedRequest(http.MethodGet, "/admin/reports", ""))
		expect.Equal(t, status, http.StatusOK)
	})
	t.Run("Success", func(t *testing.T) {
		a := moderationApp(t, moderationDeps{role: account.RoleAdmin, moderationStore: &moderationStoreMock{ListReportsStub: listing("resolved")}})
		status, body := serveJSON(t, a, authedRequest(http.MethodGet, "/admin/reports?status=resolved", ""))
		expect.Equal(t, status, http.StatusOK)
		items, _ := body["items"].([]any)
		item, _ := items[0].(map[string]any)
		expect.Equal[any](t, item["targetPreview"], map[string]any{
			"username": "bob", "text": "caption", "thumbnailUrl": "http://example.com/files/" + testAvatarKey,
		})
	})
}

func Test_app_resolveReportEndpoint(t *testing.T) {
	path := "/admin/reports/" + testReportUUID.String() + "/resolve"
	openReport := func(targetType string) func(context.Context, uuid.UUID) (moderation.Report, error) {
		return func(context.Context, uuid.UUID) (moderation.Report, error) {
			return moderation.Report{UUID: testReportUUID, Status: moderation.StatusOpen, TargetType: targetType, TargetUUID: testPostUUID}, nil
		}
	}
	action := func(name string) string { return `{"action":"` + name + `","note":" handled "}` }

	type testCase struct {
		request        *http.Request
		deps           moderationDeps
		expectedStatus int
		// expectedResolution is checked when ResolveReports runs; nil means it must not run.
		expectedResolution *moderation.Resolution
		expectedDeleted    []string
		expectedSuspended  *uuid.UUID
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			deps := testCase.deps
			if deps.role == "" {
				deps.role = account.RoleModerator
			}
			if deps.moderationStore == nil {
				deps.moderationStore = &moderationStoreMock{}
			}
			var resolution *moderation.Resolution
			deps.moderationStore.ResolveReportsStub = func(_ context.Context, res moderation.Resolution) error {
				resolution = &res
				return nil
			}
			var suspended *uuid.UUID
			if deps.moderationStore.SuspendAccountStub == nil {
				deps.moderationStore.SuspendAccountStub = func(_ context.Context, id uuid.UUID) error {
					suspended = &id
					return nil
				}
			}
			if deps.moderationStore.GetReportStub == nil {
				deps.moderationStore.GetReportStub = openReport(moderation.TargetPost)
			}
			var deleted []string
			deps.blobStore = &blobStoreMock{DeleteStub: func(_ context.Context, key string) error {
				deleted = append(deleted, key)
				return nil
			}}

			status, _ := serveJSON(t, moderationApp(t, deps), testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
			expect.Equal(t, resolution, testCase.expectedResolution)
			expect.Equal(t, deleted, testCase.expectedDeleted)
			expect.Equal(t, suspended, testCase.expectedSuspended)
		})
	}

	resolvedAll := func(targetType string) *moderation.Resolution {
		return &moderation.Resolution{
			TargetType: targetType, TargetUUID: testPostUUID, ModeratorUUID: testAccountUUID,
			Status: moderation.StatusResolved, Note: "handled",
		}
	}
	roleOf := func(role string) *accountStoreMock {
		return &accountStoreMock{GetAccountRoleStub: func(context.Context, uuid.UUID) (string, error) { return role, nil }}
	}
	authorIsOther := func(context.Context, string, uuid.UUID) (uuid.UUID, error) { return otherAccountUUID, nil }

	run("NotModerator", testCase{
		request:        authedRequest(http.MethodPost, path, action("dismiss")),
		deps:           moderationDeps{role: account.RoleUser},
		expectedStatus: http.StatusForbidden,
	})
	run("InvalidAction", testCase{
		request:        authedRequest(http.MethodPost, path, action("ban")),
		expectedStatus: http.StatusBadRequest,
	})
	run("NotFound", testCase{
		request: authedRequest(http.MethodPost, path, action("dismiss")),
		deps: moderationDeps{moderationStore: &moderationStoreMock{GetReportStub: func(context.Context, uuid.UUID) (moderation.Report, error) {
			return moderation.Report{}, postgres.ErrReportNotFound
		}}},
		expectedStatus: http.StatusNotFound,
	})
	run("AlreadyResolved", testCase{
		request: authedRequest(http.MethodPost, path, action("dismiss")),
		deps: moderationDeps{moderationStore: &moderationStoreMock{GetReportStub: func(context.Context, uuid.UUID) (moderation.Report, error) {
			return moderation.Report{Status: moderation.StatusResolved}, nil
		}}},
		expectedStatus: http.StatusConflict,
	})
	run("Dismiss", testCase{
		request:        authedRequest(http.MethodPost, path, action("dismiss")),
		expectedStatus: http.StatusOK,
		expectedResolution: &moderation.Resolution{
			TargetType: "post", TargetUUID: testPostUUID, ModeratorUUID: testAccountUUID,
			Status: moderation.StatusDismissed, Note: "handled", OnlyReportUUID: &testReportUUID,
		},
	})
	removed := testPost().Media
	run("RemovePost", testCase{
		request: authedRequest(http.MethodPost, path, action("removeContent")),
		deps: moderationDeps{moderationStore: &moderationStoreMock{RemovePostStub: func(_ context.Context, postUUID uuid.UUID) (media.Media, error) {
			expect.Equal(t, postUUID, testPostUUID)
			return removed, nil
		}}},
		expectedStatus:     http.StatusOK,
		expectedResolution: resolvedAll("post"),
		expectedDeleted:    []string{removed.DisplayKey, removed.ThumbnailKey},
	})
	run("RemovePostAlreadyGone", testCase{
		request: authedRequest(http.MethodPost, path, action("removeContent")),
		deps: moderationDeps{moderationStore: &moderationStoreMock{RemovePostStub: func(context.Context, uuid.UUID) (media.Media, error) {
			return media.Media{}, postgres.ErrPostNotFound
		}}},
		expectedStatus:     http.StatusOK,
		expectedResolution: resolvedAll("post"),
	})
	run("RemoveComment", testCase{
		request: authedRequest(http.MethodPost, path, action("removeContent")),
		deps: moderationDeps{moderationStore: &moderationStoreMock{
			GetReportStub:         openReport(moderation.TargetComment),
			SoftDeleteCommentStub: func(context.Context, uuid.UUID) error { return nil },
		}},
		expectedStatus:     http.StatusOK,
		expectedResolution: resolvedAll("comment"),
	})
	run("RemoveAccount", testCase{
		request:        authedRequest(http.MethodPost, path, action("removeContent")),
		deps:           moderationDeps{moderationStore: &moderationStoreMock{GetReportStub: openReport(moderation.TargetAccount)}},
		expectedStatus: http.StatusUnprocessableEntity,
	})
	run("SuspendAuthorOfPost", testCase{
		request: authedRequest(http.MethodPost, path, action("suspendAccount")),
		deps: moderationDeps{
			accountStore:    roleOf(account.RoleUser),
			moderationStore: &moderationStoreMock{GetContentAuthorStub: authorIsOther},
		},
		expectedStatus:     http.StatusOK,
		expectedResolution: resolvedAll("post"),
		expectedSuspended:  &otherAccountUUID,
	})
	run("SuspendAdmin", testCase{
		request: authedRequest(http.MethodPost, path, action("suspendAccount")),
		deps: moderationDeps{
			role:            account.RoleAdmin,
			accountStore:    roleOf(account.RoleAdmin),
			moderationStore: &moderationStoreMock{GetContentAuthorStub: authorIsOther},
		},
		expectedStatus: http.StatusForbidden,
	})
	run("ModeratorSuspendsModerator", testCase{
		request: authedRequest(http.MethodPost, path, action("suspendAccount")),
		deps: moderationDeps{
			accountStore:    roleOf(account.RoleModerator),
			moderationStore: &moderationStoreMock{GetContentAuthorStub: authorIsOther},
		},
		expectedStatus: http.StatusForbidden,
	})
	run("AdminSuspendsModerator", testCase{
		request: authedRequest(http.MethodPost, path, action("suspendAccount")),
		deps: moderationDeps{
			role:            account.RoleAdmin,
			accountStore:    roleOf(account.RoleModerator),
			moderationStore: &moderationStoreMock{GetContentAuthorStub: authorIsOther},
		},
		expectedStatus:     http.StatusOK,
		expectedResolution: resolvedAll("post"),
		expectedSuspended:  &otherAccountUUID,
	})
}

func Test_app_unsuspendEndpoint(t *testing.T) {
	path := "/admin/accounts/" + otherAccountUUID.String() + "/unsuspend"

	t.Run("NotAdmin", func(t *testing.T) {
		a := moderationApp(t, moderationDeps{role: account.RoleModerator})
		status, _ := serveJSON(t, a, authedRequest(http.MethodPost, path, ""))
		expect.Equal(t, status, http.StatusForbidden)
	})
	t.Run("NotFound", func(t *testing.T) {
		a := moderationApp(t, moderationDeps{role: account.RoleAdmin, moderationStore: &moderationStoreMock{
			UnsuspendAccountStub: func(context.Context, uuid.UUID) error { return postgres.ErrAccountNotFound },
		}})
		status, _ := serveJSON(t, a, authedRequest(http.MethodPost, path, ""))
		expect.Equal(t, status, http.StatusNotFound)
	})
	t.Run("Success", func(t *testing.T) {
		a := moderationApp(t, moderationDeps{role: account.RoleAdmin, moderationStore: &moderationStoreMock{
			UnsuspendAccountStub: func(_ context.Context, id uuid.UUID) error {
				expect.Equal(t, id, otherAccountUUID)
				return nil
			},
		}})
		status, _ := serveJSON(t, a, authedRequest(http.MethodPost, path, ""))
		expect.Equal(t, status, http.StatusNoContent)
	})
}
