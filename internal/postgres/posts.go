package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/post"
)

const postMediaUniqueConstraint = "post_media_uuid_key"

// postSelect returns one row per post. Every query built on it uses $1 for the
// viewer UUID (or NULL for anonymous), then its own values from $2. Later
// features extend this one query, so post columns are never listed anywhere else.
//
// ponytail: counts are subqueries per row. Add counter columns when feeds get slow.
const postSelect = `
SELECT p.uuid, p.caption, p.created_at,
	a.uuid, a.username, a.display_name, ` + avatarColumn + `,
	` + mediaColumns + `,
	(SELECT COUNT(*) FROM post_like l WHERE l.post_uuid = p.uuid) AS like_count,
	(SELECT COUNT(*) FROM comment c WHERE c.post_uuid = p.uuid AND c.deleted_at IS NULL) AS comment_count,
	EXISTS (SELECT 1 FROM post_like l WHERE l.post_uuid = p.uuid AND l.account_uuid = $1) AS liked_by_me
FROM post p
JOIN account a ON a.uuid = p.account_uuid
` + avatarJoin + `
JOIN media m ON m.uuid = p.media_uuid
`

// postVisible is the WHERE part for the visibility rules in 01-architecture.md:
// not deleted, media ready unless the viewer is the author, author not
// suspended, and no block between viewer and author.
var postVisible = `p.deleted_at IS NULL AND (m.status = 'ready' OR p.account_uuid = $1)
	AND a.suspended_at IS NULL AND ` + notBlockedByViewer("p.account_uuid")

// feedVisible is like postVisible, but media must be ready for everyone:
// feeds only show finished posts, even to their author.
var feedVisible = `p.deleted_at IS NULL AND m.status = 'ready'
	AND a.suspended_at IS NULL AND ` + notBlockedByViewer("p.account_uuid")

// notBlockedByViewer returns SQL that is true when there is no block in either
// direction between the viewer ($1) and the account in column. A NULL viewer
// (anonymous) matches no block, so anonymous readers see everything else.
func notBlockedByViewer(column string) string {
	return `NOT EXISTS (SELECT 1 FROM block b WHERE
		(b.blocker_uuid = $1 AND b.blocked_uuid = ` + column + `) OR
		(b.blocker_uuid = ` + column + ` AND b.blocked_uuid = $1))`
}

// scanner is satisfied by *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanPost scans the columns of postSelect.
func scanPost(row scanner) (post.Post, error) {
	var p post.Post
	dest := []any{&p.UUID, &p.Caption, &p.CreatedAt, &p.Author.UUID, &p.Author.Username, &p.Author.DisplayName, &p.Author.AvatarKey}
	dest = append(dest, mediaScanDest(&p.Media)...)
	dest = append(dest, &p.LikeCount, &p.CommentCount, &p.LikedByMe)
	if err := row.Scan(dest...); err != nil {
		return post.Post{}, err
	}
	p.CreatedAt = p.CreatedAt.UTC()
	p.Media.CreatedAt = p.Media.CreatedAt.UTC()
	return p, nil
}

// cursorArgs returns the keyset arguments for a list query: NULLs for the
// first page, else the cursor's time and UUID.
func cursorArgs(cursor pagination.Cursor) (createdAt *time.Time, id *uuid.UUID) {
	if cursor.IsZero() {
		return nil, nil
	}
	return &cursor.CreatedAt, &cursor.UUID
}

func (s *Store) InsertPost(ctx context.Context, newPost post.NewPost) (uuid.UUID, error) {
	var postUUID uuid.UUID
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO post (account_uuid, media_uuid, caption)
		SELECT $1, $2, $3
		WHERE NOT EXISTS (SELECT 1 FROM account WHERE avatar_media_uuid = $2)
		RETURNING uuid`,
		newPost.AccountUUID,
		newPost.MediaUUID,
		newPost.Caption,
	).Scan(&postUUID)
	// No row means the media is some account's avatar. Posts and avatars never share media.
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrMediaAlreadyUsed
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == postMediaUniqueConstraint {
		return uuid.Nil, ErrMediaAlreadyUsed
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("inserting post: %w", err)
	}
	return postUUID, nil
}

func (s *Store) GetPost(ctx context.Context, postUUID uuid.UUID, viewerUUID *uuid.UUID) (post.Post, error) {
	p, err := scanPost(s.db.QueryRowContext(ctx, postSelect+`WHERE `+postVisible+` AND p.uuid = $2`, viewerUUID, postUUID))
	if errors.Is(err, sql.ErrNoRows) {
		return post.Post{}, ErrPostNotFound
	}
	if err != nil {
		return post.Post{}, fmt.Errorf("getting post: %w", err)
	}
	return p, nil
}

// ListUserPosts returns up to limit+1 rows, newest first, strictly after cursor.
// The extra row tells the caller whether another page exists.
func (s *Store) ListUserPosts(ctx context.Context, accountUUID uuid.UUID, viewerUUID *uuid.UUID, cursor pagination.Cursor, limit int) ([]post.Post, error) {
	cursorTime, cursorUUID := cursorArgs(cursor)
	rows, err := s.db.QueryContext(ctx, postSelect+`
		WHERE `+postVisible+`
			AND p.account_uuid = $2
			AND ($3::timestamptz IS NULL OR (p.created_at, p.uuid) < ($3, $4))
		ORDER BY p.created_at DESC, p.uuid DESC
		LIMIT $5`,
		viewerUUID, accountUUID, cursorTime, cursorUUID, limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("listing user posts: %w", err)
	}
	return scanPosts(rows)
}

// scanPosts reads every row of a postSelect query and closes rows.
func scanPosts(rows *sql.Rows) ([]post.Post, error) {
	defer rows.Close()
	posts := []post.Post{}
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning post: %w", err)
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading posts: %w", err)
	}
	return posts, nil
}

// SoftDeletePost hides a post and returns its media so the caller can delete
// the files. A post by another account is reported as not found.
func (s *Store) SoftDeletePost(ctx context.Context, postUUID, accountUUID uuid.UUID) (media.Media, error) {
	var m media.Media
	err := s.db.QueryRowContext(ctx, `
		WITH deleted AS (
			UPDATE post SET deleted_at = NOW()
			WHERE uuid = $1 AND account_uuid = $2 AND deleted_at IS NULL
			RETURNING media_uuid
		)
		SELECT `+mediaColumns+` FROM media m JOIN deleted d ON d.media_uuid = m.uuid`,
		postUUID, accountUUID,
	).Scan(mediaScanDest(&m)...)
	if errors.Is(err, sql.ErrNoRows) {
		return media.Media{}, ErrPostNotFound
	}
	if err != nil {
		return media.Media{}, fmt.Errorf("deleting post: %w", err)
	}
	m.CreatedAt = m.CreatedAt.UTC()
	return m, nil
}
