CREATE TABLE IF NOT EXISTS media (
	uuid UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
	account_uuid UUID NOT NULL,
	kind TEXT NOT NULL,
	status TEXT NOT NULL,
	content_type TEXT NOT NULL,
	display_key TEXT NOT NULL DEFAULT '',
	thumbnail_key TEXT NOT NULL DEFAULT '',
	original_key TEXT NOT NULL DEFAULT '',
	width INTEGER NOT NULL DEFAULT 0,
	height INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	size_bytes BIGINT NOT NULL DEFAULT 0,
	failure_reason TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	processed_at TIMESTAMPTZ NULL,
	CONSTRAINT media_account_fkey FOREIGN KEY (account_uuid) REFERENCES account (uuid) ON DELETE CASCADE,
	CONSTRAINT media_kind_check CHECK (kind IN ('image', 'video')),
	CONSTRAINT media_status_check CHECK (status IN ('processing', 'ready', 'failed'))
);
CREATE INDEX IF NOT EXISTS media_account_uuid_idx ON media (account_uuid);
