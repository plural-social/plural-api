package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/post"
	"github.com/plural-social/plural-api/internal/social"
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

func Test_Store_avatar(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	owner, other := insertTestAccount(t, store), insertTestAccount(t, store)

	insertMedia := func(t *testing.T, ownerUUID uuid.UUID, kind, status string) media.Media {
		t.Helper()
		id := uuid.New()
		m, err := store.InsertMedia(ctx, media.NewMedia{
			UUID: id, AccountUUID: ownerUUID, Kind: kind, Status: status, ContentType: "image/jpeg",
			DisplayKey: media.DisplayKey(id), ThumbnailKey: media.ThumbnailKey(id), Width: 10, Height: 10,
		})
		expect.ErrorNil(t, err)
		return m
	}

	t.Run("InvalidMedia", func(t *testing.T) {
		usedByPost := insertMedia(t, owner.UUID, media.KindImage, media.StatusReady)
		_, err := store.InsertPost(ctx, post.NewPost{AccountUUID: owner.UUID, MediaUUID: usedByPost.UUID})
		expect.ErrorNil(t, err)

		for name, m := range map[string]media.Media{
			"OtherUsersMedia": insertMedia(t, other.UUID, media.KindImage, media.StatusReady),
			"Video":           insertMedia(t, owner.UUID, media.KindVideo, media.StatusReady),
			"Processing":      insertMedia(t, owner.UUID, media.KindImage, media.StatusProcessing),
			"UsedByPost":      usedByPost,
		} {
			t.Run(name, func(t *testing.T) {
				expect.ErrorIs(ErrInvalidAvatarMedia)(t, store.SetAvatar(ctx, owner.UUID, m.UUID))
			})
		}
	})

	avatar := insertMedia(t, owner.UUID, media.KindImage, media.StatusReady)
	expect.ErrorNil(t, store.SetAvatar(ctx, owner.UUID, avatar.UUID))

	t.Run("SetAvatarShowsEverywhere", func(t *testing.T) {
		me, err := store.GetMe(ctx, owner.UUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, me.AvatarKey, avatar.ThumbnailKey)

		profile, err := store.GetProfile(ctx, owner.Username, nil)
		expect.ErrorNil(t, err)
		expect.Equal(t, profile.AvatarKey, avatar.ThumbnailKey)

		postUUID := insertTestPost(t, store, owner.UUID, media.StatusReady)
		got, err := store.GetPost(ctx, postUUID, nil)
		expect.ErrorNil(t, err)
		expect.Equal(t, got.Author.AvatarKey, avatar.ThumbnailKey)

		commentUUID, err := store.InsertComment(ctx, social.NewComment{PostUUID: postUUID, AccountUUID: owner.UUID, Body: "hi"})
		expect.ErrorNil(t, err)
		comment, err := store.GetComment(ctx, commentUUID, nil)
		expect.ErrorNil(t, err)
		expect.Equal(t, comment.Author.AvatarKey, avatar.ThumbnailKey)
	})
	t.Run("AvatarMediaCannotBecomePost", func(t *testing.T) {
		_, err := store.InsertPost(ctx, post.NewPost{AccountUUID: owner.UUID, MediaUUID: avatar.UUID})
		expect.ErrorIs(ErrMediaAlreadyUsed)(t, err)
	})
	t.Run("RemoveAvatar", func(t *testing.T) {
		expect.ErrorNil(t, store.RemoveAvatar(ctx, owner.UUID))
		me, err := store.GetMe(ctx, owner.UUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, me.AvatarKey, "")
	})
}
