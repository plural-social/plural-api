// Package post holds the post domain: one media item plus a caption,
// published by an account.
package post

import (
	"time"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/media"
)

const (
	CaptionMaxLength = 2200

	ErrTypeMediaNotFound    = "mediaNotFound"
	ErrTypeMediaFailed      = "mediaFailed"
	ErrTypeMediaAlreadyUsed = "mediaAlreadyUsed"
)

type CreatePostRequest struct {
	MediaUUID string `json:"mediaUuid"`
	Caption   string `json:"caption"`
}

type NewPost struct {
	AccountUUID uuid.UUID
	MediaUUID   uuid.UUID
	Caption     string
}

// Post is a stored post joined with its author and media.
type Post struct {
	UUID         uuid.UUID
	Caption      string
	CreatedAt    time.Time
	Author       account.UserSummary
	Media        media.Media
	LikeCount    int  // 0 until F09
	CommentCount int  // 0 until F09
	LikedByMe    bool // false until F09
}

// Response is the JSON shape (type Post in 01-architecture.md).
type Response struct {
	UUID         uuid.UUID           `json:"uuid"`
	Author       account.UserSummary `json:"author"`
	Caption      string              `json:"caption"`
	Media        media.Response      `json:"media"`
	LikeCount    int                 `json:"likeCount"`
	CommentCount int                 `json:"commentCount"`
	LikedByMe    bool                `json:"likedByMe"`
	CreatedAt    time.Time           `json:"createdAt"`
}
