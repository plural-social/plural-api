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
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/post"
	"github.com/plural-social/plural-api/internal/postgres"
	"github.com/plural-social/plural-api/internal/social"
)

var testCommentUUID = uuid.MustParse("9c8d7e6f-5a4b-4c3d-8e2f-1a0b9c8d7e6f")

var otherAccountUUID = uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

// testComment is a comment by alice on a post by alice.
func testComment() social.Comment {
	return social.Comment{
		UUID: testCommentUUID, PostUUID: testPostUUID, Body: "nice one", CreatedAt: testCreatedAt,
		Author:         account.UserSummary{UUID: testAccountUUID, Username: "alice"},
		PostAuthorUUID: testAccountUUID,
	}
}

func postFound(context.Context, uuid.UUID, *uuid.UUID) (post.Post, error) { return testPost(), nil }

func postMissing(context.Context, uuid.UUID, *uuid.UUID) (post.Post, error) {
	return post.Post{}, postgres.ErrPostNotFound
}

func Test_app_getCommentsEndpoint(t *testing.T) {
	path := "/posts/" + testPostUUID.String() + "/comments"

	type testCase struct {
		path           string
		postStore      *postStoreMock
		socialStore    *socialStoreMock
		expectedStatus int
		check          func(t *testing.T, body map[string]any)
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.postStore.T = t
			testCase.socialStore.T = t
			a := &app{postStore: testCase.postStore, socialStore: testCase.socialStore, logger: zap.NewNop()}
			status, body := serveJSON(t, a, httptest.NewRequest(http.MethodGet, testCase.path, nil))
			expect.Equal(t, status, testCase.expectedStatus)
			if testCase.check != nil {
				testCase.check(t, body)
			}
		})
	}

	run("InvalidUUID", testCase{
		path: "/posts/nope/comments", postStore: &postStoreMock{}, socialStore: &socialStoreMock{},
		expectedStatus: http.StatusNotFound,
	})
	run("InvalidCursor", testCase{
		path: path + "?cursor=%21%21", postStore: &postStoreMock{}, socialStore: &socialStoreMock{},
		expectedStatus: http.StatusBadRequest,
	})
	run("PostNotFound", testCase{
		path: path, postStore: &postStoreMock{GetPostStub: postMissing}, socialStore: &socialStoreMock{},
		expectedStatus: http.StatusNotFound,
	})
	run("ListError", testCase{
		path:      path,
		postStore: &postStoreMock{GetPostStub: postFound},
		socialStore: &socialStoreMock{ListCommentsStub: func(context.Context, uuid.UUID, pagination.Cursor, int) ([]social.Comment, error) {
			return nil, expect.ErrTest
		}},
		expectedStatus: http.StatusInternalServerError,
	})
	run("Success", testCase{
		path:      path + "?limit=1",
		postStore: &postStoreMock{GetPostStub: postFound},
		socialStore: &socialStoreMock{ListCommentsStub: func(_ context.Context, postUUID uuid.UUID, _ pagination.Cursor, limit int) ([]social.Comment, error) {
			expect.Equal(t, postUUID, testPostUUID)
			expect.Equal(t, limit, 1)
			return []social.Comment{testComment(), testComment()}, nil
		}},
		expectedStatus: http.StatusOK,
		check: func(t *testing.T, body map[string]any) {
			items, _ := body["items"].([]any)
			expect.Equal(t, len(items), 1)
			item, _ := items[0].(map[string]any)
			_, leaked := item["postAuthorUuid"]
			expect.Equal(t, leaked, false)
			expect.Equal[any](t, item["body"], "nice one")
			expect.Equal[any](t, body["nextCursor"], pagination.Encode(pagination.Cursor{CreatedAt: testCreatedAt, UUID: testCommentUUID}))
		},
	})
}

