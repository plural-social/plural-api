package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/moderation"
	"github.com/plural-social/plural-api/internal/pagination"
)

// inTx runs fn in a transaction, committing when fn returns nil.
func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if commitErr := tx.Commit(); commitErr != nil {
		return fmt.Errorf("committing transaction: %w", commitErr)
	}
	return nil
}

// BlockAccount is idempotent. It also removes follows in both directions, so
// neither account keeps seeing the other in its Following feed.
func (s *Store) BlockAccount(ctx context.Context, blockerUUID, blockedUUID uuid.UUID) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO block (blocker_uuid, blocked_uuid)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING`,
			blockerUUID, blockedUUID,
		); err != nil {
			return fmt.Errorf("inserting block: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM follow
			WHERE (follower_uuid = $1 AND followee_uuid = $2) OR (follower_uuid = $2 AND followee_uuid = $1)`,
			blockerUUID, blockedUUID,
		); err != nil {
			return fmt.Errorf("deleting follows: %w", err)
		}
		return nil
	})
}

// UnblockAccount is idempotent. Follows removed by the block are not restored.
func (s *Store) UnblockAccount(ctx context.Context, blockerUUID, blockedUUID uuid.UUID) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM block WHERE blocker_uuid = $1 AND blocked_uuid = $2`, blockerUUID, blockedUUID); err != nil {
		return fmt.Errorf("deleting block: %w", err)
	}
	return nil
}

func (s *Store) IsBlockedEitherWay(ctx context.Context, a, b uuid.UUID) (bool, error) {
	var blocked bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM block
			WHERE (blocker_uuid = $1 AND blocked_uuid = $2) OR (blocker_uuid = $2 AND blocked_uuid = $1)
		)`,
		a, b,
	).Scan(&blocked)
	if err != nil {
		return false, fmt.Errorf("checking block: %w", err)
	}
	return blocked, nil
}

// ListBlocks returns up to limit+1 accounts the blocker blocked, most recent first.
func (s *Store) ListBlocks(ctx context.Context, blockerUUID uuid.UUID, cursor pagination.Cursor, limit int) ([]moderation.BlockedAccount, error) {
	cursorTime, cursorUUID := cursorArgs(cursor)
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.uuid, a.username, a.display_name, `+avatarColumn+`, b.created_at
		FROM block b
		JOIN account a ON a.uuid = b.blocked_uuid
		`+avatarJoin+`
		WHERE b.blocker_uuid = $1
			AND ($2::timestamptz IS NULL OR (b.created_at, a.uuid) < ($2, $3))
		ORDER BY b.created_at DESC, a.uuid DESC
		LIMIT $4`,
		blockerUUID, cursorTime, cursorUUID, limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("listing blocks: %w", err)
	}
	defer rows.Close()

	blocks := []moderation.BlockedAccount{}
	for rows.Next() {
		var b moderation.BlockedAccount
		if err := rows.Scan(&b.UUID, &b.Username, &b.DisplayName, &b.AvatarKey, &b.BlockedAt); err != nil {
			return nil, fmt.Errorf("scanning block: %w", err)
		}
		b.BlockedAt = b.BlockedAt.UTC()
		blocks = append(blocks, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading blocks: %w", err)
	}
	return blocks, nil
}

func (s *Store) InsertReport(ctx context.Context, report moderation.NewReport) (uuid.UUID, error) {
	var reportUUID uuid.UUID
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO report (reporter_uuid, target_type, target_uuid, reason, details)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING uuid`,
		report.ReporterUUID, report.TargetType, report.TargetUUID, report.Reason, report.Details,
	).Scan(&reportUUID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("inserting report: %w", err)
	}
	return reportUUID, nil
}

// reportSelect returns a report, its reporter, and a preview of the target.
// The preview ignores visibility on purpose: moderators must see removed or
// hidden content to judge it.
const reportSelect = `
SELECT r.uuid, r.target_type, r.target_uuid, r.reason, r.details, r.status,
	r.resolution_note, r.created_at, r.resolved_at,
	ra.uuid, ra.username, ra.display_name, COALESCE(rav.thumbnail_key, ''),
	COALESCE(ta.username, ''),
	COALESCE(CASE r.target_type WHEN 'post' THEN tp.caption WHEN 'comment' THEN tc.body ELSE ta.bio END, ''),
	COALESCE(tm.thumbnail_key, '')
FROM report r
JOIN account ra ON ra.uuid = r.reporter_uuid
LEFT JOIN media rav ON rav.uuid = ra.avatar_media_uuid AND rav.status = 'ready'
LEFT JOIN post tp ON r.target_type = 'post' AND tp.uuid = r.target_uuid
LEFT JOIN media tm ON tm.uuid = tp.media_uuid
LEFT JOIN comment tc ON r.target_type = 'comment' AND tc.uuid = r.target_uuid
LEFT JOIN account ta ON ta.uuid = CASE r.target_type
	WHEN 'post' THEN tp.account_uuid
	WHEN 'comment' THEN tc.account_uuid
	ELSE r.target_uuid END
`

