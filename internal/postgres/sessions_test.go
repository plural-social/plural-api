package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/nicheinc/expect"

	"github.com/plural-social/plural-api/internal/account"
)

func Test_Store_sessions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	username := uniqueName(t)
	email := username + "@example.com"
	created, err := store.InsertAccount(ctx, account.NewAccount{Username: username, Email: email, PasswordHash: "hash"})
	expect.ErrorNil(t, err)

	newSession := func(t *testing.T, expiresAt time.Time) []byte {
		t.Helper()
		_, hash, err := account.NewSessionToken()
		expect.ErrorNil(t, err)
		expect.ErrorNil(t, store.InsertSession(ctx, account.NewSession{
			TokenHash: hash, AccountUUID: created.UUID, UserAgent: "test", ExpiresAt: expiresAt,
		}))
		return hash
	}

	t.Run("UnknownLogin", func(t *testing.T) {
		_, err := store.GetCredentials(ctx, uniqueName(t))
		expect.ErrorIs(ErrAccountNotFound)(t, err)
	})
	t.Run("CredentialsByUsernameAndEmail", func(t *testing.T) {
		for _, login := range []string{username, email} {
			credentials, err := store.GetCredentials(ctx, login)
			expect.ErrorNil(t, err)
			expect.Equal(t, credentials, account.Credentials{AccountUUID: created.UUID, PasswordHash: "hash"})
		}
	})
	t.Run("ExpiredSession", func(t *testing.T) {
		hash := newSession(t, time.Now().Add(-time.Hour))
		_, err := store.GetViewer(ctx, hash)
		expect.ErrorIs(ErrSessionNotFound)(t, err)
	})
	t.Run("GetViewerAndDelete", func(t *testing.T) {
		hash := newSession(t, time.Now().Add(time.Hour))
		viewer, err := store.GetViewer(ctx, hash)
		expect.ErrorNil(t, err)
		expect.Equal(t, viewer, account.Viewer{UUID: created.UUID, Username: username, Role: account.RoleUser})

		expect.ErrorNil(t, store.DeleteSession(ctx, hash))
		expect.ErrorNil(t, store.DeleteSession(ctx, hash))
		_, err = store.GetViewer(ctx, hash)
		expect.ErrorIs(ErrSessionNotFound)(t, err)
	})
	t.Run("GetMe", func(t *testing.T) {
		me, err := store.GetMe(ctx, created.UUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, me, account.Me{
			UUID: created.UUID, Username: username, Email: email, Role: account.RoleUser, CreatedAt: created.CreatedAt.UTC(),
		})
	})
}
