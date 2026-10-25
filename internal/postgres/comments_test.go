package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"

	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/social"
)

func Test_Store_comments(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	author, commenter := insertTestAccount(t, store), insertTestAccount(t, store)
	postUUID := insertTestPost(t, store, author.UUID, media.StatusReady)

	var ids []uuid.UUID // oldest first
	for _, body := range []string{"one", "two", "three"} {
		id, err := store.InsertComment(ctx, social.NewComment{PostUUID: postUUID, AccountUUID: commenter.UUID, Body: body})
		expect.ErrorNil(t, err)
		ids = append(ids, id)
	}

	postCommentCount := func(t *testing.T) int {
		t.Helper()
		got, err := store.GetPost(ctx, postUUID, nil)
		expect.ErrorNil(t, err)
		return got.CommentCount
	}

	t.Run("MissingPost", func(t *testing.T) {
		_, err := store.InsertComment(ctx, social.NewComment{PostUUID: uuid.New(), AccountUUID: commenter.UUID, Body: "x"})
		expect.ErrorIs(ErrPostNotFound)(t, err)
	})
	t.Run("CommentCount", func(t *testing.T) {
		expect.Equal(t, postCommentCount(t), 3)
	})
	t.Run("GetCommentSetsPostAuthor", func(t *testing.T) {
		got, err := store.GetComment(ctx, ids[0])
		expect.ErrorNil(t, err)
		expect.Equal(t, got.PostAuthorUUID, author.UUID)
		expect.Equal(t, got.Author.UUID, commenter.UUID)
		_, err = store.GetComment(ctx, uuid.New())
		expect.ErrorIs(ErrCommentNotFound)(t, err)
	})
	t.Run("ListPagination", func(t *testing.T) {
		rows, err := store.ListComments(ctx, postUUID, pagination.Cursor{}, 2)
		expect.ErrorNil(t, err)
		expect.Equal(t, len(rows), 3) // limit+1
		expect.Equal(t, rows[0].UUID, ids[2])

		next, err := store.ListComments(ctx, postUUID, pagination.Cursor{CreatedAt: rows[1].CreatedAt, UUID: rows[1].UUID}, 2)
		expect.ErrorNil(t, err)
		expect.Equal(t, len(next), 1)
		expect.Equal(t, next[0].UUID, ids[0])
	})
	t.Run("SoftDelete", func(t *testing.T) {
		expect.ErrorNil(t, store.SoftDeleteComment(ctx, ids[1]))
		expect.ErrorIs(ErrCommentNotFound)(t, store.SoftDeleteComment(ctx, ids[1]))
		rows, err := store.ListComments(ctx, postUUID, pagination.Cursor{}, 10)
		expect.ErrorNil(t, err)
		expect.Equal(t, len(rows), 2)
		expect.Equal(t, postCommentCount(t), 2)
	})
}
