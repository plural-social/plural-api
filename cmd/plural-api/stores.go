package main

import (
	"context"
	"io"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/blob"
	"github.com/plural-social/plural-api/internal/media"
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
