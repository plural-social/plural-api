package postgres

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"

	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/post"
)

// postUUIDs returns the UUIDs of posts, keeping only those in mine.
// The explore feed spans the whole database, so tests filter to their own posts.
func postUUIDs(posts []post.Post, mine ...uuid.UUID) []uuid.UUID {
	var found []uuid.UUID
	for _, p := range posts {
		if slices.Contains(mine, p.UUID) {
			found = append(found, p.UUID)
		}
	}
	return found
}

func Test_Store_feeds(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	a, b, c := insertTestAccount(t, store), insertTestAccount(t, store), insertTestAccount(t, store)
	expect.ErrorNil(t, store.Follow(ctx, a.UUID, b.UUID))

	// Insert order is creation order, so newest first is the reverse.
	a1 := insertTestPost(t, store, a.UUID, media.StatusReady)
	b1 := insertTestPost(t, store, b.UUID, media.StatusReady)
	c1 := insertTestPost(t, store, c.UUID, media.StatusReady)
	b2 := insertTestPost(t, store, b.UUID, media.StatusProcessing)
	mine := []uuid.UUID{a1, b1, c1, b2}

	t.Run("FollowingFeed", func(t *testing.T) {
		rows, err := store.ListFollowingFeed(ctx, a.UUID, pagination.Cursor{}, 30)
		expect.ErrorNil(t, err)
		expect.Equal(t, postUUIDs(rows, mine...), []uuid.UUID{b1, a1})
	})
	t.Run("ExploreFeed", func(t *testing.T) {
		rows, err := store.ListExploreFeed(ctx, nil, pagination.Cursor{}, 1000)
		expect.ErrorNil(t, err)
		expect.Equal(t, postUUIDs(rows, mine...), []uuid.UUID{c1, b1, a1})
	})
	t.Run("PaginationWalksAllWithoutRepeats", func(t *testing.T) {
		var seen []uuid.UUID
		var cursor pagination.Cursor
		for range 10 {
			rows, err := store.ListFollowingFeed(ctx, a.UUID, cursor, 1)
			expect.ErrorNil(t, err)
			if len(rows) == 0 {
				break
			}
			seen = append(seen, rows[0].UUID)
			if len(rows) == 1 {
				break
			}
			cursor = pagination.Cursor{CreatedAt: rows[0].CreatedAt, UUID: rows[0].UUID}
		}
		expect.Equal(t, seen, []uuid.UUID{b1, a1})
	})
	t.Run("DeletedPostsExcluded", func(t *testing.T) {
		_, err := store.SoftDeletePost(ctx, b1, b.UUID)
		expect.ErrorNil(t, err)
		rows, err := store.ListFollowingFeed(ctx, a.UUID, pagination.Cursor{}, 30)
		expect.ErrorNil(t, err)
		expect.Equal(t, postUUIDs(rows, mine...), []uuid.UUID{a1})
	})
}
