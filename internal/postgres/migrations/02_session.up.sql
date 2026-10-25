CREATE TABLE IF NOT EXISTS session (
	token_hash BYTEA NOT NULL PRIMARY KEY,
	account_uuid UUID NOT NULL,
	user_agent TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	last_used_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	expires_at TIMESTAMPTZ NOT NULL,
	CONSTRAINT session_account_fkey FOREIGN KEY (account_uuid) REFERENCES account (uuid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS session_account_uuid_idx ON session (account_uuid);
