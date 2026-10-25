package main

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/blob"
	"github.com/plural-social/plural-api/internal/media"
)

//go:generate go tool mock -o jobStore_mock.go jobStore
type jobStore interface {
	ClaimMediaJob(ctx context.Context, staleAfter time.Duration) (media.Job, bool, error)
	GetMedia(ctx context.Context, mediaUUID uuid.UUID) (media.Media, error)
	CompleteVideoMedia(ctx context.Context, jobUUID, mediaUUID uuid.UUID, result media.VideoResult) error
	RetryMediaJob(ctx context.Context, jobUUID uuid.UUID, lastError string, runAfter time.Time) error
	FailVideoMedia(ctx context.Context, jobUUID, mediaUUID uuid.UUID, lastError, reason string) error
}

//go:generate go tool mock -o blobStore_mock.go blobStore
type blobStore interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (*blob.Object, error)
	Delete(ctx context.Context, key string) error
}

//go:generate go tool mock -o videoTools_mock.go videoTools
type videoTools interface {
	Probe(ctx context.Context, path string) (media.ProbeResult, error)
	Transcode(ctx context.Context, inPath, outPath string) error
	Poster(ctx context.Context, videoPath string, durationMS int, outPath string) error
}
