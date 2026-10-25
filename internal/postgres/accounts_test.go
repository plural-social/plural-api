package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"

	"github.com/plural-social/plural-api/internal/account"
)

// insertTestAccount creates an account with a unique username.
func insertTestAccount(t *testing.T, store *Store) account.Account {
	t.Helper()
	username := uniqueName(t)
	created, err := store.InsertAccount(context.Background(), account.NewAccount{
		Username:     username,
		Email:        username + "@example.com",
		PasswordHash: "hash",
	})
	expect.ErrorNil(t, err)
	return created
}

func Test_Store_profile(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	created := insertTestAccount(t, store)

	expect.ErrorNil(t, store.UpdateProfile(ctx, created.UUID, account.ProfileUpdate{DisplayName: new("Bob B")}))
	expect.ErrorNil(t, store.UpdateProfile(ctx, created.UUID, account.ProfileUpdate{Bio: new("hi")}))

	t.Run("UnknownAccount", func(t *testing.T) {
		err := store.UpdateProfile(ctx, uuid.New(), account.ProfileUpdate{Bio: new("x")})
		expect.ErrorIs(ErrAccountNotFound)(t, err)
	})
	t.Run("OnlyBioKeepsDisplayName", func(t *testing.T) {
		profile, err := store.GetProfile(ctx, created.Username, nil)
		expect.ErrorNil(t, err)
		expect.Equal(t, profile.DisplayName, "Bob B")
		expect.Equal(t, profile.Bio, "hi")
	})
	t.Run("IsSelf", func(t *testing.T) {
		other := uuid.New()
		for _, testCase := range []struct {
			viewer   *uuid.UUID
			expected bool
		}{{&created.UUID, true}, {&other, false}, {nil, false}} {
			profile, err := store.GetProfile(ctx, created.Username, testCase.viewer)
			expect.ErrorNil(t, err)
			expect.Equal(t, profile.IsSelf, testCase.expected)
		}
	})
}
