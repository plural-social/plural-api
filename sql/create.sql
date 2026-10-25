-- Latest full schema. Keep in sync with internal/postgres/migrations.

CREATE TABLE IF NOT EXISTS account (
	uuid UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
	username TEXT NOT NULL,
	email TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT account_username_key UNIQUE (username),
	CONSTRAINT account_email_key UNIQUE (email)
);