func scanReport(row scanner) (moderation.Report, error) {
	var r moderation.Report
	preview := &moderation.TargetPreview{}
	err := row.Scan(
		&r.UUID, &r.TargetType, &r.TargetUUID, &r.Reason, &r.Details, &r.Status,
		&r.ResolutionNote, &r.CreatedAt, &r.ResolvedAt,
		&r.Reporter.UUID, &r.Reporter.Username, &r.Reporter.DisplayName, &r.Reporter.AvatarKey,
		&preview.Username, &preview.Text, &preview.ThumbnailKey,
	)
	r.CreatedAt = r.CreatedAt.UTC()
	if r.ResolvedAt != nil {
		r.ResolvedAt = new(r.ResolvedAt.UTC())
	}
	r.TargetPreview = preview
	return r, err
}

func (s *Store) GetReport(ctx context.Context, reportUUID uuid.UUID) (moderation.Report, error) {
	r, err := scanReport(s.db.QueryRowContext(ctx, reportSelect+`WHERE r.uuid = $1`, reportUUID))
	if errors.Is(err, sql.ErrNoRows) {
		return moderation.Report{}, ErrReportNotFound
	}
	if err != nil {
		return moderation.Report{}, fmt.Errorf("getting report: %w", err)
	}
	return r, nil
}

// ListReports returns up to limit+1 reports with the status, newest first.
func (s *Store) ListReports(ctx context.Context, status string, cursor pagination.Cursor, limit int) ([]moderation.Report, error) {
	cursorTime, cursorUUID := cursorArgs(cursor)
	rows, err := s.db.QueryContext(ctx, reportSelect+`
		WHERE r.status = $1
			AND ($2::timestamptz IS NULL OR (r.created_at, r.uuid) < ($2, $3))
		ORDER BY r.created_at DESC, r.uuid DESC
		LIMIT $4`,
		status, cursorTime, cursorUUID, limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("listing reports: %w", err)
	}
	defer rows.Close()

	reports := []moderation.Report{}
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning report: %w", err)
		}
		reports = append(reports, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading reports: %w", err)
	}
	return reports, nil
}

// ResolveReports closes open reports on a target: one report, or all of them
// when OnlyReportUUID is nil.
func (s *Store) ResolveReports(ctx context.Context, res moderation.Resolution) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE report
		SET status = $1, resolved_by_uuid = $2, resolution_note = $3, resolved_at = NOW()
		WHERE status = 'open' AND target_type = $4 AND target_uuid = $5
			AND ($6::uuid IS NULL OR uuid = $6)`,
		res.Status, res.ModeratorUUID, res.Note, res.TargetType, res.TargetUUID, res.OnlyReportUUID,
	)
	if err != nil {
		return fmt.Errorf("resolving reports: %w", err)
	}
	return nil
}

// GetContentAuthor reads who wrote a post or comment, ignoring visibility and
// deletion, so moderators can act on hidden content.
func (s *Store) GetContentAuthor(ctx context.Context, targetType string, targetUUID uuid.UUID) (uuid.UUID, error) {
	query := `SELECT account_uuid FROM post WHERE uuid = $1`
	notFound := ErrPostNotFound
	if targetType == moderation.TargetComment {
		query = `SELECT account_uuid FROM comment WHERE uuid = $1`
		notFound = ErrCommentNotFound
	}
	var author uuid.UUID
	err := s.db.QueryRowContext(ctx, query, targetUUID).Scan(&author)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, notFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("getting content author: %w", err)
	}
	return author, nil
}

// RemovePost hides any post (moderator action) and returns its media so the
// caller can delete the files. An already deleted post returns ErrPostNotFound.
func (s *Store) RemovePost(ctx context.Context, postUUID uuid.UUID) (media.Media, error) {
	var m media.Media
	err := s.db.QueryRowContext(ctx, `
		WITH deleted AS (
			UPDATE post SET deleted_at = NOW()
			WHERE uuid = $1 AND deleted_at IS NULL
			RETURNING media_uuid
		)
		SELECT `+mediaColumns+` FROM media m JOIN deleted d ON d.media_uuid = m.uuid`,
		postUUID,
	).Scan(mediaScanDest(&m)...)
	if errors.Is(err, sql.ErrNoRows) {
		return media.Media{}, ErrPostNotFound
	}
	if err != nil {
		return media.Media{}, fmt.Errorf("removing post: %w", err)
	}
	m.CreatedAt = m.CreatedAt.UTC()
	return m, nil
}

// SuspendAccount hides the account and its content and ends all its sessions.
// Suspending an already suspended account changes nothing.
func (s *Store) SuspendAccount(ctx context.Context, accountUUID uuid.UUID) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE account SET suspended_at = NOW() WHERE uuid = $1 AND suspended_at IS NULL`, accountUUID); err != nil {
			return fmt.Errorf("suspending account: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM session WHERE account_uuid = $1`, accountUUID); err != nil {
			return fmt.Errorf("deleting sessions: %w", err)
		}
		return nil
	})
}

func (s *Store) UnsuspendAccount(ctx context.Context, accountUUID uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `UPDATE account SET suspended_at = NULL WHERE uuid = $1`, accountUUID)
	if err != nil {
		return fmt.Errorf("unsuspending account: %w", err)
	}
	rows, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return fmt.Errorf("counting unsuspended accounts: %w", rowsErr)
	}
	if rows == 0 {
		return ErrAccountNotFound
	}
	return nil
}
