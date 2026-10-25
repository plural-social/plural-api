// Package social holds follows, likes and comments.
package social

import (
	"time"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/account"
)

const (
	ErrTypeCannotFollowSelf = "cannotFollowSelf"

	CommentMaxLength = 500
)

type CreateCommentRequest struct {
	Body string `json:"body"`
}

type NewComment struct {
	PostUUID    uuid.UUID
	AccountUUID uuid.UUID
	Body        string
}

// Comment is a stored comment with its author. JSON matches type Comment in
// 01-architecture.md. PostAuthorUUID is used for delete permission and is not
// sent to clients.
type Comment struct {
	UUID           uuid.UUID           `json:"uuid"`
	PostUUID       uuid.UUID           `json:"postUuid"`
	Author         account.UserSummary `json:"author"`
	Body           string              `json:"body"`
	CreatedAt      time.Time           `json:"createdAt"`
	PostAuthorUUID uuid.UUID           `json:"-"`
}
