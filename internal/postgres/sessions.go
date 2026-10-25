package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/plural-social/plural-api/internal/account"
)

func (s *Store) GetCredentials(ctx context.Context, login string) (account.Credentials, error) {
	var credentials account.Credentials
	err := s.db.QueryRowContext(ctx, `
		SELECT uuid, password_hash
		FROM account
		WHERE username = $1 OR email = $1`,
		login,
	).Scan(&credentials.AccountUUID, &credentials.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return account.Credentials{}, ErrAccountNotFound
	}
	if err != nil {
		return account.Credentials{}, fmt.Errorf("getting credentials: %w", err)
	}
	return credentials, nil
}

func (s *Store) InsertSession(ctx context.Context, session account.NewSession) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session (token_hash, account_uuid, user_agent, expires_at)
		VALUES ($1, $2, $3, $4)`,
		session.TokenHash,
		session.AccountUUID,
		session.UserAgent,
		session.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}
	return nil
}

func (s *Store) GetViewer(ctx context.Context, tokenHash []byte) (account.Viewer, error) {
	viewer := account.Viewer{Role: account.RoleUser}
	err := s.db.QueryRowContext(ctx, `
		UPDATE session s
		SET last_used_at = NOW()
		FROM account a
		WHERE s.token_hash = $1
			AND s.expires_at > NOW()
			AND a.uuid = s.account_uuid
		RETURNING a.uuid, a.username`,
		tokenHash,
	).Scan(&viewer.UUID, &viewer.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return account.Viewer{}, ErrSessionNotFound
	}
	if err != nil {
		return account.Viewer{}, fmt.Errorf("getting viewer: %w", err)
	}
	return viewer, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM session WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}
