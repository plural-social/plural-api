package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	postLikePostForeignKey = "post_like_post_fkey"
	commentPostForeignKey  = "comment_post_fkey"
)

// isForeignKeyViolation reports whether err violates the named foreign key.
func isForeignKeyViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation && pgErr.ConstraintName == constraint
}

// LikePost is idempotent. A missing post returns ErrPostNotFound.
func (s *Store) LikePost(ctx context.Context, postUUID, accountUUID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO post_like (post_uuid, account_uuid)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`,
		postUUID, accountUUID,
	)
	if isForeignKeyViolation(err, postLikePostForeignKey) {
		return ErrPostNotFound
	}
	if err != nil {
		return fmt.Errorf("inserting like: %w", err)
	}
	return nil
}

// UnlikePost is idempotent.
func (s *Store) UnlikePost(ctx context.Context, postUUID, accountUUID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM post_like
		WHERE post_uuid = $1 AND account_uuid = $2`,
		postUUID, accountUUID,
	)
	if err != nil {
		return fmt.Errorf("deleting like: %w", err)
	}
	return nil
}