func Test_app_postCommentEndpoint(t *testing.T) {
	path := "/posts/" + testPostUUID.String() + "/comments"
	validationError := map[string]any{
		"error": "invalid request body", "type": "validation",
		"details": map[string]any{"body": "must be 1-500 characters"},
	}

	type testCase struct {
		request        *http.Request
		postStore      *postStoreMock
		socialStore    *socialStoreMock
		expectedStatus int
		expectedBody   map[string]any
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.postStore.T = t
			testCase.socialStore.T = t
			a := &app{
				accountStore: &accountStoreMock{T: t, GetViewerStub: viewerStub},
				postStore:    testCase.postStore,
				socialStore:  testCase.socialStore,
				logger:       zap.NewNop(),
			}
			status, body := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
			if testCase.expectedBody != nil {
				expect.Equal(t, body, testCase.expectedBody)
			}
		})
	}

	run("Unauthenticated", testCase{
		request:   httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"body":"hi"}`)),
		postStore: &postStoreMock{}, socialStore: &socialStoreMock{},
		expectedStatus: http.StatusUnauthorized,
	})
	run("MalformedJSON", testCase{
		request:   authedRequest(http.MethodPost, path, `{"body":`),
		postStore: &postStoreMock{}, socialStore: &socialStoreMock{},
		expectedStatus: http.StatusBadRequest,
	})
	run("EmptyBody", testCase{
		request:   authedRequest(http.MethodPost, path, `{"body":"   "}`),
		postStore: &postStoreMock{}, socialStore: &socialStoreMock{},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   validationError,
	})
	run("TooLong", testCase{
		request:   authedRequest(http.MethodPost, path, `{"body":"`+strings.Repeat("é", 501)+`"}`),
		postStore: &postStoreMock{}, socialStore: &socialStoreMock{},
		expectedStatus: http.StatusBadRequest,
		expectedBody:   validationError,
	})
	run("PostNotFound", testCase{
		request:   authedRequest(http.MethodPost, path, `{"body":"hi"}`),
		postStore: &postStoreMock{GetPostStub: postMissing}, socialStore: &socialStoreMock{},
		expectedStatus: http.StatusNotFound,
	})
	run("InsertError", testCase{
		request:   authedRequest(http.MethodPost, path, `{"body":"hi"}`),
		postStore: &postStoreMock{GetPostStub: postFound},
		socialStore: &socialStoreMock{InsertCommentStub: func(context.Context, social.NewComment) (uuid.UUID, error) {
			return uuid.Nil, expect.ErrTest
		}},
		expectedStatus: http.StatusInternalServerError,
	})
	run("Success", testCase{
		request:   authedRequest(http.MethodPost, path, `{"body":"  nice one "}`),
		postStore: &postStoreMock{GetPostStub: postFound},
		socialStore: &socialStoreMock{
			InsertCommentStub: func(_ context.Context, newComment social.NewComment) (uuid.UUID, error) {
				expect.Equal(t, newComment, social.NewComment{PostUUID: testPostUUID, AccountUUID: testAccountUUID, Body: "nice one"})
				return testCommentUUID, nil
			},
			GetCommentStub: func(context.Context, uuid.UUID) (social.Comment, error) { return testComment(), nil },
		},
		expectedStatus: http.StatusCreated,
		expectedBody: map[string]any{
			"uuid": testCommentUUID.String(), "postUuid": testPostUUID.String(), "body": "nice one",
			"createdAt": "2026-10-01T08:00:00Z",
			"author":    map[string]any{"uuid": testAccountUUID.String(), "username": "alice", "displayName": ""},
		},
	})
}

func Test_app_deleteCommentEndpoint(t *testing.T) {
	path := "/comments/" + testCommentUUID.String()
	commentBy := func(author, postAuthor uuid.UUID) func(context.Context, uuid.UUID) (social.Comment, error) {
		return func(context.Context, uuid.UUID) (social.Comment, error) {
			c := testComment()
			c.Author.UUID, c.PostAuthorUUID = author, postAuthor
			return c, nil
		}
	}
	deleteOK := func(_ context.Context, commentUUID uuid.UUID) error {
		expect.Equal(t, commentUUID, testCommentUUID)
		return nil
	}

	type testCase struct {
		request        *http.Request
		socialStore    *socialStoreMock
		expectedStatus int
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			testCase.socialStore.T = t
			a := &app{
				accountStore: &accountStoreMock{T: t, GetViewerStub: viewerStub},
				socialStore:  testCase.socialStore,
				logger:       zap.NewNop(),
			}
			status, _ := serveJSON(t, a, testCase.request)
			expect.Equal(t, status, testCase.expectedStatus)
		})
	}

	run("Unauthenticated", testCase{
		request:        httptest.NewRequest(http.MethodDelete, path, nil),
		socialStore:    &socialStoreMock{},
		expectedStatus: http.StatusUnauthorized,
	})
	run("NotFound", testCase{
		request: authedRequest(http.MethodDelete, path, ""),
		socialStore: &socialStoreMock{GetCommentStub: func(context.Context, uuid.UUID) (social.Comment, error) {
			return social.Comment{}, postgres.ErrCommentNotFound
		}},
		expectedStatus: http.StatusNotFound,
	})
	run("Forbidden", testCase{
		request:        authedRequest(http.MethodDelete, path, ""),
		socialStore:    &socialStoreMock{GetCommentStub: commentBy(otherAccountUUID, otherAccountUUID)},
		expectedStatus: http.StatusForbidden,
	})
	run("ByCommentAuthor", testCase{
		request: authedRequest(http.MethodDelete, path, ""),
		socialStore: &socialStoreMock{
			GetCommentStub:        commentBy(testAccountUUID, otherAccountUUID),
			SoftDeleteCommentStub: deleteOK,
		},
		expectedStatus: http.StatusNoContent,
	})
	run("ByPostAuthor", testCase{
		request: authedRequest(http.MethodDelete, path, ""),
		socialStore: &socialStoreMock{
			GetCommentStub:        commentBy(otherAccountUUID, testAccountUUID),
			SoftDeleteCommentStub: deleteOK,
		},
		expectedStatus: http.StatusNoContent,
	})
	run("DeleteError", testCase{
		request: authedRequest(http.MethodDelete, path, ""),
		socialStore: &socialStoreMock{
			GetCommentStub:        commentBy(testAccountUUID, testAccountUUID),
			SoftDeleteCommentStub: func(context.Context, uuid.UUID) error { return expect.ErrTest },
		},
		expectedStatus: http.StatusInternalServerError,
	})
}
