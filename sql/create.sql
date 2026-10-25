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

CREATE TABLE IF NOT EXISTS media (
	uuid UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
	account_uuid UUID NOT NULL,
	kind TEXT NOT NULL,
	status TEXT NOT NULL,
	content_type TEXT NOT NULL, -- content type of display_key
	display_key TEXT NOT NULL DEFAULT '',
	thumbnail_key TEXT NOT NULL DEFAULT '',
	original_key TEXT NOT NULL DEFAULT '', -- videos only, deleted after processing
	width INTEGER NOT NULL DEFAULT 0,
	height INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	size_bytes BIGINT NOT NULL DEFAULT 0, -- size of display_key
	failure_reason TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	processed_at TIMESTAMPTZ NULL,
	CONSTRAINT media_account_fkey FOREIGN KEY (account_uuid) REFERENCES account (uuid) ON DELETE CASCADE,
	CONSTRAINT media_kind_check CHECK (kind IN ('image', 'video')),
	CONSTRAINT media_status_check CHECK (status IN ('processing', 'ready', 'failed'))
);
CREATE INDEX IF NOT EXISTS media_account_uuid_idx ON media (account_uuid);

CREATE TABLE IF NOT EXISTS post (
	uuid UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
	account_uuid UUID NOT NULL,
	media_uuid UUID NOT NULL,
	caption TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	deleted_at TIMESTAMPTZ NULL,
	CONSTRAINT post_account_fkey FOREIGN KEY (account_uuid) REFERENCES account (uuid) ON DELETE CASCADE,
	CONSTRAINT post_media_fkey FOREIGN KEY (media_uuid) REFERENCES media (uuid) ON DELETE RESTRICT,
	CONSTRAINT post_media_uuid_key UNIQUE (media_uuid)
);
CREATE INDEX IF NOT EXISTS post_created_idx ON post (created_at DESC, uuid DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS post_account_created_idx ON post (account_uuid, created_at DESC, uuid DESC) WHERE deleted_at IS NULL;
