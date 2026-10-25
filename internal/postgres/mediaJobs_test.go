package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"

	"github.com/plural-social/plural-api/internal/media"
)

// staleAfter matches the transcoder default.
const testStaleAfter = 15 * time.Minute

func Test_Store_mediaJobs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	owner := insertTestAccount(t, store)

	insertVideo := func(t *testing.T) media.Media {
		t.Helper()
		id := uuid.New()
		m, err := store.InsertVideoMedia(ctx, media.NewMedia{
			UUID: id, AccountUUID: owner.UUID, Kind: media.KindVideo, Status: media.StatusProcessing,
			ContentType: "video/mp4", OriginalKey: media.OriginalKey(id),
		})
		expect.ErrorNil(t, err)
		return m
	}
	// claimMine claims jobs until it finds the one for mediaUUID. Other tests
	// may have runnable jobs in the same database; those are put back.
	claimMine := func(t *testing.T, mediaUUID uuid.UUID) (media.Job, bool) {
		t.Helper()
		var others []media.Job
		defer func() {
			for _, other := range others {
				expect.ErrorNil(t, store.RetryMediaJob(ctx, other.UUID, "", time.Now()))
			}
		}()
		for range 100 {
			job, ok, err := store.ClaimMediaJob(ctx, testStaleAfter)
			expect.ErrorNil(t, err)
			if !ok {
				return media.Job{}, false
			}
			if job.MediaUUID == mediaUUID {
				return job, true
			}
			others = append(others, job)
		}
		t.Fatal("too many foreign jobs")
		return media.Job{}, false
	}

	t.Run("ClaimOnce", func(t *testing.T) {
		m := insertVideo(t)
		job, ok := claimMine(t, m.UUID)
		expect.Equal(t, ok, true)
		expect.Equal(t, job.Attempts, 1)
		_, ok = claimMine(t, m.UUID)
		expect.Equal(t, ok, false)
	})
	t.Run("StaleLockReclaimed", func(t *testing.T) {
		m := insertVideo(t)
		job, _ := claimMine(t, m.UUID)
		_, err := store.db.ExecContext(ctx, `UPDATE media_job SET locked_at = NOW() - INTERVAL '1 hour' WHERE uuid = $1`, job.UUID)
		expect.ErrorNil(t, err)
		again, ok := claimMine(t, m.UUID)
		expect.Equal(t, ok, true)
		expect.Equal(t, again.Attempts, 2)
	})
	t.Run("RetryRespectsRunAfter", func(t *testing.T) {
		m := insertVideo(t)
		job, _ := claimMine(t, m.UUID)
		expect.ErrorNil(t, store.RetryMediaJob(ctx, job.UUID, "boom", time.Now().Add(time.Hour)))
		_, ok := claimMine(t, m.UUID)
		expect.Equal(t, ok, false)
		expect.ErrorNil(t, store.RetryMediaJob(ctx, job.UUID, "boom", time.Now().Add(-time.Second)))
		_, ok = claimMine(t, m.UUID)
		expect.Equal(t, ok, true)
	})
	t.Run("Complete", func(t *testing.T) {
		m := insertVideo(t)
		job, _ := claimMine(t, m.UUID)
		expect.ErrorNil(t, store.CompleteVideoMedia(ctx, job.UUID, m.UUID, media.VideoResult{
			DisplayKey: media.VideoKey(m.UUID), ThumbnailKey: media.PosterKey(m.UUID),
			Width: 720, Height: 1280, DurationMS: 3000, SizeBytes: 1234,
		}))
		got, err := store.GetMedia(ctx, m.UUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, got.Status, media.StatusReady)
		expect.Equal(t, got.DisplayKey, media.VideoKey(m.UUID))
		expect.Equal(t, got.ThumbnailKey, media.PosterKey(m.UUID))
		expect.Equal(t, [2]int{got.Width, got.Height}, [2]int{720, 1280})
		expect.Equal(t, got.OriginalKey, "")
		_, ok := claimMine(t, m.UUID)
		expect.Equal(t, ok, false)
	})
	t.Run("Fail", func(t *testing.T) {
		m := insertVideo(t)
		job, _ := claimMine(t, m.UUID)
		expect.ErrorNil(t, store.FailVideoMedia(ctx, job.UUID, m.UUID, "ffmpeg exploded", "video must be 3 minutes or shorter"))
		got, err := store.GetMedia(ctx, m.UUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, got.Status, media.StatusFailed)
		expect.Equal(t, got.FailureReason, "video must be 3 minutes or shorter")
		_, ok := claimMine(t, m.UUID)
		expect.Equal(t, ok, false)
	})
}
