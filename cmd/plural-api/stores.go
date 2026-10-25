package main

import (
	"context"
	"io"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/blob"
	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/post"
	"github.com/plural-social/plural-api/internal/social"
)

//go:generate go tool mock -o accountStore_mock.go accountStore
type accountStore interface {
	Ping(ctx context.Context) error
	InsertAccount(ctx context.Context, newAccount account.NewAccount) (account.Account, error)
	GetCredentials(ctx context.Context, login string) (account.Credentials, error)
	InsertSession(ctx context.Context, session account.NewSession) error
	GetViewer(ctx context.Context, tokenHash []byte) (account.Viewer, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
	GetMe(ctx context.Context, accountUUID uuid.UUID) (account.Me, error)
	UpdateProfile(ctx context.Context, accountUUID uuid.UUID, update account.ProfileUpdate) error
	GetProfile(ctx context.Context, username string, viewerUUID *uuid.UUID) (account.Profile, error)
	GetAccountUUID(ctx context.Context, username string) (uuid.UUID, error)
}

//go:generate go tool mock -o mediaStore_mock.go mediaStore
type mediaStore interface {
	InsertMedia(ctx context.Context, newMedia media.NewMedia) (media.Media, error)
	GetMedia(ctx context.Context, mediaUUID uuid.UUID) (media.Media, error)
}

//go:generate go tool mock -o blobStore_mock.go blobStore
type blobStore interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (*blob.Object, error)
	Delete(ctx context.Context, key string) error
}

//go:generate go tool mock -o postStore_mock.go postStore
type postStore interface {
	InsertPost(ctx context.Context, newPost post.NewPost) (uuid.UUID, error)
	GetPost(ctx context.Context, postUUID uuid.UUID, viewerUUID *uuid.UUID) (post.Post, error)
	ListUserPosts(ctx context.Context, accountUUID uuid.UUID, viewerUUID *uuid.UUID, cursor pagination.Cursor, limit int) ([]post.Post, error)
	SoftDeletePost(ctx context.Context, postUUID, accountUUID uuid.UUID) (media.Media, error)
}

//go:generate go tool mock -o socialStore_mock.go socialStore
type socialStore interface {
	Follow(ctx context.Context, followerUUID, followeeUUID uuid.UUID) error
	Unfollow(ctx context.Context, followerUUID, followeeUUID uuid.UUID) error
	ListFollowingFeed(ctx context.Context, viewerUUID uuid.UUID, cursor pagination.Cursor, limit int) ([]post.Post, error)
	ListExploreFeed(ctx context.Context, viewerUUID *uuid.UUID, cursor pagination.Cursor, limit int) ([]post.Post, error)
	LikePost(ctx context.Context, postUUID, accountUUID uuid.UUID) error
	UnlikePost(ctx context.Context, postUUID, accountUUID uuid.UUID) error
	InsertComment(ctx context.Context, newComment social.NewComment) (uuid.UUID, error)
	GetComment(ctx context.Context, commentUUID uuid.UUID) (social.Comment, error)
	ListComments(ctx context.Context, postUUID uuid.UUID, cursor pagination.Cursor, limit int) ([]social.Comment, error)
	SoftDeleteComment(ctx context.Context, commentUUID uuid.UUID) error
}
