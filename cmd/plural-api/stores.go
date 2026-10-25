package main

import (
	"context"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/account"
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
