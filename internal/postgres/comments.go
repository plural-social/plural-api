package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/social"
)

const commentSelect = `
SELECT c.uuid, c.post_uuid, c.body, c.created_at,
	a.uuid, a.username, a.display_name,
	p.account_uuid
FROM comment c
JOIN account a ON a.uuid = c.account_uuid
JOIN post p ON p.uuid = c.post_uuid
`

// scanComment scans the columns of commentSelect.
func scanComment(row scanner) (social.Comment, error) {
	var c social.Comment
	err := row.Scan(
		&c.UUID, &c.PostUUID, &c.Body, &c.CreatedAt,
		&c.Author.UUID, &c.Author.Username, &c.Author.DisplayName,
		&c.PostAuthorUUID,
	)
	c.CreatedAt = c.CreatedAt.UTC()
	return c, err
}

// InsertComment returns ErrPostNotFound when the post does not exist.
func (s *Store) InsertComment(ctx context.Context, newComment social.NewComment) (uuid.UUID, error) {
	var commentUUID uuid.UUID
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO comment (post_uuid, account_uuid, body)
		VALUES ($1, $2, $3)
		RETURNING uuid`,
		newComment.PostUUID, newComment.AccountUUID, newComment.Body,
	).Scan(&commentUUID)
	if isForeignKeyViolation(err, commentPostForeignKey) {
		return uuid.Nil, ErrPostNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("inserting comment: %w", err)
	}
	return commentUUID, nil
}

func (s *Store) GetComment(ctx context.Context, commentUUID uuid.UUID) (social.Comment, error) {
	c, err := scanComment(s.db.QueryRowContext(ctx, commentSelect+`WHERE c.deleted_at IS NULL AND c.uuid = $1`, commentUUID))
	if errors.Is(err, sql.ErrNoRows) {
		return social.Comment{}, ErrCommentNotFound
	}
	if err != nil {
		return social.Comment{}, fmt.Errorf("getting comment: %w", err)
	}
	return c, nil
}

// ListComments returns up to limit+1 rows, newest first, strictly after cursor.
func (s *Store) ListComments(ctx context.Context, postUUID uuid.UUID, cursor pagination.Cursor, limit int) ([]social.Comment, error) {
	cursorTime, cursorUUID := cursorArgs(cursor)
	rows, err := s.db.QueryContext(ctx, commentSelect+`
		WHERE c.deleted_at IS NULL AND c.post_uuid = $1
			AND ($2::timestamptz IS NULL OR (c.created_at, c.uuid) < ($2, $3))
		ORDER BY c.created_at DESC, c.uuid DESC
		LIMIT $4`,
		postUUID, cursorTime, cursorUUID, limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("listing comments: %w", err)
	}
	defer rows.Close()

	comments := []social.Comment{}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning comment: %w", err)
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading comments: %w", err)
	}
	return comments, nil
}

func (s *Store) SoftDeleteComment(ctx context.Context, commentUUID uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `UPDATE comment SET deleted_at = NOW() WHERE uuid = $1 AND deleted_at IS NULL`, commentUUID)
	if err != nil {
		return fmt.Errorf("deleting comment: %w", err)
	}
	rows, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return fmt.Errorf("counting deleted comments: %w", rowsErr)
	}
	if rows == 0 {
		return ErrCommentNotFound
	}
	return nil
}
