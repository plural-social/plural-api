package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/post"
)

// Feeds are stricter than profiles: media must be ready for everyone,
// including the author, because feeds only show finished posts.

// ListFollowingFeed returns posts by the viewer and the people they follow,
// newest first, up to limit+1 rows.
func (s *Store) ListFollowingFeed(ctx context.Context, viewerUUID uuid.UUID, cursor pagination.Cursor, limit int) ([]post.Post, error) {
	cursorTime, cursorUUID := cursorArgs(cursor)
	rows, err := s.db.QueryContext(ctx, postSelect+`
		WHERE p.deleted_at IS NULL AND m.status = 'ready'
			AND (p.account_uuid = $1
				OR p.account_uuid IN (SELECT followee_uuid FROM follow WHERE follower_uuid = $1))
			AND ($2::timestamptz IS NULL OR (p.created_at, p.uuid) < ($2, $3))
		ORDER BY p.created_at DESC, p.uuid DESC
		LIMIT $4`,
		viewerUUID, cursorTime, cursorUUID, limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("listing following feed: %w", err)
	}
	return scanPosts(rows)
}

// ListExploreFeed returns every ready post on the instance, newest first, up
// to limit+1 rows. viewerUUID is unused until likes (F09) need it.
func (s *Store) ListExploreFeed(ctx context.Context, _ *uuid.UUID, cursor pagination.Cursor, limit int) ([]post.Post, error) {
	cursorTime, cursorUUID := cursorArgs(cursor)
	rows, err := s.db.QueryContext(ctx, postSelect+`
		WHERE p.deleted_at IS NULL AND m.status = 'ready'
			AND ($1::timestamptz IS NULL OR (p.created_at, p.uuid) < ($1, $2))
		ORDER BY p.created_at DESC, p.uuid DESC
		LIMIT $3`,
		cursorTime, cursorUUID, limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("listing explore feed: %w", err)
	}
	return scanPosts(rows)
}
