package main

// Fixtures and helpers shared by the handler tests.

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

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/post"
)

var (
	testAccountUUID = uuid.MustParse("0bc5b194-85ce-4e49-96be-0c91916e90ac")
	testNow         = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	testCreatedAt   = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	testMe          = account.Me{
		UserSummary: account.UserSummary{UUID: testAccountUUID, Username: "alice"},
		Email:       "alice@example.com",
		Role:        account.RoleUser,
		CreatedAt:   testCreatedAt,
	}
	// testMeJSON is testMe as a decoded response body.
	testMeJSON = map[string]any{
		"uuid":        testAccountUUID.String(),
		"username":    "alice",
		"displayName": "",
		"email":       "alice@example.com",
		"bio":         "",
		"role":        "user",
		"createdAt":   "2026-10-01T08:00:00Z",
	}
)

// viewerStub resolves any token to alice.
func viewerStub(context.Context, []byte) (account.Viewer, error) {
	return account.Viewer{UUID: testAccountUUID, Username: "alice", Role: account.RoleUser}, nil
}

// serveJSON runs one request through the full router and decodes the JSON body.
func serveJSON(t *testing.T, a *app, request *http.Request) (int, map[string]any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	a.routes().ServeHTTP(recorder, request)
	var body map[string]any
	if recorder.Body.Len() > 0 {
		expect.ErrorNil(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	}
	return recorder.Code, body
}

// authedRequest builds a request carrying a bearer token resolved by viewerStub.
func authedRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer abc")
	return request
}

var testPostUUID = uuid.MustParse("7a2b9f10-3c4d-4e5f-8a6b-1c2d3e4f5a6b")

// testPost is a ready image post by alice.
func testPost() post.Post {
	return post.Post{
		UUID:      testPostUUID,
		Caption:   "first post",
		CreatedAt: testCreatedAt,
		Author:    account.UserSummary{UUID: testAccountUUID, Username: "alice"},
		Media: media.Media{
			UUID: testMediaUUID, AccountUUID: testAccountUUID, Kind: media.KindImage, Status: media.StatusReady,
			DisplayKey: media.DisplayKey(testMediaUUID), ThumbnailKey: media.ThumbnailKey(testMediaUUID),
			Width: 40, Height: 20,
		},
	}
}

// testPostJSON is testPost as a decoded response body.
func testPostJSON() map[string]any {
	return map[string]any{
		"uuid":         testPostUUID.String(),
		"author":       map[string]any{"uuid": testAccountUUID.String(), "username": "alice", "displayName": ""},
		"caption":      "first post",
		"likeCount":    0.0,
		"commentCount": 0.0,
		"likedByMe":    false,
		"createdAt":    "2026-10-01T08:00:00Z",
		"media": map[string]any{
			"uuid":         testMediaUUID.String(),
			"kind":         "image",
			"status":       "ready",
			"url":          "http://example.com/files/" + media.DisplayKey(testMediaUUID),
			"thumbnailUrl": "http://example.com/files/" + media.ThumbnailKey(testMediaUUID),
			"width":        40.0,
			"height":       20.0,
			"durationMs":   0.0,
		},
	}
}
