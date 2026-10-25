package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"

	"github.com/plural-social/plural-api/internal/media"
)

func Test_Store_media(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	owner := insertTestAccount(t, store)

	newMedia := media.NewMedia{
		UUID:         uuid.New(),
		AccountUUID:  owner.UUID,
		Kind:         media.KindImage,
		Status:       media.StatusReady,
		ContentType:  "image/jpeg",
		DisplayKey:   "media/x/display.jpg",
		ThumbnailKey: "media/x/thumb.jpg",
		Width:        640,
		Height:       480,
		SizeBytes:    1234,
	}

	t.Run("UnknownMedia", func(t *testing.T) {
		_, err := store.GetMedia(ctx, uuid.New())
		expect.ErrorIs(ErrMediaNotFound)(t, err)
	})
	t.Run("InsertThenGet", func(t *testing.T) {
		inserted, err := store.InsertMedia(ctx, newMedia)
		expect.ErrorNil(t, err)
		got, err := store.GetMedia(ctx, newMedia.UUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, got, inserted)
		expect.Equal(t, got.Width, 640)
		expect.Equal(t, got.DisplayKey, "media/x/display.jpg")
	})
}
