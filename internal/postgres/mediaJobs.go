package postgres

// The media_job table is the queue between the API and the transcoder (D9 in
// 01-architecture.md). A job moves queued → running → done or failed, and back
// to queued for a retry. Rows are claimed with FOR UPDATE SKIP LOCKED, so many
// transcoders can poll at once without taking the same job.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/media"
)

// lastErrorMaxBytes caps stored errors; ffmpeg output can be long.
const lastErrorMaxBytes = 2000

// ClaimMediaJob takes the oldest runnable job and marks it running. A running
// job whose lock is older than staleAfter belongs to a crashed worker and is
// taken again. ok is false when no job is ready.
func (s *Store) ClaimMediaJob(ctx context.Context, staleAfter time.Duration) (job media.Job, ok bool, err error) {
	err = s.db.QueryRowContext(ctx, `
		UPDATE media_job
		SET status = 'running', attempts = attempts + 1, locked_at = NOW(), updated_at = NOW()
		WHERE uuid = (
			SELECT uuid FROM media_job
			WHERE (status = 'queued' AND run_after <= NOW())
				OR (status = 'running' AND locked_at < NOW() - $1::interval)
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING uuid, media_uuid, attempts`,
		fmt.Sprintf("%d seconds", int(staleAfter.Seconds())),
	).Scan(&job.UUID, &job.MediaUUID, &job.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return media.Job{}, false, nil
	}
	if err != nil {
		return media.Job{}, false, fmt.Errorf("claiming media job: %w", err)
	}
	return job, true, nil
}

// CompleteVideoMedia marks the media ready with the transcoded files and closes the job.
func (s *Store) CompleteVideoMedia(ctx context.Context, jobUUID, mediaUUID uuid.UUID, result media.VideoResult) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			UPDATE media
			SET status = 'ready', display_key = $2, thumbnail_key = $3, width = $4, height = $5,
				duration_ms = $6, size_bytes = $7, original_key = '', processed_at = NOW()
			WHERE uuid = $1`,
			mediaUUID, result.DisplayKey, result.ThumbnailKey, result.Width, result.Height, result.DurationMS, result.SizeBytes,
		); err != nil {
			return fmt.Errorf("completing media: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE media_job SET status = 'done', locked_at = NULL, updated_at = NOW() WHERE uuid = $1`,
			jobUUID,
		); err != nil {
			return fmt.Errorf("completing media job: %w", err)
		}
		return nil
	})
}

// RetryMediaJob puts the job back in the queue, runnable from runAfter.
func (s *Store) RetryMediaJob(ctx context.Context, jobUUID uuid.UUID, lastError string, runAfter time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE media_job
		SET status = 'queued', locked_at = NULL, last_error = $2, run_after = $3, updated_at = NOW()
		WHERE uuid = $1`,
		jobUUID, truncateBytes(lastError, lastErrorMaxBytes), runAfter,
	)
	if err != nil {
		return fmt.Errorf("retrying media job: %w", err)
	}
	return nil
}

// FailVideoMedia gives up: the media shows reason to its owner, and the job
// keeps lastError for operators.
func (s *Store) FailVideoMedia(ctx context.Context, jobUUID, mediaUUID uuid.UUID, lastError, reason string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			UPDATE media SET status = 'failed', failure_reason = $2, processed_at = NOW() WHERE uuid = $1`,
			mediaUUID, reason,
		); err != nil {
			return fmt.Errorf("failing media: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE media_job SET status = 'failed', locked_at = NULL, last_error = $2, updated_at = NOW() WHERE uuid = $1`,
			jobUUID, truncateBytes(lastError, lastErrorMaxBytes),
		); err != nil {
			return fmt.Errorf("failing media job: %w", err)
		}
		return nil
	})
}

// truncateBytes cuts s to at most n bytes without splitting a UTF-8 character.
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
