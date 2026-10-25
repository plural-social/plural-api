package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/plural-social/plural-api/internal/account"
)

const (
	accountUsernameUniqueConstraint = "account_username_key"
	accountEmailUniqueConstraint    = "account_email_key"
)

func (s *Store) InsertAccount(ctx context.Context, newAccount account.NewAccount) (account.Account, error) {
	created := account.Account{Username: newAccount.Username}
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO account (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING uuid, created_at`,
		newAccount.Username,
		newAccount.Email,
		newAccount.PasswordHash,
	).Scan(&created.UUID, &created.CreatedAt)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		switch pgErr.ConstraintName {
		case accountUsernameUniqueConstraint:
			return account.Account{}, UsernameConflictError{Username: newAccount.Username}
		case accountEmailUniqueConstraint:
			return account.Account{}, EmailConflictError{}
		}
	}
	if err != nil {
		return account.Account{}, fmt.Errorf("inserting account: %w", err)
	}
	created.CreatedAt = created.CreatedAt.UTC()
	return created, nil
}

// avatarJoin and avatarColumn add a user's avatar thumbnail key to a query on
// account (aliased "a"). Avatars that are not ready count as no avatar.
const (
	avatarJoin   = `LEFT JOIN media av ON av.uuid = a.avatar_media_uuid AND av.status = 'ready'`
	avatarColumn = `COALESCE(av.thumbnail_key, '')`
)

func (s *Store) GetMe(ctx context.Context, accountUUID uuid.UUID) (account.Me, error) {
	me := account.Me{Role: account.RoleUser}
	err := s.db.QueryRowContext(ctx, `
		SELECT a.uuid, a.username, a.display_name, `+avatarColumn+`, a.email, a.bio, a.created_at
		FROM account a
		`+avatarJoin+`
		WHERE a.uuid = $1`,
		accountUUID,
	).Scan(&me.UUID, &me.Username, &me.DisplayName, &me.AvatarKey, &me.Email, &me.Bio, &me.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return account.Me{}, ErrAccountNotFound
	}
	if err != nil {
		return account.Me{}, fmt.Errorf("getting account: %w", err)
	}
	me.CreatedAt = me.CreatedAt.UTC()
	return me, nil
}

// UpdateProfile sets the non-nil fields of update. Nil pointers become SQL
// NULL, which COALESCE turns into "keep the current value".
func (s *Store) UpdateProfile(ctx context.Context, accountUUID uuid.UUID, update account.ProfileUpdate) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE account
		SET display_name = COALESCE($2, display_name),
			bio = COALESCE($3, bio)
		WHERE uuid = $1`,
		accountUUID,
		update.DisplayName,
		update.Bio,
	)
	if err != nil {
		return fmt.Errorf("updating profile: %w", err)
	}
	rows, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return fmt.Errorf("counting updated profiles: %w", rowsErr)
	}
	if rows == 0 {
		return ErrAccountNotFound
	}
	return nil
}

func (s *Store) GetProfile(ctx context.Context, username string, viewerUUID *uuid.UUID) (account.Profile, error) {
	var profile account.Profile
	err := s.db.QueryRowContext(ctx, `
		SELECT a.uuid, a.username, a.display_name, `+avatarColumn+`, a.bio, a.created_at,
			(SELECT COUNT(*) FROM post p JOIN media m ON m.uuid = p.media_uuid
			 WHERE p.account_uuid = a.uuid AND p.deleted_at IS NULL
				AND (m.status = 'ready' OR p.account_uuid = $2)) AS post_count,
			(SELECT COUNT(*) FROM follow f WHERE f.followee_uuid = a.uuid) AS follower_count,
			(SELECT COUNT(*) FROM follow f WHERE f.follower_uuid = a.uuid) AS following_count,
			EXISTS (SELECT 1 FROM follow f WHERE f.follower_uuid = $2 AND f.followee_uuid = a.uuid) AS is_following
		FROM account a
		`+avatarJoin+`
		WHERE a.username = $1`,
		username, viewerUUID,
	).Scan(&profile.UUID, &profile.Username, &profile.DisplayName, &profile.AvatarKey, &profile.Bio, &profile.CreatedAt,
		&profile.PostCount, &profile.FollowerCount, &profile.FollowingCount, &profile.IsFollowing)
	if errors.Is(err, sql.ErrNoRows) {
		return account.Profile{}, ErrAccountNotFound
	}
	if err != nil {
		return account.Profile{}, fmt.Errorf("getting profile: %w", err)
	}
	profile.IsSelf = viewerUUID != nil && *viewerUUID == profile.UUID
	profile.CreatedAt = profile.CreatedAt.UTC()
	return profile, nil
}

func (s *Store) GetAccountUUID(ctx context.Context, username string) (uuid.UUID, error) {
	var accountUUID uuid.UUID
	err := s.db.QueryRowContext(ctx, `SELECT uuid FROM account WHERE username = $1`, username).Scan(&accountUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrAccountNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("getting account UUID: %w", err)
	}
	return accountUUID, nil
}

// SetAvatar needs a ready image the account uploaded that no post uses. Posts
// are public, so an avatar that doubled as a post would be deleted along with
// it. Anything else returns ErrInvalidAvatarMedia.
//
// ponytail: replaced avatars leave their files in storage. Add a cleanup job when storage cost matters.
func (s *Store) SetAvatar(ctx context.Context, accountUUID, mediaUUID uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE account SET avatar_media_uuid = $2
		WHERE uuid = $1
			AND EXISTS (
				SELECT 1 FROM media m
				WHERE m.uuid = $2 AND m.account_uuid = $1 AND m.kind = 'image' AND m.status = 'ready'
					AND NOT EXISTS (SELECT 1 FROM post p WHERE p.media_uuid = m.uuid)
			)`,
		accountUUID, mediaUUID,
	)
	if err != nil {
		return fmt.Errorf("setting avatar: %w", err)
	}
	rows, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return fmt.Errorf("counting updated avatars: %w", rowsErr)
	}
	if rows == 0 {
		return ErrInvalidAvatarMedia
	}
	return nil
}

func (s *Store) RemoveAvatar(ctx context.Context, accountUUID uuid.UUID) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE account SET avatar_media_uuid = NULL WHERE uuid = $1`, accountUUID); err != nil {
		return fmt.Errorf("removing avatar: %w", err)
	}
	return nil
}
