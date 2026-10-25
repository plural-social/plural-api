package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"

	"github.com/plural-social/plural-api/internal/media"
)

func Test_Store_likes(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	author, liker, other := insertTestAccount(t, store), insertTestAccount(t, store), insertTestAccount(t, store)
	postUUID := insertTestPost(t, store, author.UUID, media.StatusReady)

	expect.ErrorNil(t, store.LikePost(ctx, postUUID, liker.UUID))
	expect.ErrorNil(t, store.LikePost(ctx, postUUID, liker.UUID)) // idempotent

	t.Run("CountsAndLikedByMe", func(t *testing.T) {
		for _, testCase := range []struct {
			viewer   *uuid.UUID
			expected bool
		}{{&liker.UUID, true}, {&other.UUID, false}, {nil, false}} {
			got, err := store.GetPost(ctx, postUUID, testCase.viewer)
			expect.ErrorNil(t, err)
			expect.Equal(t, got.LikeCount, 1)
			expect.Equal(t, got.LikedByMe, testCase.expected)
		}
	})
	t.Run("Unlike", func(t *testing.T) {
		expect.ErrorNil(t, store.UnlikePost(ctx, postUUID, liker.UUID))
		expect.ErrorNil(t, store.UnlikePost(ctx, postUUID, liker.UUID)) // idempotent
		got, err := store.GetPost(ctx, postUUID, &liker.UUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, got.LikeCount, 0)
		expect.Equal(t, got.LikedByMe, false)
	})
	t.Run("MissingPost", func(t *testing.T) {
		expect.ErrorIs(ErrPostNotFound)(t, store.LikePost(ctx, uuid.New(), liker.UUID))
	})
}
