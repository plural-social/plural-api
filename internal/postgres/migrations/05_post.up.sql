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
