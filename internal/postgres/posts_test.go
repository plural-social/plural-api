package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"

	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/post"
)

// insertTestMedia stores a media row owned by owner with the given status.
func insertTestMedia(t *testing.T, store *Store, owner uuid.UUID, status string) media.Media {
	t.Helper()
	id := uuid.New()
	inserted, err := store.InsertMedia(context.Background(), media.NewMedia{
		UUID: id, AccountUUID: owner, Kind: media.KindImage, Status: status, ContentType: "image/jpeg",
		DisplayKey: media.DisplayKey(id), ThumbnailKey: media.ThumbnailKey(id), Width: 10, Height: 10,
	})
	expect.ErrorNil(t, err)
	return inserted
}

// insertTestPost creates a post by owner on fresh media of the given status.
func insertTestPost(t *testing.T, store *Store, owner uuid.UUID, status string) uuid.UUID {
	t.Helper()
	m := insertTestMedia(t, store, owner, status)
	postUUID, err := store.InsertPost(context.Background(), post.NewPost{AccountUUID: owner, MediaUUID: m.UUID, Caption: "c"})
	expect.ErrorNil(t, err)
	return postUUID
}

func Test_Store_posts(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	author, other := insertTestAccount(t, store), insertTestAccount(t, store)

	t.Run("InsertGetAndMediaReuse", func(t *testing.T) {
		m := insertTestMedia(t, store, author.UUID, media.StatusReady)
		postUUID, err := store.InsertPost(ctx, post.NewPost{AccountUUID: author.UUID, MediaUUID: m.UUID, Caption: "hello"})
		expect.ErrorNil(t, err)

		got, err := store.GetPost(ctx, postUUID, nil)
		expect.ErrorNil(t, err)
		expect.Equal(t, got.Caption, "hello")
		expect.Equal(t, got.Author.Username, author.Username)
		expect.Equal(t, got.Media.UUID, m.UUID)

		_, err = store.InsertPost(ctx, post.NewPost{AccountUUID: author.UUID, MediaUUID: m.UUID})
		expect.ErrorIs(ErrMediaAlreadyUsed)(t, err)
	})

	t.Run("ProcessingVisibleOnlyToAuthor", func(t *testing.T) {
		postUUID := insertTestPost(t, store, author.UUID, media.StatusProcessing)

		_, err := store.GetPost(ctx, postUUID, &author.UUID)
		expect.ErrorNil(t, err)
		_, err = store.GetPost(ctx, postUUID, &other.UUID)
		expect.ErrorIs(ErrPostNotFound)(t, err)
		_, err = store.GetPost(ctx, postUUID, nil)
		expect.ErrorIs(ErrPostNotFound)(t, err)
	})

	t.Run("ListPagination", func(t *testing.T) {
		lister := insertTestAccount(t, store)
		for range 3 {
			insertTestPost(t, store, lister.UUID, media.StatusReady)
		}

		rows, err := store.ListUserPosts(ctx, lister.UUID, nil, pagination.Cursor{}, 2)
		expect.ErrorNil(t, err)
		expect.Equal(t, len(rows), 3) // limit+1
		expect.Equal(t, rows[0].CreatedAt.After(rows[1].CreatedAt) || rows[0].CreatedAt.Equal(rows[1].CreatedAt), true)

		cursor := pagination.Cursor{CreatedAt: rows[1].CreatedAt, UUID: rows[1].UUID}
		next, err := store.ListUserPosts(ctx, lister.UUID, nil, cursor, 2)
		expect.ErrorNil(t, err)
		expect.Equal(t, len(next), 1)
		expect.Equal(t, next[0].UUID, rows[2].UUID)
	})

	t.Run("DeleteAndProfileCount", func(t *testing.T) {
		owner := insertTestAccount(t, store)
		kept := insertTestPost(t, store, owner.UUID, media.StatusReady)
		deleted := insertTestPost(t, store, owner.UUID, media.StatusReady)
		insertTestPost(t, store, owner.UUID, media.StatusProcessing)

		_, err := store.SoftDeletePost(ctx, deleted, other.UUID)
		expect.ErrorIs(ErrPostNotFound)(t, err)

		m, err := store.SoftDeletePost(ctx, deleted, owner.UUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, m.DisplayKey != "", true)
		_, err = store.GetPost(ctx, deleted, &owner.UUID)
		expect.ErrorIs(ErrPostNotFound)(t, err)
		_, err = store.GetPost(ctx, kept, nil)
		expect.ErrorNil(t, err)

		// Viewers see ready posts only; the author also sees the processing one.
		for _, testCase := range []struct {
			viewer   *uuid.UUID
			expected int
		}{{nil, 1}, {&other.UUID, 1}, {&owner.UUID, 2}} {
			profile, err := store.GetProfile(ctx, owner.Username, testCase.viewer)
			expect.ErrorNil(t, err)
			expect.Equal(t, profile.PostCount, testCase.expected)
		}
	})
}
