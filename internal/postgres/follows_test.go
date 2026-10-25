package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"
)

func Test_Store_follows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	follower, followee, stranger := insertTestAccount(t, store), insertTestAccount(t, store), insertTestAccount(t, store)

	expect.ErrorNil(t, store.Follow(ctx, follower.UUID, followee.UUID))
	expect.ErrorNil(t, store.Follow(ctx, follower.UUID, followee.UUID)) // idempotent

	t.Run("ProfileCounts", func(t *testing.T) {
		profile, err := store.GetProfile(ctx, followee.Username, &follower.UUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, profile.FollowerCount, 1)
		other, err := store.GetProfile(ctx, follower.Username, nil)
		expect.ErrorNil(t, err)
		expect.Equal(t, other.FollowingCount, 1)
	})
	t.Run("IsFollowing", func(t *testing.T) {
		for _, testCase := range []struct {
			viewer   *uuid.UUID
			expected bool
		}{{&follower.UUID, true}, {&stranger.UUID, false}, {nil, false}} {
			profile, err := store.GetProfile(ctx, followee.Username, testCase.viewer)
			expect.ErrorNil(t, err)
			expect.Equal(t, profile.IsFollowing, testCase.expected)
		}
	})
	t.Run("Unfollow", func(t *testing.T) {
		expect.ErrorNil(t, store.Unfollow(ctx, follower.UUID, followee.UUID))
		expect.ErrorNil(t, store.Unfollow(ctx, follower.UUID, followee.UUID)) // idempotent
		profile, err := store.GetProfile(ctx, followee.Username, &follower.UUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, profile.FollowerCount, 0)
		expect.Equal(t, profile.IsFollowing, false)
	})
	t.Run("FollowSelfViolatesCheck", func(t *testing.T) {
		expect.ErrorNonNil(t, store.Follow(ctx, follower.UUID, follower.UUID))
	})
	t.Run("GetAccountUUID", func(t *testing.T) {
		got, err := store.GetAccountUUID(ctx, follower.Username)
		expect.ErrorNil(t, err)
		expect.Equal(t, got, follower.UUID)
		_, err = store.GetAccountUUID(ctx, uniqueName(t))
		expect.ErrorIs(ErrAccountNotFound)(t, err)
	})
}
