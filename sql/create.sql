-- Latest full schema. Keep in sync with internal/postgres/migrations.

CREATE TABLE IF NOT EXISTS account (
	uuid UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
	username TEXT NOT NULL,
	email TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	display_name TEXT NOT NULL DEFAULT '',
	bio TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT account_username_key UNIQUE (username),
	CONSTRAINT account_email_key UNIQUE (email)
);

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
