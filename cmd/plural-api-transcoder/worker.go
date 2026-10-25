package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/plural-social/plural-api/internal/media"
)

const (
	// retryBackoffStep is the linear backoff unit: attempt n waits n × step.
	retryBackoffStep = 30 * time.Second
	genericFailure   = "processing failed, try uploading again"
)

// worker turns one claimed job into a ready or failed media item.
type worker struct {
	store       jobStore
	blobs       blobStore
	tools       videoTools
	logger      *zap.Logger
	tmpDir      string // empty means os.TempDir()
	maxAttempts int
	now         func() time.Time
}

// process runs one job and records the outcome in the store. It never returns
// an error: every failure becomes a retry or a failed media item.
func (w *worker) process(ctx context.Context, job media.Job) {
	m, err := w.store.GetMedia(ctx, job.MediaUUID)
	if err != nil {
		w.retryOrFail(ctx, job, job.MediaUUID, fmt.Errorf("getting media: %w", err))
		return
	}

	dir, err := os.MkdirTemp(w.tmpDir, "job-*")
	if err != nil {
		w.retryOrFail(ctx, job, m.UUID, fmt.Errorf("creating temp dir: %w", err))
		return
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "input")
	if err := w.download(ctx, m.OriginalKey, input); err != nil {
		w.retryOrFail(ctx, job, m.UUID, err)
		return
	}

	inputProbe, err := w.tools.Probe(ctx, input)
	if err == nil {
		err = media.ValidateProbe(inputProbe)
	}
	var invalid media.InvalidVideoError
	if errors.As(err, &invalid) {
		w.fail(ctx, job, m, err, invalid.Reason)
		return
	}
	if err != nil {
		w.retryOrFail(ctx, job, m.UUID, fmt.Errorf("probing input: %w", err))
		return
	}

	video, poster := filepath.Join(dir, "video.mp4"), filepath.Join(dir, "poster.jpg")
	if err := w.tools.Transcode(ctx, input, video); err != nil {
		w.retryOrFail(ctx, job, m.UUID, fmt.Errorf("transcoding: %w", err))
		return
	}
	// The output decides the stored sizes: scaling and rotation changed them.
	output, err := w.tools.Probe(ctx, video)
	if err != nil {
		w.retryOrFail(ctx, job, m.UUID, fmt.Errorf("probing output: %w", err))
		return
	}
	if err := w.tools.Poster(ctx, video, output.DurationMS, poster); err != nil {
		w.retryOrFail(ctx, job, m.UUID, fmt.Errorf("making poster: %w", err))
		return
	}

	videoKey, posterKey := media.VideoKey(m.UUID), media.PosterKey(m.UUID)
	videoSize, err := w.upload(ctx, video, videoKey, "video/mp4")
	if err == nil {
		_, err = w.upload(ctx, poster, posterKey, "image/jpeg")
	}
	if err != nil {
		w.retryOrFail(ctx, job, m.UUID, err)
		return
	}

	if err := w.store.CompleteVideoMedia(ctx, job.UUID, m.UUID, media.VideoResult{
		DisplayKey:   videoKey,
		ThumbnailKey: posterKey,
		Width:        output.Width,
		Height:       output.Height,
		DurationMS:   output.DurationMS,
		SizeBytes:    videoSize,
	}); err != nil {
		w.retryOrFail(ctx, job, m.UUID, err)
		return
	}

	w.deleteOriginal(ctx, m)
	w.logger.Info("Processed video", zap.Stringer("mediaUUID", m.UUID), zap.Int("durationMs", output.DurationMS))
}

// download copies a stored object to a local file.
func (w *worker) download(ctx context.Context, key, path string) error {
	object, err := w.blobs.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("downloading original: %w", err)
	}
	defer object.Close()

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating input file: %w", err)
	}
	defer file.Close()
	if _, err := io.Copy(file, object); err != nil {
		return fmt.Errorf("writing input file: %w", err)
	}
	return nil
}

// upload stores a local file and returns its size.
func (w *worker) upload(ctx context.Context, path, key, contentType string) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", key, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("reading size of %s: %w", key, err)
	}
	if err := w.blobs.Put(ctx, key, file, info.Size(), contentType); err != nil {
		return 0, fmt.Errorf("uploading %s: %w", key, err)
	}
	return info.Size(), nil
}

// retryOrFail retries with a linear backoff (attempts × 30 s) until
// maxAttempts, then fails the media with a generic reason.
func (w *worker) retryOrFail(ctx context.Context, job media.Job, mediaUUID uuid.UUID, err error) {
	w.logger.Error("Error processing video",
		zap.Stringer("jobUUID", job.UUID),
		zap.Stringer("mediaUUID", mediaUUID),
		zap.Int("attempts", job.Attempts),
		zap.Error(err),
	)
	if job.Attempts >= w.maxAttempts {
		w.fail(ctx, job, media.Media{UUID: mediaUUID, OriginalKey: media.OriginalKey(mediaUUID)}, err, genericFailure)
		return
	}
	runAfter := w.now().Add(time.Duration(job.Attempts) * retryBackoffStep)
	if retryErr := w.store.RetryMediaJob(ctx, job.UUID, err.Error(), runAfter); retryErr != nil {
		// The job stays "running"; stale-lock recovery picks it up later.
		w.logger.Error("Error scheduling retry", zap.Stringer("jobUUID", job.UUID), zap.Error(retryErr))
	}
}

// fail marks the media failed with a user-facing reason and deletes the original.
func (w *worker) fail(ctx context.Context, job media.Job, m media.Media, err error, reason string) {
	w.logger.Error("Failed video",
		zap.Stringer("jobUUID", job.UUID),
		zap.Stringer("mediaUUID", m.UUID),
		zap.Int("attempts", job.Attempts),
		zap.String("reason", reason),
		zap.Error(err),
	)
	if failErr := w.store.FailVideoMedia(ctx, job.UUID, m.UUID, err.Error(), reason); failErr != nil {
		w.logger.Error("Error marking video failed", zap.Stringer("jobUUID", job.UUID), zap.Error(failErr))
		return
	}
	w.deleteOriginal(ctx, m)
}

// deleteOriginal removes the upload once it is no longer needed. A leftover
// original is never served, so a failure is only logged.
func (w *worker) deleteOriginal(ctx context.Context, m media.Media) {
	if m.OriginalKey == "" {
		return
	}
	if err := w.blobs.Delete(ctx, m.OriginalKey); err != nil {
		w.logger.Warn("Error deleting original", zap.String("key", m.OriginalKey), zap.Error(err))
	}
}
