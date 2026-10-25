package main

import (
	"context"

	"github.com/plural-social/plural-api/internal/account"
)

//go:generate go tool mock -o accountStore_mock.go accountStore
type accountStore interface {
	Ping(ctx context.Context) error
	InsertAccount(ctx context.Context, newAccount account.NewAccount) (account.Account, error)
}
