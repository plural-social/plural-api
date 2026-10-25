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

func (s *Store) GetMe(ctx context.Context, accountUUID uuid.UUID) (account.Me, error) {
	me := account.Me{Role: account.RoleUser}
	err := s.db.QueryRowContext(ctx, `
		SELECT uuid, username, display_name, email, bio, created_at
		FROM account
		WHERE uuid = $1`,
		accountUUID,
	).Scan(&me.UUID, &me.Username, &me.DisplayName, &me.Email, &me.Bio, &me.CreatedAt)
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
		SELECT a.uuid, a.username, a.display_name, a.bio, a.created_at,
			(SELECT COUNT(*) FROM post p JOIN media m ON m.uuid = p.media_uuid
			 WHERE p.account_uuid = a.uuid AND p.deleted_at IS NULL
				AND (m.status = 'ready' OR p.account_uuid = $2)) AS post_count
		FROM account a
		WHERE a.username = $1`,
		username, viewerUUID,
	).Scan(&profile.UUID, &profile.Username, &profile.DisplayName, &profile.Bio, &profile.CreatedAt, &profile.PostCount)
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
