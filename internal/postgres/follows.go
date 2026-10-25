package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Follow is idempotent: following twice is not an error.
func (s *Store) Follow(ctx context.Context, followerUUID, followeeUUID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO follow (follower_uuid, followee_uuid)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`,
		followerUUID, followeeUUID,
	)
	if err != nil {
		return fmt.Errorf("inserting follow: %w", err)
	}
	return nil
}

// Unfollow is idempotent: unfollowing someone you do not follow is not an error.
func (s *Store) Unfollow(ctx context.Context, followerUUID, followeeUUID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM follow
		WHERE follower_uuid = $1 AND followee_uuid = $2`,
		followerUUID, followeeUUID,
	)
	if err != nil {
		return fmt.Errorf("deleting follow: %w", err)
	}
	return nil
}
