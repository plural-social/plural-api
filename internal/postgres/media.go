package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/media"
)

// mediaColumns and mediaScanDest must list the same columns in the same order.
// Queries alias the table as "m" so they can join it.
const mediaColumns = `m.uuid, m.account_uuid, m.kind, m.status, m.content_type,
	m.display_key, m.thumbnail_key, m.original_key, m.width, m.height,
	m.duration_ms, m.size_bytes, m.failure_reason, m.created_at`

func mediaScanDest(m *media.Media) []any {
	return []any{
		&m.UUID, &m.AccountUUID, &m.Kind, &m.Status, &m.ContentType,
		&m.DisplayKey, &m.ThumbnailKey, &m.OriginalKey, &m.Width, &m.Height,
		&m.DurationMS, &m.SizeBytes, &m.FailureReason, &m.CreatedAt,
	}
}

func (s *Store) InsertMedia(ctx context.Context, newMedia media.NewMedia) (media.Media, error) {
	return insertMedia(ctx, s.db, newMedia)
}

// InsertVideoMedia stores an uploaded video and queues its transcoding job.
// Both happen or neither does, so no video waits forever without a job.
func (s *Store) InsertVideoMedia(ctx context.Context, newMedia media.NewMedia) (media.Media, error) {
	var inserted media.Media
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var insertErr error
		if inserted, insertErr = insertMedia(ctx, tx, newMedia); insertErr != nil {
			return insertErr
		}
		if _, jobErr := tx.ExecContext(ctx, `INSERT INTO media_job (media_uuid) VALUES ($1)`, newMedia.UUID); jobErr != nil {
			return fmt.Errorf("inserting media job: %w", jobErr)
		}
		return nil
	})
	return inserted, err
}

// queryRower is satisfied by *sql.DB and *sql.Tx.
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func insertMedia(ctx context.Context, db queryRower, newMedia media.NewMedia) (media.Media, error) {
	inserted := media.Media{
		UUID:          newMedia.UUID,
		AccountUUID:   newMedia.AccountUUID,
		Kind:          newMedia.Kind,
		Status:        newMedia.Status,
		ContentType:   newMedia.ContentType,
		DisplayKey:    newMedia.DisplayKey,
		ThumbnailKey:  newMedia.ThumbnailKey,
		OriginalKey:   newMedia.OriginalKey,
		Width:         newMedia.Width,
		Height:        newMedia.Height,
		DurationMS:    newMedia.DurationMS,
		SizeBytes:     newMedia.SizeBytes,
		FailureReason: newMedia.FailureReason,
	}
	err := db.QueryRowContext(ctx, `
		INSERT INTO media (
			uuid, account_uuid, kind, status, content_type, display_key, thumbnail_key,
			original_key, width, height, duration_ms, size_bytes, failure_reason, processed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, CASE WHEN $4 = 'ready' THEN NOW() END)
		RETURNING created_at`,
		newMedia.UUID,
		newMedia.AccountUUID,
		newMedia.Kind,
		newMedia.Status,
		newMedia.ContentType,
		newMedia.DisplayKey,
		newMedia.ThumbnailKey,
		newMedia.OriginalKey,
		newMedia.Width,
		newMedia.Height,
		newMedia.DurationMS,
		newMedia.SizeBytes,
		newMedia.FailureReason,
	).Scan(&inserted.CreatedAt)
	if err != nil {
		return media.Media{}, fmt.Errorf("inserting media: %w", err)
	}
	inserted.CreatedAt = inserted.CreatedAt.UTC()
	return inserted, nil
}

func (s *Store) GetMedia(ctx context.Context, mediaUUID uuid.UUID) (media.Media, error) {
	var m media.Media
	err := s.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM media m WHERE m.uuid = $1`, mediaUUID).
		Scan(mediaScanDest(&m)...)
	if errors.Is(err, sql.ErrNoRows) {
		return media.Media{}, ErrMediaNotFound
	}
	if err != nil {
		return media.Media{}, fmt.Errorf("getting media: %w", err)
	}
	m.CreatedAt = m.CreatedAt.UTC()
	return m, nil
}
